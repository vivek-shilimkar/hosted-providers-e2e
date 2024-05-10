package k8s_chart_support_upgrade_test

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

var _ = Describe("K8sChartSupportUpgradeImport", func() {
	var cluster *management.Cluster

	BeforeEach(func() {
		var err error
		err = helper.CreateAKSClusterOnAzure(location, clusterName, k8sVersion, "1")
		Expect(err).To(BeNil())

		aksConfig := new(helper.ImportClusterConfig)
		config.LoadAndUpdateConfig(aks.AKSClusterConfigConfigurationFileKey, aksConfig, func() {
			aksConfig.ResourceGroup = clusterName
			aksConfig.ResourceLocation = location
			aksConfig.Tags = helper.GetTags()
		})

		cluster, err = helper.ImportAKSHostedCluster(ctx.RancherClient, clusterName, ctx.CloudCred.ID, false, false, false, false, map[string]string{})
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherClient)
		Expect(err).To(BeNil())
		// Workaround to add new Nodegroup till https://github.com/rancher/aks-operator/issues/251 is fixed
		cluster.AKSConfig = cluster.AKSStatus.UpstreamSpec
	})
	AfterEach(func() {
		if ctx.ClusterCleanup {
			err := helper.DeleteAKSHostCluster(cluster, ctx.RancherClient)
			Expect(err).To(BeNil())
			err = helper.DeleteAKSClusteronAzure(clusterName)
			Expect(err).To(BeNil())
		} else {
			fmt.Println("Skipping downstream cluster deletion: ", clusterName)
		}
	})

	It("should successfully test k8s chart support import in an upgrade scenario", func() {
		GinkgoLogr.Info(fmt.Sprintf("Testing K8s %s chart support for import on Rancher upgraded from %s to %s", helpers.K8sUpgradedMinorVersion, helpers.RancherVersion, helpers.RancherUpgradeVersion))

		testCaseID = 253 // Report to Qase
		commonchecks(&ctx, cluster, clusterName, helpers.RancherUpgradeVersion, helpers.RancherHostname, helpers.K8sUpgradedMinorVersion)
	})

})
