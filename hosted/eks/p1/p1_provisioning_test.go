package p1_test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters/eks"

	"github.com/rancher/hosted-providers-e2e/hosted/eks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
	"k8s.io/utils/pointer"
)

var _ = Describe("P1Provisioning", func() {
	var cluster *management.Cluster

	var _ = BeforeEach(func() {
		var err error
		k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, false)
		Expect(err).To(BeNil())
		GinkgoLogr.Info(fmt.Sprintf("While provisioning, using kubernetes version %s for cluster %s", k8sVersion, clusterName))
	})

	Context("Provisioning a cluster with invalid config", func() {

		AfterEach(func() {
			if ctx.ClusterCleanup && cluster != nil {
				if cluster != nil {
					err := helper.DeleteEKSHostCluster(cluster, ctx.RancherAdminClient)
					Expect(err).To(BeNil())
				}
			}
		})

		It("should error out to provision a cluster with no nodegroups", func() {
			testCaseID = 141

			updateFunc := func(clusterConfig *eks.ClusterConfig) {
				*clusterConfig.NodeGroupsConfig = nil
			}

			var err error
			cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCred.ID, k8sVersion, region, updateFunc)
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
			cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCred.ID, k8sVersion, region, updateFunc)
			Expect(err).To(BeNil())

			Eventually(func() bool {
				cluster, err := ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				return cluster.Transitioning == "error" && strings.Contains(cluster.TransitioningMessage, "node group names must be unique")
			}, "1m", "3s").Should(BeTrue())

		})
	})

	When("a cluster is created for upgrade", func() {

		BeforeEach(func() {
			var err error
			k8sVersion, err := helper.GetK8sVersion(ctx.RancherAdminClient, true)
			Expect(err).To(BeNil())
			GinkgoLogr.Info(fmt.Sprintf("While provisioning, using kubernetes version %s for cluster %s", k8sVersion, clusterName))

			cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCred.ID, k8sVersion, region, nil)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			if ctx.ClusterCleanup && cluster != nil {
				err := helper.DeleteEKSHostCluster(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
			} else {
				fmt.Println("Skipping downstream cluster deletion: ", clusterName)
			}
		})

		It("Upgrade version of node group only", func() {
			testCaseID = 126

			By("upgrading only the NodeGroups", func() {
				var err error
				upgradeToVersion, err := helper.GetK8sVersion(ctx.RancherAdminClient, false)
				Expect(err).To(BeNil())
				GinkgoLogr.Info("Upgrading Nodegroup's EKS version")
				cluster, err = helper.UpgradeNodeKubernetesVersion(cluster, upgradeToVersion, ctx.RancherAdminClient, false, false)
				Expect(err).To(BeNil())
			})

			// wait until the error is visible on the cluster
			Eventually(func() bool {
				cluster, err := ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				return cluster.Transitioning == "error" && strings.Contains(cluster.TransitioningMessage, "all nodegroup kubernetes versionsmust be equal to or one minor version lower than the cluster kubernetes version")
			}, "1m", "3s").Should(BeTrue())
		})
	})
})
