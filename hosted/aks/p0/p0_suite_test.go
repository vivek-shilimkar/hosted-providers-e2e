package p0_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestP0(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "P0 Suite")
}
