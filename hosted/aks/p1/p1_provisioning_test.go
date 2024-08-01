package p1_test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters/aks"
	"k8s.io/utils/pointer"

	"github.com/rancher/hosted-providers-e2e/hosted/aks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("P1Provisioning", func() {
	var cluster *management.Cluster
	var k8sVersion string
	BeforeEach(func() {
		var err error
		k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, ctx.CloudCred.ID, location, false)
		Expect(err).NotTo(HaveOccurred())
		GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))
	})
	AfterEach(func() {
		if ctx.ClusterCleanup && cluster != nil {
			err := helper.DeleteAKSHostCluster(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		} else {
			fmt.Println("Skipping downstream cluster deletion: ", clusterName)
		}
	})
	When("a cluster with invalid config is created", func() {
		It("should fail to create a cluster with 0 nodecount", func() {
			testCaseID = 186
			updateFunc := func(aksConfig *aks.ClusterConfig) {
				nodepools := *aksConfig.NodePools
				for i := range nodepools {
					nodepools[i].NodeCount = pointer.Int64(0)
				}
				aksConfig.NodePools = &nodepools
			}
			var err error
			cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCred.ID, k8sVersion, location, updateFunc)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).NotTo(HaveOccurred())
				return cluster.Transitioning == "error" && strings.Contains(cluster.TransitioningMessage, "agentPoolProfile.count was 0. It must be greater or equal to minCount:1 and less than or equal to maxCount:1000")
			}, "1m", "2s").Should(BeTrue())
		})
		It("should fail to create a cluster with 0 nodepool", func() {
			testCaseID = 187
			updateFunc := func(aksConfig *aks.ClusterConfig) {
				aksConfig.NodePools = &[]aks.NodePool{}
			}
			var err error
			cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCred.ID, k8sVersion, location, updateFunc)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() bool {
				cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).NotTo(HaveOccurred())
				return cluster.Transitioning == "error" && cluster.TransitioningMessage == "at least one NodePool with mode System is required"
			}, "1m", "2s").Should(BeTrue())
		})
	})
	When("a cluster is created", func() {
		BeforeEach(func() {
			var err error
			cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCred.ID, k8sVersion, location, nil)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should be able to update autoscaling", func() {
			testCaseID = 176
			updateAutoScaling(cluster, ctx.RancherAdminClient)
		})
	})
})
