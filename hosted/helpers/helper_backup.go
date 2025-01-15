package helpers

import (
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"
	"github.com/rancher-sandbox/ele-testhelpers/rancher"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
)

/*
Install Backup Operator
  - @param k kubectl structure
  - @returns Nothing, the function will fail through Ginkgo in case of issue
*/
func InstallBackupOperator(k *kubectl.Kubectl) {
	// Set specific operator version if defined
	backupRestoreVersion := os.Getenv("BACKUP_OPERATOR_VERSION")
	chartRepo := "rancher-chart"
	if backupRestoreVersion != "" {
		chartRepo = "https://github.com/rancher/backup-restore-operator/releases/download/" + backupRestoreVersion
	} else {
		RunHelmCmdWithRetry("repo", "add", chartRepo, "https://charts.rancher.io")
		RunHelmCmdWithRetry("repo", "update")
	}

	for _, chart := range []string{"rancher-backup-crd", "rancher-backup"} {
		// Set the filename in chart if a custom version is defined
		chartName := chart
		if backupRestoreVersion != "" {
			chartName = chart + "-" + strings.TrimPrefix(backupRestoreVersion, "v") + ".tgz"
		}

		// Global installation flags
		flags := []string{
			"upgrade", "--install", chart, chartRepo + "/" + chartName,
			"--namespace", "cattle-resources-system",
			"--create-namespace",
			"--wait", "--wait-for-jobs",
		}

		// Add specific options for the rancher-backup chart
		if chart == "rancher-backup" {
			flags = append(flags,
				"--set", "persistence.enabled=true",
				"--set", "persistence.storageClass=local-path",
			)
		}

		RunHelmCmdWithRetry(flags...)

		Eventually(func() error {
			return rancher.CheckPod(k, [][]string{{"cattle-resources-system", "app.kubernetes.io/name=rancher-backup"}})
		}, tools.SetTimeout(5*time.Minute), 30*time.Second).Should(Not(HaveOccurred()))
	}
}

/*
Check Backup or Restore
  - @param resourceName, Backup/Restore name
  - @param message, string to check in operator logs
  - @returns Nothing, the function will fail through Ginkgo in case of issue
*/
func CheckOperation(resourceName string, message string) {
	var operation string
	if strings.Contains(resourceName, "backup") {
		operation = "backup"
	} else {
		operation = "restore"
	}
	Eventually(func() string {
		out, _ := kubectl.RunWithoutErr("get", operation, resourceName,
			"-o", "jsonpath={.metadata.name}")
		return out
	}, tools.SetTimeout(5*time.Minute), 10*time.Second).Should(ContainSubstring(resourceName))

	// Wait for operation to be done
	Eventually(func() string {
		out, _ := kubectl.RunWithoutErr("logs", "-l app.kubernetes.io/name=rancher-backup",
			"--tail=-1", "--since=5m",
			"--namespace", "cattle-resources-system")
		return out
	}, tools.SetTimeout(5*time.Minute), 10*time.Second).Should(ContainSubstring(message))
}

/*
Get PV local path
  - @returns Configured backup directory
*/
func GetLocalPath() string {
	claimName, err := kubectl.RunWithoutErr("get", "pod", "-l", "app.kubernetes.io/name=rancher-backup",
		"--namespace", "cattle-resources-system",
		"-o", "jsonpath={.items[*].spec.volumes[?(@.name==\"pv-storage\")].persistentVolumeClaim.claimName}")
	Expect(err).To(Not(HaveOccurred()))

	localPath, err := kubectl.RunWithoutErr("get", "pv",
		"--namespace", "cattle-resources-system",
		"-o", "jsonpath={.items[?(@.spec.claimRef.name==\""+claimName+"\")].spec.local.path}")
	Expect(err).To(Not(HaveOccurred()))

	return localPath
}

/*
Execute Backup
  - @param k kubectl structure
  - @returns Backup file
*/
func ExecuteBackup(k *kubectl.Kubectl, backupResourceName string) string {
	var err error
	var backupFile string

	By("Installing rancher-backup-operator", func() {
		InstallBackupOperator(k)
	})

	By("Adding a backup resource", func() {
		err = kubectl.Apply("fleet-default", "../../helpers/assets/backup.yaml")
		Expect(err).To(Not(HaveOccurred()))
	})

	By("Checking that the backup has been done", func() {
		CheckOperation(backupResourceName, "Done with backup")
	})

	By("Copying the backup file", func() {
		// Get the backup file from the previous backup
		backupFile, err = kubectl.RunWithoutErr("get", "backup", backupResourceName, "-o", "jsonpath={.status.filename}")
		Expect(err).To(Not(HaveOccurred()))

		// Get local storage path
		localPath := GetLocalPath()

		// Copy backup file
		err = exec.Command("sudo", "cp", localPath+"/"+backupFile, ".").Run()
		Expect(err).To(Not(HaveOccurred()))
	})
	return backupFile
}

/*
Execute Restore
  - @param k kubectl structure
  - @returns Nothing, the function will fail through Ginkgo in case of issue
*/
func ExecuteRestore(k *kubectl.Kubectl, restoreResourceName, backupFile string) {
	By("Installing rancher-backup-operator", func() {
		InstallBackupOperator(k)
	})

	By("Copying backup file to restore", func() {
		// Get local storage path
		localPath := GetLocalPath()

		// Copy backup file
		err := exec.Command("sudo", "cp", backupFile, localPath).Run()
		Expect(err).To(Not(HaveOccurred()))
	})

	By("Adding a restore resource", func() {
		// Set the backup file in the restore resource
		err := tools.Sed("BACKUP_FILE", backupFile, "../../helpers/assets/restore.yaml")
		Expect(err).To(Not(HaveOccurred()))

		// And apply
		err = kubectl.Apply("fleet-default", "../../helpers/assets/restore.yaml")
		Expect(err).To(Not(HaveOccurred()))
	})

	By("Checking that the restore has been done", func() {
		CheckOperation(restoreResourceName, "Done restoring")
	})
}
