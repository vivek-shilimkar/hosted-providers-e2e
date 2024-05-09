/*
Copyright © 2023 - 2024 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package p0_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rancher-sandbox/qase-ginkgo"
	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

const (
	increaseBy = 1
)

var (
	ctx         helpers.Context
	clusterName string
	testCaseID  int64
	zone        = helpers.GetGKEZone()
	project     = helpers.GetGKEProjectID()
)

func TestP0(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "P0 Suite")
}

var _ = BeforeEach(func() {
	ctx = helpers.CommonBeforeSuite(helpers.Provider)
	clusterName = namegen.AppendRandomString(helpers.ClusterNamePrefix)
})

var _ = ReportBeforeEach(func(report SpecReport) {
	// Reset case ID
	testCaseID = -1
})

var _ = ReportAfterEach(func(report SpecReport) {
	// Add result in Qase if asked
	Qase(testCaseID, report)
})

func p0upgradeK8sVersionCheck(cluster *management.Cluster, client *rancher.Client, clusterName string) {
	currentVersion := cluster.GKEConfig.KubernetesVersion
	versions, err := helper.ListGKEAvailableVersions(client, cluster.ID)
	Expect(err).To(BeNil())
	Expect(versions).ToNot(BeEmpty())
	upgradeToVersion := &versions[0]
	GinkgoLogr.Info(fmt.Sprintf("Upgrading cluster to GKE version %s", *upgradeToVersion))

	By("upgrading the ControlPlane", func() {
		cluster, err = helper.UpgradeKubernetesVersion(cluster, upgradeToVersion, client, false)
		Expect(err).To(BeNil())
		err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
		Expect(err).To(BeNil())
		Expect(cluster.GKEConfig.KubernetesVersion).To(BeEquivalentTo(upgradeToVersion))
		for _, np := range cluster.GKEConfig.NodePools {
			Expect(np.Version).To(BeEquivalentTo(currentVersion))
		}
	})

	By("upgrading the NodePools", func() {
		cluster, err = helper.UpgradeKubernetesVersion(cluster, upgradeToVersion, client, true)
		Expect(err).To(BeNil())
		err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
		Expect(err).To(BeNil())

		Expect(cluster.GKEConfig.KubernetesVersion).To(BeEquivalentTo(upgradeToVersion))
		for _, np := range cluster.GKEConfig.NodePools {
			Expect(np.Version).To(BeEquivalentTo(upgradeToVersion))
		}
	})
}

func p0NodesChecks(cluster *management.Cluster, client *rancher.Client, clusterName string) {
	helpers.ClusterIsReadyChecks(cluster, client, clusterName)

	currentNodePoolNumber := len(cluster.GKEConfig.NodePools)
	initialNodeCount := *cluster.GKEConfig.NodePools[0].InitialNodeCount

	By("scaling up the nodepool", func() {
		var err error
		cluster, err = helper.ScaleNodePool(cluster, client, initialNodeCount+1)
		Expect(err).To(BeNil())
		err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
		Expect(err).To(BeNil())
		for i := range cluster.GKEConfig.NodePools {
			Expect(*cluster.GKEConfig.NodePools[i].InitialNodeCount).To(BeNumerically("==", initialNodeCount+1))
		}
	})

	By("scaling down the nodepool", func() {
		var err error
		cluster, err = helper.ScaleNodePool(cluster, client, initialNodeCount)
		Expect(err).To(BeNil())
		err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
		Expect(err).To(BeNil())
		for i := range cluster.GKEConfig.NodePools {
			Expect(*cluster.GKEConfig.NodePools[i].InitialNodeCount).To(BeNumerically("==", initialNodeCount))
		}
	})

	By("adding a nodepool", func() {
		var err error
		cluster, err = helper.AddNodePool(cluster, increaseBy, client)
		Expect(err).To(BeNil())
		err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
		Expect(err).To(BeNil())
		Expect(len(cluster.GKEConfig.NodePools)).To(BeNumerically("==", currentNodePoolNumber+1))
		for _, np := range cluster.GKEConfig.NodePools {
			// qase: HIGHLANDER-35
			Expect(np.Version).To(BeEquivalentTo(cluster.GKEConfig.KubernetesVersion))
		}
	})
	By("deleting the nodepool", func() {
		var err error
		cluster, err = helper.DeleteNodePool(cluster, client)
		Expect(err).To(BeNil())
		err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
		Expect(err).To(BeNil())
		Expect(len(cluster.GKEConfig.NodePools)).To(BeNumerically("==", currentNodePoolNumber))

	})
}
