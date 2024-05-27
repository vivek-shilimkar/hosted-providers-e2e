package k8s_chart_support_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters/gke"
	"github.com/rancher/shepherd/pkg/config"

	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("K8sChartSupportProvisioning", func() {

	var (
		cluster *management.Cluster
	)
	BeforeEach(func() {
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

		var err error
		cluster, err = gke.CreateGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCred.ID, false, false, false, false, map[string]string{})
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if ctx.ClusterCleanup {
			err := helper.DeleteGKEHostCluster(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		} else {
			fmt.Println("Skipping downstream cluster deletion: ", clusterName)
		}
	})

	It("should successfully test k8s chart support provisioning", func() {
		testCaseID = 63 // Report to Qase
		commonChartSupport(ctx.RancherAdminClient, cluster)
	})

})
