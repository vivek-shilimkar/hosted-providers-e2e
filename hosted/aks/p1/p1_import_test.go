package p1_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"

	"github.com/rancher/hosted-providers-e2e/hosted/aks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("P1Import", func() {
	When("a cluster is created", func() {
		var cluster *management.Cluster
		BeforeEach(func() {
			k8sVersion, err := helper.GetK8sVersion(ctx.RancherAdminClient, ctx.CloudCred.ID, location, false)
			Expect(err).NotTo(HaveOccurred())
			GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))

			err = helper.CreateAKSClusterOnAzure(location, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())

			cluster, err = helper.ImportAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCred.ID, location, helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			if ctx.ClusterCleanup && cluster != nil {
				err := helper.DeleteAKSHostCluster(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
				err = helper.DeleteAKSClusteronAzure(clusterName)
				Expect(err).To(BeNil())
			} else {
				fmt.Println("Skipping downstream cluster deletion: ", clusterName)
			}
		})

		It("should be able to update autoscaling", func() {
			testCaseID = 266
			updateAutoScaling(cluster, ctx.RancherAdminClient)
		})
	})

})
