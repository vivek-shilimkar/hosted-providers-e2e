package k8s_chart_support_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters/aks"
	"github.com/rancher/shepherd/pkg/config"

	"github.com/rancher/hosted-providers-e2e/hosted/aks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("K8sChartSupportProvisioning", func() {
	var (
		cluster *management.Cluster
	)
	BeforeEach(func() {
		var err error
		aksConfig := new(aks.ClusterConfig)
		config.LoadAndUpdateConfig(aks.AKSClusterConfigConfigurationFileKey, aksConfig, func() {
			aksConfig.ResourceGroup = clusterName
			dnsPrefix := clusterName + "-dns"
			aksConfig.DNSPrefix = &dnsPrefix
			aksConfig.ResourceLocation = location
			aksConfig.Tags = helper.GetTags()
			aksConfig.KubernetesVersion = &k8sVersion
		})
		cluster, err = aks.CreateAKSHostedCluster(ctx.RancherClient, clusterName, ctx.CloudCred.ID, false, false, false, false, map[string]string{})
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherClient)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if ctx.ClusterCleanup {
			err := helper.DeleteAKSHostCluster(cluster, ctx.RancherClient)
			Expect(err).To(BeNil())
		} else {
			fmt.Println("Skipping downstream cluster deletion: ", clusterName)
		}
	})

	It("should successfully test k8s chart support provisioning", func() {
		testCaseID = 252 // Report to Qase
		commonchecks(&ctx, cluster)

	})

})
