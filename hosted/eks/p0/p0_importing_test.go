package p0_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	management "github.com/rancher/rancher/tests/framework/clients/rancher/generated/management/v3"
	"github.com/rancher/rancher/tests/framework/extensions/clusters"
	nodestat "github.com/rancher/rancher/tests/framework/extensions/nodes"
	"github.com/rancher/rancher/tests/framework/extensions/workloads/pods"
	namegen "github.com/rancher/rancher/tests/framework/pkg/namegenerator"

	"github.com/rancher/hosted-providers-e2e/hosted/eks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("P0Importing", func() {
	var (
		clusterName string
		ctx         helpers.Context
		region      = os.Getenv("EKS_REGION")
		k8sVersion  = "1.26"
		increaseBy  = 1
	)
	var _ = BeforeEach(func() {
		clusterName = namegen.AppendRandomString("ekshostcluster")
		ctx = helpers.CommonBeforeSuite("eks")
	})

	When("a cluster is imported", func() {
		var cluster *management.Cluster

		BeforeEach(func() {
			var err error
			err = helper.CreateEKSClusterOnAWS(region, clusterName, k8sVersion, "1")
			Expect(err).To(BeNil())
			cluster, err = helper.ImportEKSHostedCluster(ctx.RancherClient, clusterName, ctx.CloudCred.ID, false, false, false, false, map[string]string{})
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherClient)
			Expect(err).To(BeNil())
			// Workaround to add new Nodegroup till https://github.com/rancher/aks-operator/issues/251 is fixed
			cluster.EKSConfig = cluster.EKSStatus.UpstreamSpec
		})
		AfterEach(func() {
			err := helper.DeleteEKSHostCluster(cluster, ctx.RancherClient)
			Expect(err).To(BeNil())
			err = helper.DeleteEKSClusterOnAWS(region, clusterName)
			Expect(err).To(BeNil())
			// TODO: Force delete EKS cluster
		})

		It("should successfully import the cluster", func() {

			By("checking cluster name is same", func() {
				Expect(cluster.Name).To(BeEquivalentTo(clusterName))
			})

			By("checking service account token secret", func() {
				success, err := clusters.CheckServiceAccountTokenSecret(ctx.RancherClient, clusterName)
				Expect(err).To(BeNil())
				Expect(success).To(BeTrue())
			})

			By("checking all management nodes are ready", func() {
				err := nodestat.AllManagementNodeReady(ctx.RancherClient, cluster.ID, helpers.Timeout)
				Expect(err).To(BeNil())
			})

			By("checking all pods are ready", func() {
				podErrors := pods.StatusPods(ctx.RancherClient, cluster.ID)
				Expect(podErrors).To(BeEmpty())
			})

		})

		Context("Upgrading K8s version", func() {
			var upgradeToVersion *string
			BeforeEach(func() {
				versions, err := helper.ListEKSAvailableVersions(ctx.RancherClient, cluster.ID)
				Expect(err).To(BeNil())
				Expect(versions).ToNot(BeEmpty())
				upgradeToVersion = &versions[0]
			})

			It("should be able to upgrade k8s version of the cluster", func() {
				By("upgrading the ControlPlane", func() {
					var err error
					cluster, err = helper.UpgradeClusterKubernetesVersion(cluster, upgradeToVersion, ctx.RancherClient)
					Expect(err).To(BeNil())
					err = clusters.WaitClusterToBeUpgraded(ctx.RancherClient, cluster.ID)
					Expect(err).To(BeNil())
					Expect(cluster.EKSConfig.KubernetesVersion).To(BeEquivalentTo(upgradeToVersion))
				})

				By("upgrading the NodeGroups", func() {
					var err error
					cluster, err = helper.UpgradeNodeKubernetesVersion(cluster, upgradeToVersion, ctx.RancherClient)
					Expect(err).To(BeNil())
					// TODO Does not upgrades version, since using custom LT
					// err = clusters.WaitClusterToBeUpgraded(ctx.RancherClient, cluster.ID)
					// Expect(err).To(BeNil())
					// for _, ng := range cluster.EKSConfig.NodeGroups {
					// 	Expect(ng.Version).To(BeEquivalentTo(upgradeToVersion))
					// }
				})
			})
		})

		It("should be possible to add or delete the NodeGroups", func() {
			currentNodeGroupNumber := len(cluster.EKSConfig.NodeGroups)

			By("adding a NodeGroup/s", func() {
				var err error
				cluster, err = helper.AddNodeGroup(cluster, increaseBy, ctx.RancherClient)
				Expect(err).To(BeNil())
				err = clusters.WaitClusterToBeUpgraded(ctx.RancherClient, cluster.ID)
				Expect(err).To(BeNil())
				Expect(len(cluster.EKSConfig.NodeGroups)).To(BeNumerically("==", currentNodeGroupNumber+1))
			})
			By("deleting the NodeGroup", func() {
				var err error
				cluster, err = helper.DeleteNodeGroup(cluster, ctx.RancherClient)
				Expect(err).To(BeNil())
				err = clusters.WaitClusterToBeUpgraded(ctx.RancherClient, cluster.ID)
				Expect(err).To(BeNil())
				Expect(len(cluster.EKSConfig.NodeGroups)).To(BeNumerically("==", currentNodeGroupNumber))

			})

		})
		It("should be possible to scale up/down the NodeGroup", func() {
			initialNodeCount := *cluster.EKSConfig.NodeGroups[0].DesiredSize

			By("scaling up the NodeGroup", func() {
				var err error
				cluster, err = helper.ScaleNodeGroup(cluster, ctx.RancherClient, initialNodeCount+1)
				Expect(err).To(BeNil())
				err = clusters.WaitClusterToBeUpgraded(ctx.RancherClient, cluster.ID)
				Expect(err).To(BeNil())
				for i := range cluster.EKSConfig.NodeGroups {
					Expect(*cluster.EKSConfig.NodeGroups[i].DesiredSize).To(BeNumerically("==", initialNodeCount+1))
				}
			})

			By("scaling down the NodeGroup", func() {
				var err error
				cluster, err = helper.ScaleNodeGroup(cluster, ctx.RancherClient, initialNodeCount)
				Expect(err).To(BeNil())
				err = clusters.WaitClusterToBeUpgraded(ctx.RancherClient, cluster.ID)
				Expect(err).To(BeNil())
				for i := range cluster.EKSConfig.NodeGroups {
					Expect(*cluster.EKSConfig.NodeGroups[i].DesiredSize).To(BeNumerically("==", initialNodeCount))
				}
			})
		})

	})

})
