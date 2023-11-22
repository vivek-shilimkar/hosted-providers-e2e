package support_matrix_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"

	"github.com/rancher/hosted-providers-e2e/hosted/aks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var (
	availableVersionList []string
	ctx                  helpers.Context
)

func TestSupportMatrix(t *testing.T) {
	RegisterFailHandler(Fail)
	ctx = helpers.CommonBeforeSuite("aks")
	var err error
	availableVersionList, err = helper.ListSingleVariantAKSAvailableVersions(ctx.RancherClient, ctx.CloudCred.ID, "eastus")
	Expect(err).To(BeNil())
	RunSpecs(t, "SupportMatrix Suite")
}
