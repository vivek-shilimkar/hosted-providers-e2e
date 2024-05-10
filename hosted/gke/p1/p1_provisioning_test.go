package p1_test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters/gke"
	"github.com/rancher/shepherd/pkg/config"

	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("P1Provisioning", func() {
	var cluster *management.Cluster
	var (
		originalConfig = new(management.GKEClusterConfigSpec)
	)

	BeforeEach(func() {
		config.LoadConfig(gke.GKEClusterConfigConfigurationFileKey, originalConfig)

		gkeConfig := new(management.GKEClusterConfigSpec)

		config.LoadAndUpdateConfig(gke.GKEClusterConfigConfigurationFileKey, gkeConfig, func() {
			gkeConfig.ProjectID = project
			gkeConfig.Zone = zone
			labels := helper.GetLabels()
			gkeConfig.Labels = &labels
			gkeConfig.KubernetesVersion = &k8sVersion
			for _, np := range gkeConfig.NodePools {
				np.Version = &k8sVersion
			}
		})
	})
	AfterEach(func() {
		config.UpdateConfig(gke.GKEClusterConfigConfigurationFileKey, originalConfig)
	})

	Context("Provisioning a cluster with invalid config", func() {

		AfterEach(func() {
			if ctx.ClusterCleanup {
				if cluster != nil {
					err := helper.DeleteGKEHostCluster(cluster, ctx.RancherClient)
					Expect(err).To(BeNil())
				}
			}
		})

		It("should fail to provision a cluster when creating cluster with invalid name", func() {
			testCaseID = 36
			var err error
			cluster, err = gke.CreateGKEHostedCluster(ctx.RancherClient, "@!invalid-gke-name-@#", ctx.CloudCred.ID, false, false, false, false, map[string]string{})
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("InvalidFormat"))
		})

		It("should fail to provision a cluster with invalid nodepool name", func() {
			testCaseID = 37
			gkeConfig := new(management.GKEClusterConfigSpec)
			config.LoadAndUpdateConfig(gke.GKEClusterConfigConfigurationFileKey, gkeConfig, func() {
				for _, np := range gkeConfig.NodePools {
					*np.Name = "#@invalid-nodepoolname-$$$$"
				}
			})

			var err error
			cluster, err = gke.CreateGKEHostedCluster(ctx.RancherClient, clusterName, ctx.CloudCred.ID, false, false, false, false, map[string]string{})
			Expect(err).To(BeNil())

			Eventually(func() bool {
				clusterState, err := ctx.RancherClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				for _, condition := range clusterState.Conditions {
					if strings.Contains(condition.Message, "Invalid value for field \"node_pool.name\"") {
						return true
					}
				}
				return false
			}, "60s", "2s").Should(BeTrue())

		})

		It("should fail to provision a cluster with no nodepools", func() {
			testCaseID = 27
			gkeConfig := new(management.GKEClusterConfigSpec)
			config.LoadAndUpdateConfig(gke.GKEClusterConfigConfigurationFileKey, gkeConfig, func() {
				gkeConfig.NodePools = nil
			})

			var err error
			cluster, err = gke.CreateGKEHostedCluster(ctx.RancherClient, clusterName, ctx.CloudCred.ID, false, false, false, false, map[string]string{})
			Expect(err).To(BeNil())

			Eventually(func() bool {
				clusterState, err := ctx.RancherClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				for _, condition := range clusterState.Conditions {
					if strings.Contains(condition.Message, "Cluster.initial_node_count must be greater than zero") {
						return true
					}
				}
				return false
			}, "60s", "2s").Should(BeTrue())

		})
	})
	When("a cluster is created", func() {

		BeforeEach(func() {
			var err error
			cluster, err = gke.CreateGKEHostedCluster(ctx.RancherClient, clusterName, ctx.CloudCred.ID, false, false, false, false, map[string]string{})
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherClient)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			if ctx.ClusterCleanup {
				err := helper.DeleteGKEHostCluster(cluster, ctx.RancherClient)
				Expect(err).To(BeNil())
			} else {
				fmt.Println("Skipping downstream cluster deletion: ", clusterName)
			}
		})

		It("should be able to update mutable parameter loggingService and monitoringService", func() {
			testCaseID = 28
			By("disabling the services", func() {
				updateLoggingAndMonitoringServiceCheck(ctx, cluster, "none", "none")
			})
			By("enabling the services", func() {
				updateLoggingAndMonitoringServiceCheck(ctx, cluster, "monitoring.googleapis.com/kubernetes", "logging.googleapis.com/kubernetes")
			})
		})

		It("should be able to update autoscaling", func() {
			testCaseID = 29
			By("enabling autoscaling", func() {
				updateAutoScaling(ctx, cluster, true)
			})
			By("disabling autoscaling", func() {
				updateAutoScaling(ctx, cluster, false)
			})
		})

	})

})
