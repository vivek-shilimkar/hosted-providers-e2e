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

package p1_test

import (
	"fmt"
	"maps"
	"strconv"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
	. "github.com/rancher-sandbox/qase-ginkgo"
	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters"
	"github.com/rancher/shepherd/extensions/clusters/eks"
	"github.com/rancher/shepherd/pkg/config"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"
	"k8s.io/utils/pointer"

	"github.com/rancher/hosted-providers-e2e/hosted/eks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var (
	ctx         helpers.RancherContext
	cluster     *management.Cluster
	clusterName string
	testCaseID  int64
	region      = helpers.GetEKSRegion()
)

func TestP1(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "P1 Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	helpers.CommonSynchronizedBeforeSuite()
	return nil
}, func() {
	ctx = helpers.CommonBeforeSuite()
})

var _ = BeforeEach(func() {
	// Setting this to nil ensures we do not use the `cluster` variable value from another test running in parallel with this one.
	cluster = nil
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

// updateClusterInUpdatingState runs checks to ensure cluster in an updating state can be updated
func updateClusterInUpdatingState(cluster *management.Cluster, client *rancher.Client, upgradeToVersion string) {
	var (
		exists bool
		err    error
	)
	cluster, err = helper.UpgradeClusterKubernetesVersion(cluster, upgradeToVersion, client, false)
	Expect(err).To(BeNil())
	Expect(*cluster.EKSConfig.KubernetesVersion).To(Equal(upgradeToVersion))

	err = clusters.WaitClusterToBeInUpgrade(client, cluster.ID)
	Expect(err).To(BeNil())

	loggingTypes := []string{"api"}
	cluster, err = helper.UpdateLogging(cluster, client, loggingTypes, false)
	Expect(err).To(BeNil())
	Expect(*cluster.EKSConfig.LoggingTypes).Should(HaveExactElements(loggingTypes))

	err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
	Expect(err).To(BeNil())

	Eventually(func() bool {
		GinkgoLogr.Info("Waiting for the updated changes to appear in EKSStatus.UpstreamSpec ...")
		cluster, err = client.Management.Cluster.ByID(cluster.ID)
		Expect(err).To(BeNil())

		for _, loggingType := range loggingTypes {
			exists = helpers.ContainsString(*cluster.EKSStatus.UpstreamSpec.LoggingTypes, loggingType)
		}
		return exists && *cluster.EKSStatus.UpstreamSpec.KubernetesVersion == upgradeToVersion
	}, "15m", "30s").Should(BeTrue())
}

func syncK8sVersionUpgradeCheck(cluster *management.Cluster, client *rancher.Client, upgradeNodeGroup bool, k8sVersion, upgradeToVersion string) {
	var err error
	GinkgoLogr.Info("Upgrading cluster to version:" + upgradeToVersion)

	By("upgrading control plane", func() {
		err = helper.UpgradeEKSClusterOnAWS(region, clusterName, upgradeToVersion)
		Expect(err).To(BeNil())

		Eventually(func() string {
			GinkgoLogr.Info("Waiting for k8s upgrade to appear in EKSStatus.UpstreamSpec ...")
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			return *cluster.EKSStatus.UpstreamSpec.KubernetesVersion
		}, tools.SetTimeout(5*time.Minute), 10*time.Second).Should(Equal(upgradeToVersion), "Failed while waiting for k8s upgrade to appear in EKSStatus.UpstreamSpec")

		if !helpers.IsImport {
			// For imported clusters, EKSConfig always has null values; so we check EKSConfig only when testing provisioned clusters
			for _, ng := range *cluster.EKSConfig.NodeGroups {
				Expect(*ng.Version).To(BeEquivalentTo(k8sVersion), "EKSConfig.NodePools check failed")
			}
		}
	})

	if upgradeNodeGroup {
		By("upgrading the nodegroup", func() {
			GinkgoLogr.Info(fmt.Sprintf("Upgrading Nodegroup's EKS version to %s", upgradeToVersion))
			for _, ng := range *cluster.EKSStatus.UpstreamSpec.NodeGroups {
				err = helper.UpgradeEKSNodegroupOnAWS(region, clusterName, *ng.NodegroupName, upgradeToVersion)
				Expect(err).To(BeNil())
			}

			Eventually(func() bool {
				GinkgoLogr.Info("Waiting for the nodegroup upgrade to appear in EKSStatus.UpstreamSpec ...")
				cluster, err = client.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				for _, ng := range *cluster.EKSStatus.UpstreamSpec.NodeGroups {
					if ng.Version == nil || *ng.Version != upgradeToVersion {
						return false
					}
				}
				return true
			}, tools.SetTimeout(7*time.Minute), 10*time.Second).Should(BeTrue(), "Failed while waiting for nodegroup k8s upgrade to appear in EKSStatus.UpstreamSpec")

			if !helpers.IsImport {
				// For imported clusters, EKSConfig always has null values; so we check EKSConfig only when testing provisioned clusters
				Expect(*cluster.EKSConfig.KubernetesVersion).To(Equal(upgradeToVersion))
				for _, ng := range *cluster.EKSConfig.NodeGroups {
					Expect(ng.Version).ToNot(BeNil())
					Expect(*ng.Version).To(BeEquivalentTo(upgradeToVersion), "EKSConfig.NodePools upgrade check failed")
				}
			}
		})
	}
}

func syncAWSToRancherCheck(cluster *management.Cluster, client *rancher.Client, k8sVersion, upgradeToVersion string) {
	loggingTypes := []string{"api", "audit", "authenticator", "controllerManager", "scheduler"}
	By("Enabling the LoggingTypes", func() {
		err := helper.UpdateLoggingOnAWS(clusterName, region, loggingTypes, nil)
		Expect(err).To(BeNil())

		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			updated := len(*cluster.EKSStatus.UpstreamSpec.LoggingTypes) == len(loggingTypes)
			if !helpers.IsImport {
				updated = updated && len(*cluster.EKSConfig.LoggingTypes) == len(loggingTypes)
			}
			return updated
		}, "10m", "7s").Should(BeTrue(), "Timed out waiting for LoggingTypes update to appear in Rancher")

		for _, lType := range loggingTypes {
			if !helpers.IsImport {
				Expect(*cluster.EKSConfig.LoggingTypes).To(ContainElement(lType))
			}
			Expect(*cluster.EKSStatus.UpstreamSpec.LoggingTypes).To(ContainElement(lType))
		}
	})

	By("Disabling the LoggingTypes", func() {
		err := helper.UpdateLoggingOnAWS(clusterName, region, nil, []string{"all"})
		Expect(err).To(BeNil())
		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			updated := len(*cluster.EKSStatus.UpstreamSpec.LoggingTypes) == 0
			if !helpers.IsImport {
				updated = updated && len(*cluster.EKSConfig.LoggingTypes) == 0
			}
			return updated
		}, "10m", "7s").Should(BeTrue(), "Timed out waiting for LoggingTypes update to appear in Rancher")
	})

	By("Updating public/private access and CIDRs", func() {
		cidrs := []string{"0.0.0.0/0", helpers.GetRancherIP() + "/32"}
		err := helper.UpdateVPCAccess(clusterName, region, true, true, cidrs)
		Expect(err).To(BeNil())
		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			privateUpdated := *cluster.EKSStatus.UpstreamSpec.PrivateAccess
			publicUpdated := *cluster.EKSStatus.UpstreamSpec.PublicAccess
			cidrUpdated := len(*cluster.EKSStatus.UpstreamSpec.PublicAccessSources) == len(cidrs)
			if !helpers.IsImport {
				privateUpdated = privateUpdated && *cluster.EKSConfig.PrivateAccess
				publicUpdated = publicUpdated && *cluster.EKSConfig.PublicAccess
				cidrUpdated = cidrUpdated && len(*cluster.EKSConfig.PublicAccessSources) == len(cidrs)
			}
			return privateUpdated && publicUpdated && cidrUpdated
		}, "10m", "7s").Should(BeTrue(), "Timed out waiting for private/public access to appear in Rancher")

		for _, cidr := range cidrs {
			if !helpers.IsImport {
				Expect(*cluster.EKSConfig.PublicAccessSources).To(ContainElement(cidr))
			}
			Expect(*cluster.EKSStatus.UpstreamSpec.PublicAccessSources).To(ContainElement(cidr))
		}
	})

	By("upgrading control plane and nodegroup", func() {
		syncK8sVersionUpgradeCheck(cluster, client, true, k8sVersion, upgradeToVersion)
	})

	By("scaling up the NodeGroup", func() {
		upstreamNodeGroups := *cluster.EKSStatus.UpstreamSpec.NodeGroups
		ng := upstreamNodeGroups[0]
		var nodeCount int64 = 2
		if ng.DesiredSize != nil {
			nodeCount = *ng.DesiredSize + 2
		}
		err := helper.ScaleNodeGroupOnAWS(*ng.NodegroupName, clusterName, region, nodeCount, nodeCount+2, nodeCount-1)
		Expect(err).To(BeNil())
		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			upstreamNodeGroups = *cluster.EKSStatus.UpstreamSpec.NodeGroups
			updated := *upstreamNodeGroups[0].DesiredSize == nodeCount
			if !helpers.IsImport {
				configNodeGroups := *cluster.EKSConfig.NodeGroups
				updated = updated && *configNodeGroups[0].DesiredSize == nodeCount
			}
			return updated
		}, "10m", "7s").Should(BeTrue(), "Timed out waiting for NodeGroup scale to show in Rancher")
	})

	var nodeName = namegen.AppendRandomString("ng")
	ngCount := len(*cluster.EKSStatus.UpstreamSpec.NodeGroups)
	if helpers.IsImport {
		// The following error is encountered when adding nodegroup to a non-eksctl managed i.e. rancher provisioned cluster; so we skip it for now
		// Error: loading VPC spec for cluster "auto-eks-hp-ci-atiia": VPC configuration required for creating nodegroups on clusters not owned by eksctl: vpc.subnets, vpc.id, vpc.securityGroup
		// If this is implemented for rancher-provisioned cluster; make sure to check for Config spec.
		By("adding a NodeGroup", func() {
			err := helper.AddNodeGroupOnAWS(nodeName, clusterName, region)
			Expect(err).To(BeNil())

			Eventually(func() bool {
				cluster, err = client.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				if len(*cluster.EKSStatus.UpstreamSpec.NodeGroups) != ngCount+1 {
					return false
				}
				for _, ng := range *cluster.EKSStatus.UpstreamSpec.NodeGroups {
					if *ng.NodegroupName == nodeName {
						return true
					}
				}
				return false
			}, "10m", "7s").Should(BeTrue(), "Timed out waiting for new NodeGroup to appear in Rancher")
		})
	}

	By("deleting a NodeGroup", func() {
		if !helpers.IsImport {
			// adding an extra nodegroup to make sure there are at least 2 nodes in the cluster before deleting it for rancher-provisioned clusters
			// remove this if and when By("adding a Nodegroup") is implemented for rancher-provisioned cluster
			var err error
			cluster, err = helper.AddNodeGroup(cluster, 1, client, true, true)
			Expect(err).To(BeNil())
			configNodeGroups := *cluster.EKSConfig.NodeGroups
			nodeName = *configNodeGroups[1].NodegroupName
		}
		err := helper.ModifyEKSNodegroupOnAWS(region, clusterName, nodeName, "delete", "--wait")
		Expect(err).To(BeNil())
		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			updated := len(*cluster.EKSStatus.UpstreamSpec.NodeGroups) != ngCount
			if !helpers.IsImport {
				updated = updated && len(*cluster.EKSConfig.NodeGroups) != ngCount
			}
			if updated {
				return false
			}
			var nodeGroupPresentInUpstream, nodeGroupPresentInConfig bool
			for _, ng := range *cluster.EKSStatus.UpstreamSpec.NodeGroups {
				if *ng.NodegroupName == nodeName {
					nodeGroupPresentInUpstream = true
					break
				}
			}
			if !helpers.IsImport {
				for _, ng := range *cluster.EKSConfig.NodeGroups {
					if *ng.NodegroupName == nodeName {
						nodeGroupPresentInConfig = true
						break
					}
				}
			} else {
				// if the cluster is imported, Config will be null, so we assign this variable to true to do easy check
				nodeGroupPresentInConfig = true
			}
			return nodeGroupPresentInConfig && nodeGroupPresentInUpstream
		}, "10m", "7s").Should(BeFalse(), "Timed out waiting for NodeGroup to delete from Rancher")
	})

	tags := map[string]string{"foo": "bar", "updated": "via-cli"}
	By("adding tags to EKS cluster", func() {
		err := helper.AddClusterTagsOnAWS(clusterName, region, tags)
		Expect(err).To(BeNil())
		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			upstreamTags := *cluster.EKSStatus.UpstreamSpec.Tags
			for key := range tags {
				_, existsUpstream := upstreamTags[key]

				var existsConfig bool
				if !helpers.IsImport {
					configTags := *cluster.EKSConfig.Tags
					_, existsConfig = configTags[key]
				} else {
					// if the cluster is imported, Config will be null, so we assign this variable to true to do easy check
					existsConfig = true
				}
				if !(existsConfig && existsUpstream) {
					// TODO: check this logic
					// return early if the key doesn't exist in either of the specs
					return false
				}
			}
			return true
		}, "10m", "5s").Should(BeTrue(), "Timed out waiting for EKS tags to be updated")
		for key, value := range tags {
			if !helpers.IsImport {
				Expect(*cluster.EKSConfig.Tags).To(HaveKeyWithValue(key, value))
			}
			Expect(*cluster.EKSStatus.UpstreamSpec.Tags).To(HaveKeyWithValue(key, value))
		}
	})

	By("removing tags from EKS cluster", func() {
		var removeTags []string
		for key := range tags {
			removeTags = append(removeTags, key)
		}
		err := helper.RemoveClusterTagsOnAWS(clusterName, region, removeTags)
		Expect(err).To(BeNil())
		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			upstreamTags := *cluster.EKSStatus.UpstreamSpec.Tags
			for key := range tags {
				_, existsUpstream := upstreamTags[key]
				var existsConfig bool
				if !helpers.IsImport {
					configTags := *cluster.EKSConfig.Tags
					_, existsConfig = configTags[key]
				}
				if existsConfig || existsUpstream {
					return false
				}
			}
			return true
		}, "10m", "5s").Should(BeTrue(), "Timed out waiting for EKS tags to be removed")
	})

	addLabels := map[string]string{"foo": "bar", "updated": "via-cli"}
	const ngIndex = 0
	By("add labels to Nodegroup", func() {
		upstreamNodeGroups := *cluster.EKSStatus.UpstreamSpec.NodeGroups
		err := helper.UpdateNodeGroupLabelsOnAWS(clusterName, *upstreamNodeGroups[ngIndex].NodegroupName, region, addLabels, nil)
		Expect(err).To(BeNil())
		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			upstreamNodeGroups = *cluster.EKSStatus.UpstreamSpec.NodeGroups
			upstreamLabels := *upstreamNodeGroups[ngIndex].Labels

			for key := range addLabels {
				_, existUpstream := upstreamLabels[key]
				var existConfig bool
				if !helpers.IsImport {
					configNodeGroups := *cluster.EKSConfig.NodeGroups
					configTags := *configNodeGroups[ngIndex].Labels
					_, existConfig = configTags[key]
				} else {
					// if the cluster is imported, Config will be null, so we assign this variable to true to do easy check
					existConfig = true
				}
				if !(existConfig && existUpstream) {
					// return early if the key doesn't exist in either of the specs
					return false
				}
			}
			return true
		}, "10m", "5s").Should(BeTrue(), "Timed out waiting for EKS nodegroup labels to be added")

		upstreamNodeGroups = *cluster.EKSStatus.UpstreamSpec.NodeGroups
		for key, value := range addLabels {
			if !helpers.IsImport {
				configNodeGroups := *cluster.EKSConfig.NodeGroups
				Expect(*configNodeGroups[ngIndex].Labels).To(HaveKeyWithValue(key, value))
			}
			Expect(*upstreamNodeGroups[ngIndex].Labels).To(HaveKeyWithValue(key, value))
		}
	})

	By("deleting labels from Nodegroup", func() {
		var removeLabels []string
		for key := range addLabels {
			removeLabels = append(removeLabels, key)
		}
		upstreamNodeGroups := *cluster.EKSStatus.UpstreamSpec.NodeGroups
		err := helper.UpdateNodeGroupLabelsOnAWS(clusterName, *upstreamNodeGroups[ngIndex].NodegroupName, region, nil, removeLabels)
		Expect(err).To(BeNil())
		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			upstreamNodeGroups = *cluster.EKSStatus.UpstreamSpec.NodeGroups
			upstreamLabels := *upstreamNodeGroups[ngIndex].Labels
			for _, key := range removeLabels {
				_, existsUpstream := upstreamLabels[key]
				var existsConfig bool
				if !helpers.IsImport {
					configNodeGroups := *cluster.EKSConfig.NodeGroups
					configTags := *configNodeGroups[ngIndex].Labels
					_, existsConfig = configTags[key]
				}
				if existsConfig || existsUpstream {
					// return early if the key exists in either of the specs
					return false
				}
			}
			return true
		}, "10m", "5s").Should(BeTrue(), "Timed out waiting for EKS nodegroup labels to be deleted")

		configNodeGroups := *cluster.EKSConfig.NodeGroups
		upstreamNodeGroups = *cluster.EKSStatus.UpstreamSpec.NodeGroups
		for key, value := range addLabels {
			if !helpers.IsImport {
				Expect(*configNodeGroups[ngIndex].Labels).ToNot(HaveKeyWithValue(key, value))
			}
			Expect(*upstreamNodeGroups[ngIndex].Labels).ToNot(HaveKeyWithValue(key, value))
		}
	})
}

func syncRancherToAWSCheck(cluster *management.Cluster, client *rancher.Client, k8sVersion, upgradeToVersion string) {
	var err error
	loggingTypes := []string{"api", "audit", "authenticator", "controllerManager", "scheduler"}
	currentNodeGroupNumber := len(*cluster.EKSConfig.NodeGroups)
	configNodeGroups := *cluster.EKSConfig.NodeGroups
	initialNodeCount := *configNodeGroups[0].DesiredSize

	By("upgrading control plane", func() {
		syncK8sVersionUpgradeCheck(cluster, client, false, k8sVersion, upgradeToVersion)
	})

	By("scaling up the NodeGroup", func() {
		cluster, err = helper.ScaleNodeGroup(cluster, client, initialNodeCount+1, true, true)
		Expect(err).To(BeNil())

		// Verify the existing details do NOT change in Rancher
		for _, ng := range *cluster.EKSConfig.NodeGroups {
			Expect(*ng.Version).To(BeEquivalentTo(k8sVersion))
			Expect(len(*cluster.EKSConfig.NodeGroups)).Should(BeNumerically("==", currentNodeGroupNumber))
		}

		// Verify the new edits reflect in AWS and existing details do NOT change
		var out string
		out, err = helper.GetFromEKS(region, clusterName, "cluster", "'.[]|.Version'")
		Expect(err).To(BeNil())
		Expect(out).To(Equal(upgradeToVersion))

		out, err = helper.GetFromEKS(region, clusterName, "nodegroup", "'.|length'")
		Expect(err).To(BeNil())
		Expect(strconv.Atoi(out)).To(Equal(currentNodeGroupNumber))

		out, err = helper.GetFromEKS(region, clusterName, "nodegroup", "'.[]|.DesiredCapacity'")
		Expect(err).To(BeNil())
		Expect(strconv.ParseInt(out, 10, 64)).To(Equal(initialNodeCount + 1))
	})

	By("adding a NodeGroup", func() {
		cluster, err = helper.AddNodeGroup(cluster, 1, client, true, true)
		Expect(err).To(BeNil())

		// Verify the existing details do NOT change in Rancher
		Expect(*cluster.EKSConfig.KubernetesVersion).To(Equal(upgradeToVersion))
		Expect(*cluster.EKSConfig.LoggingTypes).ShouldNot(HaveExactElements(loggingTypes))

		// Verify the new edits reflect in AWS console and existing details do NOT change
		var out string
		out, err = helper.GetFromEKS(region, clusterName, "cluster", "'.[]|.Version'")
		Expect(err).To(BeNil())
		Expect(out).To(Equal(upgradeToVersion))

		out, err = helper.GetFromEKS(region, clusterName, "nodegroup", "'.|length'")
		Expect(err).To(BeNil())
		Expect(strconv.Atoi(out)).To(Equal(currentNodeGroupNumber + 1))
	})

	By("Adding the LoggingTypes", func() {
		cluster, err = helper.UpdateLogging(cluster, client, loggingTypes, true)
		Expect(err).To(BeNil())

		// Verify the existing details do NOT change in Rancher
		Expect(*cluster.EKSConfig.KubernetesVersion).To(Equal(upgradeToVersion))
		Expect(len(*cluster.EKSConfig.NodeGroups)).To(Equal(currentNodeGroupNumber + 1))

		// Verify the new edits reflect in AWS console and existing details do NOT change
		var out string
		out, err = helper.GetFromEKS(region, clusterName, "nodegroup", "'.|length'")
		Expect(err).To(BeNil())
		Expect(strconv.Atoi(out)).To(Equal(currentNodeGroupNumber + 1))

		out, err = helper.GetFromEKS(region, clusterName, "cluster", "'.[]|.Logging|.[]|.[]|.Types'")
		Expect(err).To(BeNil())
		Expect(out).ShouldNot(HaveExactElements(loggingTypes))
	})

}

// upgradeNodeKubernetesVersionGTCP upgrades Nodegroup version greater than Controlplane's
func upgradeNodeKubernetesVersionGTCPCheck(cluster *management.Cluster, client *rancher.Client, upgradeToVersion string) {
	GinkgoLogr.Info("Upgrading only Nodegroup's EKS version to: " + upgradeToVersion)
	var err error
	cluster, err = helper.UpgradeNodeKubernetesVersion(cluster, upgradeToVersion, client, false, false, false)
	Expect(err).To(BeNil())

	// wait until the error is visible on the cluster
	Eventually(func() bool {
		cluster, err = client.Management.Cluster.ByID(cluster.ID)
		Expect(err).To(BeNil())
		// checking for both the messages since different operator version shows different messages. To be removed once the message is updated.
		// New message:  versions for cluster [1.29] and nodegroup [1.30] not compatible: all nodegroup kubernetes versions must be equal to or one minor version lower than the cluster kubernetes version
		return cluster.Transitioning == "error" && strings.Contains(cluster.TransitioningMessage, "not compatible")
	}, "1m", "3s").Should(BeTrue())
}

// invalidEndpointCheck updates PublicAccess Sources
func invalidEndpointCheck(cluster *management.Cluster, client *rancher.Client) {
	var err error
	cidr := []string{namegen.AppendRandomString("invalid")}
	cluster, _ = helper.UpdatePublicAccessSources(cluster, client, cidr, false)

	Eventually(func() bool {
		cluster, err = client.Management.Cluster.ByID(cluster.ID)
		Expect(err).To(BeNil())
		return cluster.Transitioning == "error" && strings.Contains(cluster.TransitioningMessage, "The following CIDRs are invalid in publicAccessCidrs")
	}, "2m", "3s").Should(BeTrue())
}

// invalidAccessCheck disbales both PublicAccess & PrivateAccess
func invalidAccessValuesCheck(cluster *management.Cluster, client *rancher.Client) {
	var err error
	_, err = helper.UpdateAccess(cluster, client, false, false, false)
	Expect(err).To(MatchError(ContainSubstring("public access, private access, or both must be enabled")))
}

func upgradeCPAndAddNgCheck(cluster *management.Cluster, client *rancher.Client, upgradeToVersion string) {
	var err error
	originalLen := len(*cluster.EKSConfig.NodeGroups)
	newNodeGroupName := pointer.String(namegen.AppendRandomString("ng"))
	GinkgoLogr.Info("Upgrading control plane to version:" + upgradeToVersion)

	By("upgrading the ControlPlane", func() {
		cluster, err = helper.UpgradeClusterKubernetesVersion(cluster, upgradeToVersion, ctx.RancherAdminClient, true)
		Expect(err).To(BeNil())
	})

	var eksClusterConfig management.EKSClusterConfigSpec
	config.LoadConfig(eks.EKSClusterConfigConfigurationFileKey, &eksClusterConfig)

	updateFunc := func(cluster *management.Cluster) {
		var updatedNodeGroupsList = make([]management.NodeGroup, 0)
		nodeGroups := *eksClusterConfig.NodeGroups
		newNodeGroup := nodeGroups[0]
		newNodeGroup.NodegroupName = newNodeGroupName
		updatedNodeGroupsList = append(updatedNodeGroupsList, newNodeGroup)
		cluster.EKSConfig.NodeGroups = &updatedNodeGroupsList
	}

	cluster, err = helper.UpdateCluster(cluster, client, updateFunc)
	Expect(err).To(BeNil())
	Expect(len(*cluster.EKSConfig.NodeGroups)).To(BeEquivalentTo(originalLen))
	for _, ng := range *cluster.EKSConfig.NodeGroups {
		Expect(ng.NodegroupName).To(Equal(newNodeGroupName))
	}

	err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
	Expect(err).To(BeNil())

	// wait until the update is visible on the cluster
	Eventually(func() bool {
		GinkgoLogr.Info("Waiting for the version of new nodegroup to appear in EKSStatus.UpstreamSpec ...")
		cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
		Expect(err).To(BeNil())
		for _, ng := range *cluster.EKSStatus.UpstreamSpec.NodeGroups {
			if ng.Version == nil || *ng.Version != upgradeToVersion {
				return false
			}
		}
		return true
	}, "5m", "15s").Should(BeTrue())
}

// Automate Qase 81 and 131
func updateTagsAndLabels(cluster *management.Cluster, client *rancher.Client) {
	var err error
	tags := map[string]string{
		"foo":        "bar",
		"testCaseID": "144-97-143",
	}

	labels := map[string]string{
		"testCaseID": "142-99-145",
	}

	originalClusterTags := *cluster.EKSConfig.Tags
	// updatedTags must contain both the original and the new tags
	updatedTags := make(map[string]string)
	maps.Copy(updatedTags, originalClusterTags)
	maps.Copy(updatedTags, tags)

	By("Adding cluster tags", func() {
		cluster, err = helper.UpdateClusterTags(cluster, client, updatedTags, true)
		Expect(err).To(BeNil())
	})

	By("Removing cluster tags", func() {
		cluster, err = helper.UpdateClusterTags(cluster, client, originalClusterTags, true)
		for key, value := range tags {
			Expect(*cluster.EKSConfig.Tags).ToNot(HaveKeyWithValue(key, value))
		}
	})

	configNodeGroups := *cluster.EKSConfig.NodeGroups
	originalNGLabels := *configNodeGroups[0].Labels
	// updatedNGLabels must contain both the original and the new tags
	updatedNGLabels := make(map[string]string)
	maps.Copy(updatedNGLabels, originalNGLabels)
	maps.Copy(updatedNGLabels, labels)

	originalNGTags := *configNodeGroups[0].Tags
	// updatedNGTags must contain both the original and the new tags
	updatedNGTags := make(map[string]string)
	maps.Copy(updatedNGTags, originalNGTags)
	maps.Copy(updatedNGTags, tags)

	By("Adding Nodegroup tags & labels", func() {
		cluster, err = helper.UpdateNodegroupMetadata(cluster, client, updatedNGTags, updatedNGLabels, true)
		Expect(err).To(BeNil())
	})

	By("Removing Nodegroup tags & labels", func() {
		cluster, err = helper.UpdateNodegroupMetadata(cluster, client, originalNGTags, originalNGLabels, true)
		for _, ng := range *cluster.EKSConfig.NodeGroups {
			for key, value := range tags {
				Expect(*ng.Tags).ToNot(HaveKeyWithValue(key, value))
			}
			for key, value := range labels {
				Expect(*ng.Labels).ToNot(HaveKeyWithValue(key, value))
			}
		}
	})
}

// Automates Qase: 128 and 77
func updateLoggingCheck(cluster *management.Cluster, client *rancher.Client) {
	var err error
	loggingTypes := []string{"api", "audit", "authenticator", "controllerManager", "scheduler"}
	By("Adding the LoggingTypes", func() {
		cluster, err = helper.UpdateLogging(cluster, client, loggingTypes, true)
		Expect(err).To(BeNil())
	})

	By("Removing the LoggingTypes", func() {
		cluster, err = helper.UpdateLogging(cluster, client, []string{loggingTypes[0]}, true)
		Expect(err).To(BeNil())
	})
}

// Automates Qase: 109 and 155
func updateCloudCredentialsCheck(cluster *management.Cluster, client *rancher.Client) {
	newCCID, err := helpers.CreateCloudCredentials(client)
	Expect(err).To(BeNil())
	updateFunc := func(cluster *management.Cluster) {
		cluster.EKSConfig.AmazonCredentialSecret = newCCID
	}
	cluster, err = helper.UpdateCluster(cluster, client, updateFunc)
	Expect(err).To(BeNil())
	Expect(cluster.EKSConfig.AmazonCredentialSecret).To(Equal(newCCID))
	Eventually(func() bool {
		cluster, err = client.Management.Cluster.ByID(cluster.ID)
		Expect(err).NotTo(HaveOccurred())
		return cluster.EKSStatus.UpstreamSpec.AmazonCredentialSecret == newCCID
	}, "5m", "5s").Should(BeTrue(), "Failed while upstream cloud credentials update")

	cluster, err = helper.ScaleNodeGroup(cluster, client, 3, true, true)
	Expect(err).To(BeNil())
}

// Automates Qase: 134 and
func deleteAllNodeGroupsCheck(cluster *management.Cluster, client *rancher.Client) {
	updateFunc := func(cluster *management.Cluster) {
		// setting this to nil will do nothing, so we set it to empty array
		cluster.EKSConfig.NodeGroups = &[]management.NodeGroup{}
	}

	var err error
	_, err = helper.UpdateCluster(cluster, client, updateFunc)
	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("must have at least one nodegroup"))

	updateFunc = func(cluster *management.Cluster) {
		cluster.EKSConfig.NodeGroups = nil
	}
}
