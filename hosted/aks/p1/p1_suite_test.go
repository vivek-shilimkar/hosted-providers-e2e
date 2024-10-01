package p1_test

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rancher-sandbox/qase-ginkgo"
	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"
	"k8s.io/utils/pointer"

	"github.com/rancher/hosted-providers-e2e/hosted/aks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var (
	ctx         helpers.Context
	clusterName string
	testCaseID  int64
	location    = helpers.GetAKSLocation()
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

// updateAutoScaling tests updating `autoscaling` for AKS node pools
// Qase ID: 176 and 266
func updateAutoScaling(cluster *management.Cluster, client *rancher.Client) {
	By("enabling autoscaling with custom minCount and maxCount", func() {
		var err error
		cluster, err = helper.UpdateAutoScaling(cluster, client, true, 5, 2, true)
		Expect(err).To(BeNil())
	})

	By("disabling autoscaling", func() {
		var err error
		cluster, err = helper.UpdateAutoScaling(cluster, client, false, 0, 0, true)
		Expect(err).To(BeNil())
	})
}

// Qase ID: 191 and 267
func removeSystemNpCheck(cluster *management.Cluster, client *rancher.Client) {
	updateFunc := func(cluster *management.Cluster) {
		var updatedNodePools []management.AKSNodePool
		for _, np := range cluster.AKSConfig.NodePools {
			if np.Mode == "User" {
				updatedNodePools = append(updatedNodePools, np)
			}
		}
		cluster.AKSConfig.NodePools = updatedNodePools
	}
	var err error
	cluster, err = helper.UpdateCluster(cluster, client, updateFunc)
	Expect(err).To(BeNil())
	Eventually(func() bool {
		cluster, err = client.Management.Cluster.ByID(cluster.ID)
		Expect(err).To(BeNil())
		return cluster.Transitioning == "error" && strings.Contains(cluster.TransitioningMessage, "There has to be at least one system agent pool")
	}, "5m", "5s").Should(BeTrue())
}

// Qase ID: 194, 190, and 268
func deleteAndAddNpCheck(cluster *management.Cluster, client *rancher.Client) {
	originalLen := len(cluster.AKSConfig.NodePools)
	var npToBeDeleted management.AKSNodePool
	newPoolName := fmt.Sprintf("newpool%s", namegen.RandStringLower(3))
	newPoolAZ := []string{"2", "3"}

	updateFunc := func(cluster *management.Cluster) {
		var updatedNodePools []management.AKSNodePool
		for _, np := range cluster.AKSConfig.NodePools {
			if np.Mode == "User" {
				// We do not want to delete one of the 'System' mode nodepool; since at least one is required
				npToBeDeleted = np
			} else {
				updatedNodePools = append(updatedNodePools, np)
			}
		}
		newNodePool := npToBeDeleted
		newNodePool.Name = &newPoolName
		newNodePool.AvailabilityZones = &newPoolAZ
		// testCaseID = 194
		updatedNodePools = append(updatedNodePools, newNodePool)
		cluster.AKSConfig.NodePools = updatedNodePools
	}
	var err error
	cluster, err = helper.UpdateCluster(cluster, client, updateFunc)
	Expect(err).To(BeNil())
	var (
		npDeleted = true
		npAdded   = false
	)
	for _, np := range cluster.AKSConfig.NodePools {
		if *np.Name == *npToBeDeleted.Name {
			npDeleted = false
		}
		if *np.Name == newPoolName {
			npAdded = true
			Expect(*np.AvailabilityZones).To(Equal(newPoolAZ))
		}
	}
	Expect(npAdded).To(BeTrue())
	Expect(npDeleted).To(BeTrue())
	Expect(len(cluster.AKSConfig.NodePools)).To(BeEquivalentTo(originalLen))
	err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
	Expect(err).To(BeNil())

	Eventually(func() bool {
		cluster, err = client.Management.Cluster.ByID(cluster.ID)
		Expect(err).To(BeNil())
		if len(cluster.AKSStatus.UpstreamSpec.NodePools) != originalLen {
			return false
		}
		var (
			npDeletedFromUpstream = true
			npAddedToUpstream     = false
		)
		for _, np := range cluster.AKSConfig.NodePools {
			if *np.Name == newPoolName {
				npAddedToUpstream = true
				// testCaseID = 194
				Expect(*np.AvailabilityZones).To(Equal(newPoolAZ))
			}
			if *np.Name == *npToBeDeleted.Name {
				npDeletedFromUpstream = false
			}
		}
		return npAddedToUpstream && npDeletedFromUpstream
	}, "5m", "5s").Should(BeTrue(), "Failed while waiting for node pools to be added and deleted")

}

// npUpgradeToVersionGTCPCheck runs checks when node pool is upgraded to a version greater than control plane version
// Qase ID: 183 and 269
func npUpgradeToVersionGTCPCheck(cluster *management.Cluster, client *rancher.Client) {
	k8sVersion := *cluster.AKSConfig.KubernetesVersion
	availableVersions, err := helper.ListAKSAvailableVersions(client, cluster.ID)
	Expect(err).To(BeNil())
	upgradeK8sVersion := availableVersions[0]
	cluster, err = helper.UpgradeNodeKubernetesVersion(cluster, upgradeK8sVersion, client, false, false)
	Expect(err).To(BeNil())
	Eventually(func() bool {
		cluster, err = client.Management.Cluster.ByID(cluster.ID)
		Expect(err).NotTo(HaveOccurred())
		return cluster.Transitioning == "error" && strings.Contains(cluster.TransitioningMessage, fmt.Sprintf("Node pool version %s and control plane version %s are incompatible.", upgradeK8sVersion, k8sVersion))
	}, "1m", "2s").Should(BeTrue())
}

// updateTagsCheck runs checks to add and delete the cluster with a new tag and an empty tag
// Qase ID: 177 and 270
func updateTagsCheck(cluster *management.Cluster, client *rancher.Client) {

	By("adding new tags", func() {
		updateFunc := func(cluster *management.Cluster) {
			cluster.AKSConfig.Tags["empty-tag"] = ""
			cluster.AKSConfig.Tags["new"] = "tag"
		}
		var err error
		cluster, err = helper.UpdateCluster(cluster, client, updateFunc)
		Expect(err).To(BeNil())
		Expect(cluster.AKSConfig.Tags).To(HaveKeyWithValue("empty-tag", ""))
		Expect(cluster.AKSConfig.Tags).To(HaveKeyWithValue("new", "tag"))

		Eventually(func() int {
			GinkgoLogr.Info("Waiting for the tags to be added ...")
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())

			var count int
			for key, value := range cluster.AKSStatus.UpstreamSpec.Tags {
				if (key == "empty-tag" && value == "") || (key == "new" && value == "tag") {
					count++
				}
			}
			return count
		}, "5m", "5s").Should(Equal(2))
	})

	By("removing the tags", func() {
		updateFunc := func(cluster *management.Cluster) {
			delete(cluster.AKSConfig.Tags, "empty-tag")
			delete(cluster.AKSConfig.Tags, "new")
		}
		var err error
		cluster, err = helper.UpdateCluster(cluster, client, updateFunc)
		Expect(err).To(BeNil())

		Expect(cluster.AKSConfig.Tags).ToNot(HaveKeyWithValue("empty-tag", ""))
		Expect(cluster.AKSConfig.Tags).ToNot(HaveKeyWithValue("new", "tag"))

		Eventually(func() int {
			GinkgoLogr.Info("Waiting for the tags to be removed ...")
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			var count int
			for key, value := range cluster.AKSStatus.UpstreamSpec.Tags {
				if (key == "empty-tag" && value == "") || (key == "new" && value == "tag") {
					count++
				}
			}
			return count
		}, "7m", "5s").Should(Equal(0))
	})
}

// Qase ID: 200 and 271
func updateMonitoringCheck(cluster *management.Cluster, client *rancher.Client) {
	By("enabling the monitoring", func() {
		updateFunc := func(cluster *management.Cluster) {
			cluster.AKSConfig.Monitoring = pointer.Bool(true)
		}
		var err error
		cluster, err = helper.UpdateCluster(cluster, client, updateFunc)
		Expect(err).To(BeNil())
		Expect(*cluster.AKSConfig.Monitoring).To(BeTrue())
		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			return cluster.AKSStatus.UpstreamSpec.Monitoring != nil && *cluster.AKSStatus.UpstreamSpec.Monitoring
		}, "7m", "5s").Should(BeTrue())
	})

	By("disabling the monitoring", func() {
		updateFunc := func(cluster *management.Cluster) {
			cluster.AKSConfig.Monitoring = pointer.Bool(false)
		}
		var err error
		cluster, err = helper.UpdateCluster(cluster, client, updateFunc)
		Expect(err).To(BeNil())

		Expect(*cluster.AKSConfig.Monitoring).To(BeFalse())
		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			return cluster.AKSStatus.UpstreamSpec.Monitoring != nil && *cluster.AKSStatus.UpstreamSpec.Monitoring
		}, "7m", "5s").Should(BeFalse())
	})

}

// Qase ID: 202 and 290
func updateSystemNodePoolCountToZeroCheck(cluster *management.Cluster, client *rancher.Client) {
	updateFunc := func(cluster *management.Cluster) {
		nodepools := cluster.AKSConfig.NodePools
		for i, nodepool := range nodepools {
			if nodepool.Mode == "System" {
				nodepools[i].Count = pointer.Int64(0)
			}
		}
		cluster.AKSConfig.NodePools = nodepools
	}
	var err error
	cluster, err = helper.UpdateCluster(cluster, client, updateFunc)
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() bool {
		cluster, err = client.Management.Cluster.ByID(cluster.ID)
		Expect(err).NotTo(HaveOccurred())
		return cluster.Transitioning == "error" && strings.Contains(cluster.TransitioningMessage, "It must be greater or equal to minCount:1 and less than or equal to maxCount:1000")
	}, "1m", "2s").Should(BeTrue())
}

// Qase ID: 204 and 289
func updateSystemNodePoolCheck(cluster *management.Cluster, client *rancher.Client) {
	var (
		count              int64 = 4
		minCount           int64 = 2
		maxCount           int64 = 10
		autoscalingEnabled       = true
	)
	const systemMode = "System"
	updateFunc := func(cluster *management.Cluster) {
		nodepools := cluster.AKSConfig.NodePools
		for i := range nodepools {
			if nodepools[i].Mode == systemMode {
				nodepools[i].Count = &count
				nodepools[i].EnableAutoScaling = &autoscalingEnabled
				nodepools[i].MinCount = &minCount
				nodepools[i].MaxCount = &maxCount
			}
		}
		cluster.AKSConfig.NodePools = nodepools
	}
	var err error
	cluster, err = helper.UpdateCluster(cluster, client, updateFunc)
	Expect(err).To(BeNil())

	for _, np := range cluster.AKSConfig.NodePools {
		if np.Mode == systemMode {
			Expect(*np.EnableAutoScaling).To(BeTrue())
			Expect(*np.MinCount).To(BeNumerically("==", minCount))
			Expect(*np.MaxCount).To(BeNumerically("==", maxCount))
			Expect(*np.Count).To(BeNumerically("==", count))
		}
	}

	Eventually(func() bool {
		cluster, err = client.Management.Cluster.ByID(cluster.ID)
		Expect(err).To(BeNil())
		for _, np := range cluster.AKSStatus.UpstreamSpec.NodePools {
			if np.Mode == systemMode {
				if !((np.EnableAutoScaling != nil && *np.EnableAutoScaling == true) && (*np.MaxCount == maxCount) && (*np.MinCount == minCount) && (*np.Count == count)) {
					return false
				}
			}
		}
		return true
	}, "5m", "5s").Should(BeTrue(), "Failed while upstream nodepool update")
}

// Qase ID: 230 and 291
func updateNodePoolModeCheck(cluster *management.Cluster, client *rancher.Client) {
	var originalModeMap = make(map[string]string)
	updateFunc := func(cluster *management.Cluster) {
		nodepools := cluster.AKSConfig.NodePools
		for i := range nodepools {
			originalModeMap[*nodepools[i].Name] = nodepools[i].Mode
			if nodepools[i].Mode == "User" {
				nodepools[i].Mode = "System"
			} else if nodepools[i].Mode == "System" {
				nodepools[i].Mode = "User"
			}
		}
	}
	var err error
	cluster, err = helper.UpdateCluster(cluster, client, updateFunc)
	Expect(err).To(BeNil())
	for _, np := range cluster.AKSConfig.NodePools {
		Expect(np.Mode).ToNot(Equal(originalModeMap[*np.Name]))
	}
	err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
	Expect(err).To(BeNil())

	Eventually(func() bool {
		cluster, err = client.Management.Cluster.ByID(cluster.ID)
		Expect(err).NotTo(HaveOccurred())
		for _, np := range cluster.AKSStatus.UpstreamSpec.NodePools {
			if np.Mode == originalModeMap[*np.Name] {
				return false
			}
		}
		return true
	}, "5m", "5s").Should(BeTrue(), "Failed while upstream nodepool mode update")
}

// Qase ID: 221 and 292
func updateCloudCredentialsCheck(cluster *management.Cluster, client *rancher.Client) {
	newCCID, err := helpers.CreateCloudCredentials(client)
	Expect(err).To(BeNil())
	updateFunc := func(cluster *management.Cluster) {
		cluster.AKSConfig.AzureCredentialSecret = newCCID
	}
	cluster, err = helper.UpdateCluster(cluster, client, updateFunc)
	Expect(err).To(BeNil())
	Expect(cluster.AKSConfig.AzureCredentialSecret).To(Equal(newCCID))
	Eventually(func() bool {
		cluster, err = client.Management.Cluster.ByID(cluster.ID)
		Expect(err).NotTo(HaveOccurred())
		return cluster.AKSStatus.UpstreamSpec.AzureCredentialSecret == newCCID
	}, "5m", "5s").Should(BeTrue(), "Failed while upstream cloud credentials update")

	cluster, err = helper.AddNodePool(cluster, 1, client, true, true)
	Expect(err).To(BeNil())
}

// Qase ID: 224 and 293
func syncAddNodePoolFromAzureAndRancher(cluster *management.Cluster, client *rancher.Client) {
	initialNPCount := len(cluster.AKSConfig.NodePools)
	const npAzure = "npazure"
	By("adding nodepool from Azure", func() {
		err := helper.AddNodePoolOnAzure(npAzure, cluster.AKSConfig.ClusterName, cluster.AKSConfig.ResourceGroup, "2")
		Expect(err).To(BeNil())

		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).NotTo(HaveOccurred())
			for _, nodePool := range cluster.AKSStatus.UpstreamSpec.NodePools {
				if *nodePool.Name == npAzure {
					return true
				}
			}
			return false
		}, "5m", "5s").Should(BeTrue(), "Timed out while waiting for sync from Azure")

		if !helpers.IsImport {
			// skip this check if the cluster is imported since the AKSConfig value will not be updated
			Expect(cluster.AKSConfig.NodePools).To(HaveLen(initialNPCount + 1))
		}
	})

	By("adding nodepool from Rancher", func() {
		if helpers.IsImport {
			// if the cluster is imported, update the AKSConfig value to match UpstreamSpec so that it can perform upcoming checks correctly
			cluster.AKSConfig = cluster.AKSStatus.UpstreamSpec
		}
		var err error
		cluster, err = helper.AddNodePool(cluster, 1, client, true, true)
		Expect(err).To(BeNil())
	})
}

// Qase ID: 225 and 294
func upgradeCPK8sFromAzureAndNPFromRancherCheck(cluster *management.Cluster, client *rancher.Client, k8sVersion, upgradeToVersion string) {
	By("upgrading control plane k8s version from Azure", func() {
		err := helper.UpgradeAKSOnAzure(clusterName, cluster.AKSConfig.ResourceGroup, upgradeToVersion, "--control-plane-only")
		Expect(err).To(BeNil())
		Eventually(func() bool {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).NotTo(HaveOccurred())
			return *cluster.AKSStatus.UpstreamSpec.KubernetesVersion == upgradeToVersion
		}, "5m", "5s").Should(BeTrue(), "Timed out while waiting for upgrade to appear in UpstreamSpec")

		for _, nodepool := range cluster.AKSStatus.UpstreamSpec.NodePools {
			// NodePool version must remain the same
			Expect(*nodepool.OrchestratorVersion).To(Equal(k8sVersion))
		}

		if !helpers.IsImport {
			// skip this check if the cluster is imported since the AKSConfig value will not be updated
			Expect(*cluster.AKSConfig.KubernetesVersion).To(Equal(upgradeToVersion))
			for _, nodepool := range cluster.AKSConfig.NodePools {
				// NodePool version must remain the same
				Expect(*nodepool.OrchestratorVersion).To(Equal(k8sVersion))
			}
		}
	})

	By("upgrading node pool k8s version from Rancher", func() {
		if helpers.IsImport {
			// if the cluster is imported, update the AKSConfig value to match UpstreamSpec so that it can perform upcoming checks correctly
			cluster.AKSConfig = cluster.AKSStatus.UpstreamSpec
		}
		var err error
		cluster, err = helper.UpgradeNodeKubernetesVersion(cluster, upgradeToVersion, client, true, true)
		Expect(err).To(BeNil())
	})
}

// Qase ID: 275 and 276
func noAvailabilityZoneP0Checks(cluster *management.Cluster, client *rancher.Client) {
	helpers.ClusterIsReadyChecks(cluster, client, clusterName)

	for _, nodepool := range cluster.AKSConfig.NodePools {
		Expect(nodepool.AvailabilityZones).To(BeNil())
	}

	for _, nodepool := range cluster.AKSStatus.UpstreamSpec.NodePools {
		Expect(nodepool.AvailabilityZones).To(BeNil())
	}

	var err error
	var availableVersions []string
	availableVersions, err = helper.ListAKSAvailableVersions(client, cluster.ID)
	Expect(err).To(BeNil())
	upgradeToVersion := availableVersions[0]

	By("upgrading the cluster control plane", func() {
		cluster, err = helper.UpgradeClusterKubernetesVersion(cluster, upgradeToVersion, client, true)
		Expect(err).To(BeNil())
	})

	By("upgrading the cluster nodepools", func() {
		cluster, err = helper.UpgradeNodeKubernetesVersion(cluster, upgradeToVersion, client, true, true)
		Expect(err).To(BeNil())
	})

	By("Adding a nodepool", func() {
		initialNPCount := len(cluster.AKSConfig.NodePools)
		newNPName := fmt.Sprintf("newNPName%s", namegen.RandStringLower(3))
		updateFunc := func(cluster *management.Cluster) {
			nodepools := cluster.AKSConfig.NodePools
			npTemplate := nodepools[0]
			newNP := npTemplate
			newNP.Name = &newNPName
			nodepools = append(nodepools, newNP)
			cluster.AKSConfig.NodePools = nodepools
		}
		cluster, err = helper.UpdateCluster(cluster, client, updateFunc)
		Expect(err).To(BeNil())
		Expect(len(cluster.AKSConfig.NodePools)).Should(BeNumerically("==", initialNPCount+1))
		err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
		Expect(err).To(BeNil())
		Eventually(func() int {
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			return len(cluster.AKSStatus.UpstreamSpec.NodePools)
		}, "5m", "5s").Should(BeNumerically("==", initialNPCount+1))
	})

	By("Deleting the nodepool", func() {
		cluster, err = helper.DeleteNodePool(cluster, client, true, true)
		Expect(err).To(BeNil())
	})

	By("Scaling the nodepool", func() {
		cluster, err = helper.ScaleNodePool(cluster, client, 2, true, true)
		Expect(err).To(BeNil())
	})

}
