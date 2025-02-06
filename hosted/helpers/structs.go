package helpers

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/rancher/shepherd/clients/rancher"
	"github.com/rancher/shepherd/pkg/session"
)

const (
	Timeout        = 30 * time.Minute
	CattleSystemNS = "cattle-system"
)

var (
	RancherPassword   = os.Getenv("RANCHER_PASSWORD")
	RancherHostname   = os.Getenv("RANCHER_HOSTNAME")
	Provider          = os.Getenv("PROVIDER")
	testuser, _       = user.Current()
	clusterCleanup, _ = strconv.ParseBool(os.Getenv("DOWNSTREAM_CLUSTER_CLEANUP"))
	ClusterNamePrefix = func() string {
		if clusterCleanup {
			return fmt.Sprintf("%s-hp-ci", Provider)
		} else {
			return fmt.Sprintf("%s-%s-hp-ci", Provider, testuser.Username)
		}
	}()
	RancherFullVersion        = os.Getenv("RANCHER_VERSION")
	RancherUpgradeFullVersion = os.Getenv("RANCHER_UPGRADE_VERSION")
	Kubeconfig                = os.Getenv("KUBECONFIG")
	DownstreamKubeconfig      = func(clusterName string) string {
		return fmt.Sprintf("%s_KUBECONFIG", clusterName)
	}
	K8sUpgradedMinorVersion   = os.Getenv("K8S_UPGRADE_MINOR_VERSION")
	DownstreamK8sMinorVersion = os.Getenv("DOWNSTREAM_K8S_MINOR_VERSION")
	IsImport                  = func() bool {
		if strings.Contains(os.Getenv("CATTLE_TEST_CONFIG"), "import") {
			return true
		}
		return false
	}()
	SkipUpgradeTests = func() bool {
		// Skip upgrade tests since lowest k8s version not available
		return strings.Contains((RancherFullVersion), "2.8")
	}()
	SkipTest = func() bool {
		// Some features not available on v2.8, v2.9
		return strings.Contains((RancherFullVersion), "2.8") || strings.Contains((RancherFullVersion), "2.9")
	}()
	SkipUpgradeTestsLog = "Skipping upgrade tests since only one minor k8s version is supported by the current rancher version ..."
)

type HelmChart struct {
	Name           string `json:"name"`
	Chart          string `json:"chart"`
	AppVersion     string `json:"app_version"`
	DerivedVersion string `json:"version"`
}

type RancherContext struct {
	RancherAdminClient *rancher.Client
	StdUserClient      *rancher.Client
	Session            *session.Session
	ClusterCleanup     bool
	CloudCredID        string
}

type RancherVersionInfo struct {
	Version      string
	GitCommit    string
	RancherPrime string
	Devel        bool
}
