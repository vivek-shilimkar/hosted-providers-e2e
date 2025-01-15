/*
Copyright © 2022 - 2025 SUSE LLC

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

package backup_test

import (
	"os"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

const (
	backupResourceName  = "hp-backup"
	restoreResourceName = "hp-restore"
)

var _ = Describe("BackupRestore", func() {
	// Create kubectl context
	// Default timeout is too small, so New() cannot be used
	k := &kubectl.Kubectl{
		Namespace:    "",
		PollTimeout:  tools.SetTimeout(300 * time.Second),
		PollInterval: 500 * time.Millisecond,
	}

	It("Do a full backup/restore test", func() {
		testCaseID = 246 // Report to Qase
		By("Checking hosted cluster is ready", func() {
			helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)
		})

		By("Performing a backup", func() {
			backupFile = helpers.ExecuteBackup(k, backupResourceName)
		})

		By("Perform restore pre-requisites: Uninstalling k3s", func() {
			out, err := exec.Command("k3s-uninstall.sh").CombinedOutput()
			Expect(err).To(Not(HaveOccurred()), out)
		})

		By("Perform restore pre-requisites: Getting k3s ready", func() {
			helpers.InstallK3S(k, k3sVersion, "none", "none")
		})

		By("Performing a restore", func() {
			helpers.ExecuteRestore(k, restoreResourceName, backupFile)
		})

		By("Performing post migration installations: Installing CertManager", func() {
			helpers.InstallCertManager(k, "none", "none")
		})

		By("Performing post migration installations: Installing Rancher Manager", func() {
			rancherChannel, rancherVersion, rancherHeadVersion := helpers.GetRancherVersions()
			helpers.InstallRancherManager(k, os.Getenv("RANCHER_HOSTNAME"), rancherChannel, rancherVersion, rancherHeadVersion, "none", "none")
		})

		By("Performing post migration installations: Checking Rancher Deployments", func() {
			helpers.CheckRancherDeployments(k)
		})

		By("Checking hosted cluster is able to be modified", func() {
			restoreNodesChecks(cluster, ctx.RancherAdminClient, clusterName)
		})
	})
})
