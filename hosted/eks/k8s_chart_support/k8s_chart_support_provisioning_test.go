package k8s_chart_support_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters/eks"
	"github.com/rancher/shepherd/pkg/config"

	"github.com/rancher/hosted-providers-e2e/hosted/eks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("K8sChartSupportProvisioning", func() {
	var cluster *management.Cluster
	BeforeEach(func() {
		var err error
		eksConfig := new(eks.ClusterConfig)
		config.LoadAndUpdateConfig(eks.EKSClusterConfigConfigurationFileKey, eksConfig, func() {
			eksConfig.Region = region
			eksConfig.Tags = helper.GetTags()
			eksConfig.KubernetesVersion = &k8sVersion
		})
		cluster, err = eks.CreateEKSHostedCluster(ctx.RancherClient, clusterName, ctx.CloudCred.ID, false, false, false, false, map[string]string{})
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherClient)
		Expect(err).To(BeNil())
	})
	AfterEach(func() {
		// TODO Check if EKS cluster deleted on AWS
		if ctx.ClusterCleanup {
			err := helper.DeleteEKSHostCluster(cluster, ctx.RancherClient)
			Expect(err).To(BeNil())
		} else {
			fmt.Println("Skipping downstream cluster deletion: ", clusterName)
		}
	})

	It("should successfully test k8s chart support provisioning", func() {
		testCaseID = 317
		commonchecks(&ctx, cluster)
	})

})
