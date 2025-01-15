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

package backup_test

import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rancher-sandbox/qase-ginkgo"

	"testing"

	"github.com/rancher/hosted-providers-e2e/hosted/aks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"
)

const (
	increaseBy = 1
)

var (
	testCaseID              int64
	clusterName, backupFile string
	ctx                     helpers.RancherContext
	cluster                 *management.Cluster
	location                = helpers.GetAKSLocation()
	k3sVersion              = os.Getenv("INSTALL_K3S_VERSION")
)

func TestBackupRestore(t *testing.T) {
	RegisterFailHandler(Fail)
	helpers.CommonSynchronizedBeforeSuite()
	ctx = helpers.CommonBeforeSuite()
	RunSpecs(t, "BackupRestore Suite")
}

var _ = ReportBeforeEach(func(report SpecReport) {
	// Reset case ID
	testCaseID = -1
})

var _ = ReportAfterEach(func(report SpecReport) {
	// Add result in Qase if asked
	Qase(testCaseID, report)
})

var _ = BeforeEach(func() {
	clusterName = namegen.AppendRandomString(helpers.ClusterNamePrefix)
	k8sVersion, err := helper.GetK8sVersion(ctx.RancherAdminClient, ctx.CloudCredID, location, false)
	Expect(err).NotTo(HaveOccurred())

	GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))
	cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, nil)
	Expect(err).To(BeNil())
	cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
	Expect(err).To(BeNil())
})

var _ = AfterEach(func() {
	if ctx.ClusterCleanup && cluster != nil {
		err := helper.DeleteAKSHostCluster(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
	} else {
		fmt.Println("Skipping downstream cluster deletion: ", clusterName)
	}
})

func restoreNodesChecks(cluster *management.Cluster, client *rancher.Client, clusterName string) {
	helpers.ClusterIsReadyChecks(cluster, client, clusterName)
	initialNodeCount := *cluster.AKSConfig.NodePools[0].Count

	By("scaling up the nodepool", func() {
		var err error
		cluster, err = helper.ScaleNodePool(cluster, client, initialNodeCount+1, true, true)
		Expect(err).To(BeNil())
	})

	By("adding a nodepool", func() {
		var err error
		cluster, err = helper.AddNodePool(cluster, increaseBy, client, true, true)
		Expect(err).To(BeNil())
	})
}
