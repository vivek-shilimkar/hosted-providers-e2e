package p1_test

import (
	"fmt"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters"
	"github.com/rancher/shepherd/extensions/clusters/eks"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	"k8s.io/utils/pointer"

	"github.com/rancher/hosted-providers-e2e/hosted/eks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("P1Provisioning", func() {
	var k8sVersion string
	var _ = BeforeEach(func() {
		var err error
		k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, false)
		Expect(err).To(BeNil())
		GinkgoLogr.Info(fmt.Sprintf("While provisioning, using kubernetes version %s for cluster %s", k8sVersion, clusterName))
	})

	AfterEach(func() {
		if ctx.ClusterCleanup {
			if cluster != nil && cluster.ID != "" {
				GinkgoLogr.Info(fmt.Sprintf("Cleaning up resource cluster: %s %s", cluster.Name, cluster.ID))
				err := helper.DeleteEKSHostCluster(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
			}
		} else {
			fmt.Println("Skipping downstream cluster deletion: ", clusterName)
		}
	})

	Context("Provisioning/Editing a cluster with invalid config", func() {

		It("should error out to provision a cluster with no nodegroups", func() {
			testCaseID = 141

			updateFunc := func(clusterConfig *eks.ClusterConfig) {
				*clusterConfig.NodeGroupsConfig = nil
			}

			var err error
			cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, updateFunc)
			Expect(err).To(BeNil())

			Eventually(func() bool {
				cluster, err := ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				return cluster.Transitioning == "error" && strings.Contains(cluster.TransitioningMessage, "Cluster must have at least one managed nodegroup or one self-managed node")
			}, "10m", "30s").Should(BeTrue())

		})

		It("should fail to provision a cluster with duplicate nodegroup names", func() {
			testCaseID = 255

			var err error
			updateFunc := func(clusterConfig *eks.ClusterConfig) {
				var updatedNodeGroupsList []eks.NodeGroupConfig
				*clusterConfig, err = helper.AddNodeGroupToConfig(*clusterConfig, 2)
				Expect(err).To(BeNil())

				for _, ng := range *clusterConfig.NodeGroupsConfig {
					ng.NodegroupName = pointer.String("duplicate")
					updatedNodeGroupsList = append([]eks.NodeGroupConfig{ng}, updatedNodeGroupsList...)
				}
				*clusterConfig.NodeGroupsConfig = updatedNodeGroupsList
			}
			cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, updateFunc)
			Expect(err).To(BeNil())

			Eventually(func() bool {
				cluster, err := ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				// checking for both the messages since different operator version shows different messages. To be removed once the message is updated.
				// New Message: NodePool names must be unique within the [c-dnzzk] cluster to avoid duplication
				return cluster.Transitioning == "error" && (strings.Contains(cluster.TransitioningMessage, "is not unique within the cluster") || strings.Contains(cluster.TransitioningMessage, "names must be unique"))
			}, "1m", "3s").Should(BeTrue())
		})

		It("Fail to create cluster with different k8s versions on control plane and on nodegroup", func() {
			testCaseID = 127

			k8sVersions, err := helper.ListEKSAllVersions(ctx.RancherAdminClient)
			Expect(err).To(BeNil())

			cpK8sVersion := k8sVersions[1]
			ngK8sVersion := k8sVersions[0]

			updateFunc := func(clusterConfig *eks.ClusterConfig) {
				var updatedNodeGroupsList []eks.NodeGroupConfig
				for _, ng := range *clusterConfig.NodeGroupsConfig {
					ng.Version = pointer.String(ngK8sVersion)
					updatedNodeGroupsList = append([]eks.NodeGroupConfig{ng}, updatedNodeGroupsList...)
				}
				*clusterConfig.NodeGroupsConfig = updatedNodeGroupsList
			}

			GinkgoLogr.Info(fmt.Sprintf("Kubernetes version %s for control plane and %s for nodegroup on cluster %s", cpK8sVersion, ngK8sVersion, clusterName))
			cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, cpK8sVersion, region, updateFunc)
			Expect(err).To(BeNil())

			Eventually(func() bool {
				cluster, err := ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				return cluster.Transitioning == "error" && strings.Contains(cluster.TransitioningMessage, "version must match cluster")
			}, "1m", "3s").Should(BeTrue())
		})

		It("Fail to create cluster with only Security groups", func() {
			testCaseID = 120

			sg := []string{namegen.AppendRandomString("sg-"), namegen.AppendRandomString("sg-")}
			updateFunc := func(clusterConfig *eks.ClusterConfig) {
				clusterConfig.SecurityGroups = sg
			}
			var err error
			cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, updateFunc)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("subnets must be provided if security groups are provided")))
		})

		It("Fail to update both Public/Private access as false and invalid values of the access", func() {
			testCaseID = 147 // also covers 146

			var err error
			cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, nil)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
			invalidEndpointCheck(cluster, ctx.RancherAdminClient)
			invalidAccessValuesCheck(cluster, ctx.RancherAdminClient)
		})
	})

	It("should successfully Provision EKS with secrets encryption (KMS)", func() {
		testCaseID = 149
		createFunc := func(clusterConfig *eks.ClusterConfig) {
			clusterConfig.KmsKey = pointer.String(os.Getenv("AWS_KMS_KEY"))
		}
		var err error
		cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, createFunc)
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
		helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)

	})

	It("should successfully Provision EKS from Rancher with Enabled GPU feature", func() {
		if helpers.SkipTest {
			Skip("Skipping test for v2.8, v2.9 ...")
		}

		testCaseID = 274
		var gpuNodeName = "gpuenabled"
		createFunc := func(clusterConfig *eks.ClusterConfig) {
			nodeGroups := *clusterConfig.NodeGroupsConfig
			gpuNG := nodeGroups[0]
			gpuNG.Gpu = pointer.Bool(true)
			gpuNG.NodegroupName = &gpuNodeName
			gpuNG.InstanceType = pointer.String("p2.xlarge")
			nodeGroups = append(nodeGroups, gpuNG)
			clusterConfig.NodeGroupsConfig = &nodeGroups
		}
		var err error
		cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, createFunc)
		Expect(err).To(BeNil())

		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())

		helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)
		var amiID string
		amiID, err = helper.GetFromEKS(region, clusterName, "nodegroup", ".[].ImageID", "--name", gpuNodeName)
		Expect(err).To(BeNil())
		GinkgoLogr.Info(fmt.Sprintf("Used AMI for GPU enabled nodegroup in EKS cluster: %s", amiID))
		Expect(amiID).To(Or(Equal("AL2_x86_64_GPU"), Equal("AL2023_x86_64_NVIDIA")))
	})

	XIt("Deploy a cluster with Public/Priv access then disable Public access", func() {
		// https://github.com/rancher/eks-operator/issues/752#issuecomment-2609144199
		testCaseID = 151
		createFunc := func(clusterConfig *eks.ClusterConfig) {
			clusterConfig.PublicAccess = pointer.Bool(true)
			clusterConfig.PrivateAccess = pointer.Bool(true)
		}
		var err error
		cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, createFunc)
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())

		cluster, err = helper.UpdateAccess(cluster, ctx.RancherAdminClient, false, true, true)
		Expect(err).To(BeNil())

		helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)
	})

	Context("Upgrade testing", func() {
		var upgradeToVersion string

		BeforeEach(func() {
			if helpers.SkipUpgradeTests {
				Skip(helpers.SkipUpgradeTestsLog)
			}

			var err error
			k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, true)
			Expect(err).To(BeNil())
			upgradeToVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, false)
			Expect(err).To(BeNil())
			GinkgoLogr.Info(fmt.Sprintf("While provisioning, using kubernetes version %s for cluster %s", k8sVersion, clusterName))
		})

		When("a cluster is created", func() {

			BeforeEach(func() {
				var err error
				cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, nil)
				Expect(err).To(BeNil())
				cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
			})

			It("Upgrade version of node group only", func() {
				testCaseID = 126
				upgradeNodeKubernetesVersionGTCPCheck(cluster, ctx.RancherAdminClient, upgradeToVersion)
			})

			It("Update k8s version of cluster and add node groups", func() {
				testCaseID = 125
				upgradeCPAndAddNgCheck(cluster, ctx.RancherAdminClient, upgradeToVersion)
			})

			// eks-operator/issues/752
			XIt("should successfully update a cluster while it is still in updating state", func() {
				testCaseID = 148
				updateClusterInUpdatingState(cluster, ctx.RancherAdminClient, upgradeToVersion)
			})
		})

		When("a cluster is created with multiple nodegroups", func() {
			BeforeEach(func() {
				var err error
				createFunc := func(clusterConfig *eks.ClusterConfig) {
					*clusterConfig, err = helper.AddNodeGroupToConfig(*clusterConfig, 4)
					Expect(err).To(BeNil())
				}
				cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, createFunc)
				Expect(err).To(BeNil())
				cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
			})

			It("Update k8s version of node groups - sequential & simultaneous upgrade of multiple node groups", func() {
				testCaseID = 153
				var err error
				cluster, err = helper.UpgradeClusterKubernetesVersion(cluster, upgradeToVersion, ctx.RancherAdminClient, true)
				Expect(err).To(BeNil())

				// upgrade 2 nodegroups simultaneously
				updateFunc := func(cluster *management.Cluster) {
					nodeGroups := cluster.EKSConfig.NodeGroups
					for i := 0; i <= 1; i++ {
						nodeGroups[i].Version = &upgradeToVersion
					}
				}
				cluster, err = helper.UpdateCluster(cluster, ctx.RancherAdminClient, updateFunc)
				Expect(err).To(BeNil())
				err = clusters.WaitClusterToBeUpgraded(ctx.RancherAdminClient, cluster.ID)
				Expect(err).To(BeNil())
				Eventually(func() bool {
					cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
					Expect(err).To(BeNil())
					nodeGroups := cluster.EKSStatus.UpstreamSpec.NodeGroups
					for i := 0; i <= 1; i++ {
						if nodeGroups[i].Version == nil || *nodeGroups[i].Version != upgradeToVersion {
							return false
						}
					}
					return true
				}, "10m", "7s").Should(BeTrue())

				// upgrade the remaining nodegroups
				cluster, err = helper.UpgradeNodeKubernetesVersion(cluster, upgradeToVersion, ctx.RancherAdminClient, true, true, false)
				Expect(err).To(BeNil())
			})
		})
	})

	When("a cluster is created", func() {

		BeforeEach(func() {
			var err error
			cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, nil)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("Update cluster logging types", func() {
			testCaseID = 128
			updateLoggingCheck(cluster, ctx.RancherAdminClient)
		})

		It("Update Tags and Labels", func() {
			testCaseID = 131
			updateTagsAndLabels(cluster, ctx.RancherAdminClient)
		})

		It("Update the cloud creds", func() {
			testCaseID = 109
			updateCloudCredentialsCheck(cluster, ctx.RancherAdminClient)
		})
	})
})
