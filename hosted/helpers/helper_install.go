package helpers

import (
	"fmt"
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

/**
 * Execute RunHelmBinaryWithCustomErr within a loop with timeout
 * @param s options to pass to RunHelmBinaryWithCustomErr command
 * @returns Nothing, the function will fail through Ginkgo in case of issue
 */
func RunHelmCmdWithRetry(s ...string) {
	Eventually(func() error {
		output, err := kubectl.RunHelmBinaryWithOutput(s...)
		GinkgoWriter.Write([]byte(output))
		if err != nil {
			return err
		}
		return nil
	}, tools.SetTimeout(2*time.Minute), 20*time.Second).Should(Not(HaveOccurred()))
}

/*
*
Install k3s
  - @param k kubectl structure
  - @param k3sVersion version to install
  - @param proxy, enable proxy
  - @param proxyHost, if proxy enabled
  - @returns Nothing, the function will fail through Ginkgo in case of issue
*/
func InstallK3S(k *kubectl.Kubectl, k3sVersion, proxy, proxyHost string) {
	if proxy == "enabled" {
		By("Run local squid proxy in docker", func() {
			// Configure proxy before k3s installation if requested
			GinkgoLogr.Info("Starting squid proxy")

			cwd, _ := os.Getwd()
			GinkgoLogr.Info("Current working directory: " + cwd)

			out, err := exec.Command("docker", "run", "-d", "--rm", "--name", "squid_proxy",
				"--volume", cwd+"/.github/scripts/squid.conf:/etc/squid/squid.conf",
				"-p", "3128:3128", "ubuntu/squid").CombinedOutput()
			GinkgoWriter.Println(string(out))
			Expect(err).To(Not(HaveOccurred()))
		})

		By("Configure proxy in /etc/default/k3s", func() {
			k3sConfigPath := "/etc/default/k3s"
			k3sConfig := fmt.Sprintf(`HTTP_PROXY=http://%s
HTTPS_PROXY=http://%s
NO_PROXY=127.0.0.0/8,10.0.0.0/8,cattle-system.svc,172.16.0.0/12,192.168.0.0/16,.svc,.cluster.local`, proxyHost, proxyHost)
			// Write the k3s proxy config file as root
			out, err := exec.Command("sh", "-c", fmt.Sprintf("echo '%s' | sudo tee %s", k3sConfig, k3sConfigPath)).CombinedOutput()
			GinkgoWriter.Println(string(out))
			Expect(err).To(Not(HaveOccurred()))
		})
	}

	By("Getting k3s ready", func() {
		installCmd := exec.Command("sh", "-c", "curl -sfL https://get.k3s.io | sh -s - server --cluster-init")
		installCmd.Env = append(os.Environ(), "INSTALL_K3S_VERSION="+k3sVersion, "INSTALL_K3S_EXEC=--write-kubeconfig-mode 644")

		// Execute k3s installation
		count := 1
		Eventually(func() error {
			// Execute k3s installation
			out, err := installCmd.CombinedOutput()
			GinkgoWriter.Printf("K3s installation loop %d:\n%s\n", count, out)
			count++
			return err
		}, tools.SetTimeout(2*time.Minute), 5*time.Second).Should(BeNil(), "K3s installation failed")
	})

	By("Waiting for k3s to be started", func() {
		// Wait for all pods to be started
		checkList := [][]string{
			{"kube-system", "app=local-path-provisioner"},
			{"kube-system", "k8s-app=kube-dns"},
			{"kube-system", "k8s-app=metrics-server"},
			{"kube-system", "app.kubernetes.io/name=traefik"},
			{"kube-system", "svccontroller.k3s.cattle.io/svcname=traefik"},
		}
		Eventually(func() error {
			return rancher.CheckPod(k, checkList)
		}, tools.SetTimeout(4*time.Minute), 30*time.Second).Should(BeNil(), "K3s pods are not running")
	})
}

/*
*
Install CertManager
  - @param k kubectl structure
  - @param proxy, enable proxy
  - @param proxyHost, if proxy enabled
  - @returns Nothing, the function will fail through Ginkgo in case of issue
*/
func InstallCertManager(k *kubectl.Kubectl, proxy, proxyHost string) {
	By("Installing CertManager", func() {
		RunHelmCmdWithRetry("repo", "add", "jetstack", "https://charts.jetstack.io")
		RunHelmCmdWithRetry("repo", "update")

		// Set flags for cert-manager installation
		flags := []string{
			"upgrade", "--install", "cert-manager", "jetstack/cert-manager",
			"--namespace", "cert-manager",
			"--create-namespace",
			"--set", "crds.enabled=true",
			"--wait", "--wait-for-jobs",
		}

		if proxy == "enabled" {
			flags = append(flags, "--set", "http_proxy=http://"+proxyHost,
				"--set", "https_proxy=http://"+proxyHost,
				"--set", "no_proxy=127.0.0.0/8\\,10.0.0.0/8\\,cattle-system.svc\\,172.16.0.0/12\\,192.168.0.0/16\\,.svc\\,.cluster.local")
		}
		GinkgoWriter.Printf("Helm flags: %v\n", flags)
		RunHelmCmdWithRetry(flags...)

		checkList := [][]string{
			{"cert-manager", "app.kubernetes.io/component=controller"},
			{"cert-manager", "app.kubernetes.io/component=webhook"},
			{"cert-manager", "app.kubernetes.io/component=cainjector"},
		}
		Eventually(func() error {
			return rancher.CheckPod(k, checkList)
		}, tools.SetTimeout(4*time.Minute), 30*time.Second).Should(BeNil(), "CertManager pods are not running")
	})
}

/*
*
Install Rancher Manager

  - @param k kubectl structure

  - @param rancherHostname, Rancher Hostname

  - @param rancherChannel, Rancher Channel [eg. latest, prime]

  - @param rancherVersion, Rancher Version [eg. 2.9.3-rc2]

  - @param rancherHeadVersion, Rancher Head version [eg. 2.9-head]

  - @param proxy, enable proxy

  - @param nightlyChart, enable operator nightly chart

  - @returns Nothing, the function will fail through Ginkgo in case of issue
*/
func InstallRancherManager(k *kubectl.Kubectl, rancherHostname, rancherChannel, rancherVersion, rancherHeadVersion, proxy, nightlyChart string) {
	var proxyEnabled string
	if proxy == "enabled" {
		proxyEnabled = "rancher"
	} else {
		proxyEnabled = "none"
	}

	var extraFlags []string
	if nightlyChart == "enabled" {
		// Ensure proper extraEnv index sequence for helm rendering
		// All head versions and releases from prime-optimus[-alpha] channel require an extraEnv index of 2
		// See https://github.com/rancher-sandbox/ele-testhelpers/blob/main/rancher/install.go
		extraEnvIndex := 1
		if rancherHeadVersion != "" || strings.Contains(rancherChannel, "prime-optimus") {
			extraEnvIndex = 2
		}
		extraFlags = []string{
			"--set", fmt.Sprintf("extraEnv[%d].name=CATTLE_SKIP_HOSTED_CLUSTER_CHART_INSTALLATION", extraEnvIndex),
			"--set-string", fmt.Sprintf("extraEnv[%d].value=true", extraEnvIndex),
		}
	}

	err := rancher.DeployRancherManager(rancherHostname, rancherChannel, rancherVersion, rancherHeadVersion, "none", proxyEnabled, extraFlags)
	Expect(err).To(Not(HaveOccurred()))

	// Wait for all pods to be started
	checkList := [][]string{
		{"cattle-system", "app=rancher"},
	}
	Eventually(func() error {
		return rancher.CheckPod(k, checkList)
	}, tools.SetTimeout(4*time.Minute), 30*time.Second).Should(BeNil(), "Rancher pod is not running")
}

/*
*
Check Rancher Deployments
  - @param k kubectl structure
  - @returns Nothing, the function will fail through Ginkgo in case of issue
*/
func CheckRancherDeployments(k *kubectl.Kubectl) {
	By("Waiting for fleet", func() {
		// Wait until the kubectl command returns exit code 0
		count := 1
		Eventually(func() error {
			out, err := kubectl.Run("rollout", "status",
				"--namespace", "cattle-fleet-system",
				"deployment", "fleet-controller",
			)
			GinkgoWriter.Printf("Waiting for fleet-controller deployment, loop %d:\n%s\n", count, out)
			count++
			return err
		}, tools.SetTimeout(2*time.Minute), 5*time.Second).Should(Not(HaveOccurred()), "Fleet-controller deployment failed")

		checkList := [][]string{
			{"cattle-fleet-system", "app=fleet-controller"},
		}
		Eventually(func() error {
			return rancher.CheckPod(k, checkList)
		}, tools.SetTimeout(4*time.Minute), 30*time.Second).Should(BeNil(), "Fleet-controller pod is not running")
	})

	By("Waiting for rancher-webhook", func() {
		// Wait unit the kubectl command returns exit code 0
		count := 1
		Eventually(func() error {
			out, err := kubectl.Run("rollout", "status",
				"--namespace", "cattle-system",
				"deployment", "rancher-webhook",
			)
			GinkgoWriter.Printf("Waiting for rancher-webhook deployment, loop %d:\n%s\n", count, out)
			count++
			return err
		}, tools.SetTimeout(2*time.Minute), 5*time.Second).Should(Not(HaveOccurred()), "Rancher-webhook deployment failed")

		checkList := [][]string{
			{"cattle-system", "app=rancher-webhook"},
		}
		Eventually(func() error {
			return rancher.CheckPod(k, checkList)
		}, tools.SetTimeout(4*time.Minute), 30*time.Second).Should(BeNil(), "Rancher-webhook pod is not running")
	})

	By("Waiting for capi-controller-manager", func() {
		// Wait unit the kubectl command returns exit code 0
		count := 1
		Eventually(func() error {
			out, err := kubectl.Run("rollout", "status",
				"--namespace", "cattle-provisioning-capi-system",
				"deployment", "capi-controller-manager",
			)
			GinkgoWriter.Printf("Waiting for capi-controller-manager deployment, loop %d:\n%s\n", count, out)
			count++
			return err
		}, tools.SetTimeout(2*time.Minute), 5*time.Second).Should(Not(HaveOccurred()), "Capi-controller-manager deployment failed")

		checkList := [][]string{
			{"cattle-provisioning-capi-system", "cluster.x-k8s.io/provider=cluster-api"},
		}
		Eventually(func() error {
			return rancher.CheckPod(k, checkList)
		}, tools.SetTimeout(4*time.Minute), 30*time.Second).Should(BeNil(), "Capi-controller-manager pod is not running")
	})
}
