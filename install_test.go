/*
Copyright © 2022 - 2024 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"

	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("Provision k3s cluster and Rancher", Label("install"), func() {
	k := kubectl.New()

	It("Install upstream k3s cluster", func() {
		By("Installing K3S", func() {
			helpers.InstallK3S(k, k3sVersion, proxy, proxyHost)
		})

		By("Installing CertManager", func() {
			helpers.InstallCertManager(k, proxy, proxyHost)
		})

		if skipInstallRancher != "true" {
			By("Installing Rancher Manager", func() {
				helpers.InstallRancherManager(k, rancherHostname, rancherChannel, rancherVersion, rancherHeadVersion, proxy, nightlyChart)
			})

			By("Checking Rancher Deployments", func() {
				helpers.CheckRancherDeployments(k)
			})
		} else {
			GinkgoLogr.Info("Skipping Rancher Manager installation; SKIP_RANCHER_INSTALL=\"true\"")
		}

		if nightlyChart == "enabled" {
			By(fmt.Sprintf("Install nightly rancher-%s-operator via Helm", providerOperator), func() {
				// Get the current date to use as the build date
				buildDate := time.Now().Format("20060102")

				RunHelmCmdWithRetry("upgrade", "--install", "rancher-"+providerOperator+"-operator-crds",
					"oci://ghcr.io/rancher/rancher-"+providerOperator+"-operator-crd-chart/rancher-"+providerOperator+"-operator-crd",
					"--version", buildDate)
				RunHelmCmdWithRetry("upgrade", "--install", "rancher-"+providerOperator+"-operator",
					"oci://ghcr.io/rancher/rancher-"+providerOperator+"-operator-chart/rancher-"+providerOperator+"-operator",
					"--version", buildDate, "--namespace", "cattle-system")
			})
		}
	})
})
