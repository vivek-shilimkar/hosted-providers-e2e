package p1_test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"

	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("P1Importing", func() {

	var _ = BeforeEach(func() {
		var err error
		k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, project, ctx.CloudCred.ID, zone, "", false)
		Expect(err).To(BeNil())
		GinkgoLogr.Info(fmt.Sprintf("Using kubernetes version %s for cluster %s", k8sVersion, clusterName))
	})

	When("a cluster is created", func() {
		var cluster *management.Cluster

		BeforeEach(func() {
			var err error
			err = helper.CreateGKEClusterOnGCloud(zone, clusterName, project, k8sVersion)
			Expect(err).To(BeNil())

			cluster, err = helper.ImportGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCred.ID, zone, project)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
			// Workaround to add new Nodegroup till https://github.com/rancher/aks-operator/issues/251 is fixed
			cluster.GKEConfig = cluster.GKEStatus.UpstreamSpec
		})

		AfterEach(func() {
			if ctx.ClusterCleanup && cluster != nil {
				err := helper.DeleteGKEHostCluster(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
				err = helper.DeleteGKEClusterOnGCloud(zone, project, clusterName)
				Expect(err).To(BeNil())
			} else {
				fmt.Println("Skipping downstream cluster deletion: ", clusterName)
			}
		})

		It("should fail to reimport an imported cluster", func() {
			testCaseID = 49
			_, err := helper.ImportGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCred.ID, zone, project)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("cluster already exists for GKE cluster [%s] in zone [%s]", clusterName, zone)))
		})

		It("should be able to update mutable parameter", func() {
			testCaseID = 52
			By("disabling the services", func() {
				updateLoggingAndMonitoringServiceCheck(ctx, cluster, "none", "none")
			})
			By("enabling the services", func() {
				updateLoggingAndMonitoringServiceCheck(ctx, cluster, "monitoring.googleapis.com/kubernetes", "logging.googleapis.com/kubernetes")
			})
		})

		It("should be able to update autoscaling", func() {
			testCaseID = 53
			By("enabling autoscaling", func() {
				updateAutoScaling(ctx, cluster, true)
			})
			By("disabling autoscalling", func() {
				updateAutoScaling(ctx, cluster, false)
			})
		})

		It("should be able to reimport a deleted cluster", func() {
			testCaseID = 57
			err := helper.DeleteGKEHostCluster(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
			clusterID := cluster.ID
			Eventually(func() error {
				_, err := ctx.RancherAdminClient.Management.Cluster.ByID(clusterID)
				return err
			}, "10s", "1s").ShouldNot(BeNil())
			cluster, err = helper.ImportGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCred.ID, zone, project)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("should successfully add a windows nodepool", func() {
			testCaseID = 54
			var err error
			_, err = helper.AddNodePool(cluster, ctx.RancherAdminClient, 1, "WINDOWS_LTSC_CONTAINERD", true, true)
			Expect(err).To(BeNil())
		})

		It("updating a cluster to all windows nodepool should fail", func() {
			testCaseID = 264
			_, err := helper.UpdateCluster(cluster, ctx.RancherAdminClient, func(upgradedCluster *management.Cluster) {
				updateNodePoolsList := cluster.GKEConfig.NodePools
				for i := 0; i < len(updateNodePoolsList); i++ {
					updateNodePoolsList[i].Config.ImageType = "WINDOWS_LTSC_CONTAINERD"
				}

				upgradedCluster.GKEConfig.NodePools = updateNodePoolsList
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least 1 Linux node pool is required"))
		})
	})

	When("a cluster is created with at least 2 node pools", func() {
		var cluster *management.Cluster

		BeforeEach(func() {
			var err error
			err = helper.CreateGKEClusterOnGCloud(zone, clusterName, project, k8sVersion)
			Expect(err).To(BeNil())

			err = helper.AddNodePoolOnGCloud(clusterName, zone, project, "")
			Expect(err).To(BeNil())

			cluster, err = helper.ImportGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCred.ID, zone, project)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
			// Workaround to add new Nodegroup till https://github.com/rancher/aks-operator/issues/251 is fixed
			cluster.GKEConfig = cluster.GKEStatus.UpstreamSpec
		})

		AfterEach(func() {
			if ctx.ClusterCleanup && cluster != nil {
				err := helper.DeleteGKEHostCluster(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
				err = helper.DeleteGKEClusterOnGCloud(zone, project, clusterName)
				Expect(err).To(BeNil())
			} else {
				fmt.Println("Skipping downstream cluster deletion: ", clusterName)
			}
		})

		It("for a given NodePool with a non-windows imageType, updating it to a windows imageType should fail", func() {
			testCaseID = 55
			var err error
			cluster, err = helper.UpdateCluster(cluster, ctx.RancherAdminClient, func(upgradedCluster *management.Cluster) {
				updateNodePoolsList := cluster.GKEConfig.NodePools
				updateNodePoolsList[0].Config.ImageType = "WINDOWS_LTSC_CONTAINERD"

				upgradedCluster.GKEConfig.NodePools = updateNodePoolsList
			})

			Eventually(func() bool {
				cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				return cluster.Transitioning == "error" && strings.Contains(cluster.TransitioningMessage, "Node pools cannot be upgraded between Windows and non-Windows image families")
			}, "30s", "2s").Should(BeTrue())
		})
	})

})
