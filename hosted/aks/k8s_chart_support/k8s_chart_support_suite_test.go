package k8s_chart_support_test

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
	. "github.com/rancher-sandbox/qase-ginkgo"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	"github.com/rancher/hosted-providers-e2e/hosted/aks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var (
	ctx                     helpers.Context
	clusterName, k8sVersion string
	testCaseID              int64
	location                = helpers.GetAKSLocation()
)

func TestK8sChartSupport(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "K8sChartSupport Suite")
}

var _ = BeforeSuite(func() {
	Expect(helpers.Kubeconfig).ToNot(BeEmpty())

	By("Adding the necessary chart repos", func() {
		helpers.AddRancherCharts()
	})

})

var _ = BeforeEach(func() {
	var err error
	ctx = helpers.CommonBeforeSuite(helpers.Provider)
	clusterName = namegen.AppendRandomString(helpers.ClusterNamePrefix)
	k8sVersion, err = helper.GetK8sVersion(ctx.RancherClient, ctx.CloudCred.ID, location)
	Expect(err).To(BeNil())
	Expect(k8sVersion).ToNot(BeEmpty())
	GinkgoLogr.Info(fmt.Sprintf("Using AKS version %s", k8sVersion))

})

var _ = AfterEach(func() {

	By("Uninstalling the existing operator charts", func() {
		helpers.UninstallOperatorCharts()
	})
})

var _ = ReportBeforeEach(func(report SpecReport) {
	// Reset case ID
	testCaseID = -1
})

var _ = ReportAfterEach(func(report SpecReport) {
	// Add result in Qase if asked
	Qase(testCaseID, report)
})

func commonchecks(ctx *helpers.Context, cluster *management.Cluster) {
	var originalChartVersion string

	By("checking the chart version", func() {
		originalChartVersion = helpers.GetCurrentOperatorChartVersion()
		Expect(originalChartVersion).ToNot(BeEmpty())
		GinkgoLogr.Info("Original chart version: " + originalChartVersion)
	})

	var downgradedVersion string
	By("obtaining a version to downgrade", func() {
		downgradedVersion = helpers.GetDowngradeOperatorChartVersion(originalChartVersion)
		Expect(downgradedVersion).ToNot(BeEmpty())
		GinkgoLogr.Info("Downgrading to version: " + downgradedVersion)
	})

	By("downgrading the chart version", func() {
		helpers.DowngradeProviderChart(downgradedVersion)
	})

	By("making a change to the cluster to validate functionality after chart downgrade", func() {
		initialNodeCount := cluster.NodeCount
		var err error
		cluster, err = helper.ScaleNodePool(cluster, ctx.RancherClient, initialNodeCount+1)
		Expect(err).To(BeNil())
		err = clusters.WaitClusterToBeUpgraded(ctx.RancherClient, cluster.ID)
		Expect(err).To(BeNil())
		for i := range cluster.AKSConfig.NodePools {
			Expect(*cluster.AKSConfig.NodePools[i].Count).To(BeNumerically("==", initialNodeCount+1))
		}
	})

	By("uninstalling the operator chart", func() {
		helpers.UninstallOperatorCharts()
	})

	By("making a change(adding a nodepool) to the cluster to re-install the operator and validating it is re-installed to the latest/original version", func() {
		currentNodePoolNumber := len(cluster.AKSConfig.NodePools)
		var err error
		cluster, err = helper.AddNodePool(cluster, 1, ctx.RancherClient)
		Expect(err).To(BeNil())

		By("ensuring that the chart is re-installed to the latest/original version", func() {
			helpers.WaitUntilOperatorChartInstallation(originalChartVersion, "", 0)
		})

		// We do not use WaitClusterToBeUpgraded because it has been flaky here and times out
		Eventually(func() bool {
			return len(cluster.AKSConfig.NodePools) == currentNodePoolNumber+1
		}, tools.SetTimeout(5*time.Minute), 2*time.Second).Should(BeTrue())
	})
}
