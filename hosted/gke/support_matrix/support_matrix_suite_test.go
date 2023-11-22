package support_matrix_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"

	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var (
	availableVersionList []string
	ctx                  helpers.Context
)

func TestSupportMatrix(t *testing.T) {
	RegisterFailHandler(Fail)
	ctx = helpers.CommonBeforeSuite("gke")
	var err error
	availableVersionList, err = helper.ListSingleVariantGKEAvailableVersions(ctx.RancherClient, "container-project-qa", ctx.CloudCred.ID, "", "us-central1")
	Expect(err).To(BeNil())
	RunSpecs(t, "SupportMatrix Suite")
}
