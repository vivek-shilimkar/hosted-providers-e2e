package helper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/tools"

	"github.com/rancher/shepherd/extensions/clusters"
	"github.com/rancher/shepherd/extensions/clusters/gke"
	k8slabels "k8s.io/apimachinery/pkg/labels"

	"github.com/rancher/hosted-providers-e2e/hosted/helpers"

	"github.com/Masterminds/semver/v3"
	"github.com/epinio/epinio/acceptance/helpers/proc"
	"github.com/pkg/errors"
	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters/kubernetesversions"
	"github.com/rancher/shepherd/pkg/config"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"
	"k8s.io/utils/pointer"
)

// GetLabels fetches labels from config file, appends the common labels/tags to it and returns the map
func GetLabels() map[string]string {
	gkeConfig := new(management.GKEClusterConfigSpec)
	config.LoadConfig(gke.GKEClusterConfigConfigurationFileKey, gkeConfig)
	labels := helpers.GetCommonMetadataLabels()
	if gkeConfig.Labels != nil {
		for key, value := range *gkeConfig.Labels {
			labels[key] = value
		}
	}
	return labels
}

// UpgradeKubernetesVersion upgrades the k8s version to the value defined by upgradeToVersion; if upgradeNodePool is true, it also upgrades nodepool k8s version;
// if wait is set to true, it waits until the update is complete; if checkClusterConfig is true, it validates the update
func UpgradeKubernetesVersion(cluster *management.Cluster, upgradeToVersion string, client *rancher.Client, upgradeNodePool, wait, checkClusterConfig bool) (*management.Cluster, error) {
	currentVersion := *cluster.GKEConfig.KubernetesVersion
	upgradedCluster := new(management.Cluster)
	upgradedCluster.Name = cluster.Name
	upgradedCluster.GKEConfig = cluster.GKEConfig

	var upgradeCP bool
	if *cluster.GKEConfig.KubernetesVersion != upgradeToVersion {
		upgradedCluster.GKEConfig.KubernetesVersion = &upgradeToVersion
		upgradeCP = true
	}

	if upgradeNodePool {
		for i := range upgradedCluster.GKEConfig.NodePools {
			upgradedCluster.GKEConfig.NodePools[i].Version = &upgradeToVersion
		}
	}

	cluster, err := client.Management.Cluster.Update(cluster, &upgradedCluster)
	if err != nil {
		return nil, err
	}

	if checkClusterConfig {
		Expect(*cluster.GKEConfig.KubernetesVersion).To(Equal(upgradeToVersion))
		for _, np := range cluster.GKEConfig.NodePools {
			if upgradeNodePool {
				Expect(*np.Version).To(Equal(upgradeToVersion))
			} else {
				Expect(*np.Version).To(Equal(currentVersion))
			}
		}
	}
	if wait {
		err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
		Expect(err).To(BeNil())
	}
	if checkClusterConfig {
		if upgradeCP {
			Eventually(func() string {
				ginkgo.GinkgoLogr.Info("Waiting for k8s upgrade to appear in GKEStatus.UpstreamSpec ...")
				cluster, err = client.Management.Cluster.ByID(cluster.ID)
				Expect(err).NotTo(HaveOccurred())
				return *cluster.GKEStatus.UpstreamSpec.KubernetesVersion
			}, tools.SetTimeout(12*time.Minute), 10*time.Second).Should(Equal(upgradeToVersion))

			if !upgradeNodePool {
				for _, np := range cluster.GKEConfig.NodePools {
					Expect(*np.Version).To(BeEquivalentTo(currentVersion))
				}
			}
		}

		if upgradeNodePool {
			Eventually(func() bool {
				ginkgo.GinkgoLogr.Info("waiting for the nodepool upgrade to appear in GKEStatus.UpstreamSpec ...")
				cluster, err = client.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				for _, np := range cluster.GKEStatus.UpstreamSpec.NodePools {
					if *np.Version != upgradeToVersion {
						return false
					}
				}
				return true
			}, tools.SetTimeout(12*time.Minute), 10*time.Second).Should(BeTrue())
		}

		Expect(*cluster.GKEStatus.UpstreamSpec.KubernetesVersion).To(Equal(upgradeToVersion))
	}
	return cluster, nil
}

// DeleteGKEHostCluster deletes the GKE cluster
func DeleteGKEHostCluster(cluster *management.Cluster, client *rancher.Client) error {
	return client.Management.Cluster.Delete(cluster)
}

// AddNodePool adds a nodepool to the list
// if wait is set to true, it waits until the update is complete; if checkClusterConfig is true, it validates the update
func AddNodePool(cluster *management.Cluster, increaseBy int, client *rancher.Client, wait, checkClusterConfig bool) (*management.Cluster, error) {
	currentNodePoolNumber := len(cluster.GKEConfig.NodePools)
	upgradedCluster := new(management.Cluster)
	upgradedCluster.Name = cluster.Name
	upgradedCluster.GKEConfig = cluster.GKEConfig
	nodeConfig := GkeHostNodeConfig()
	updateNodePoolsList := cluster.GKEConfig.NodePools
	for i := 1; i <= increaseBy; i++ {
		for _, np := range nodeConfig {
			newNodepool := management.GKENodePoolConfig{
				InitialNodeCount:  np.InitialNodeCount,
				Version:           cluster.GKEConfig.KubernetesVersion,
				Config:            np.Config,
				Autoscaling:       np.Autoscaling,
				Management:        np.Management,
				MaxPodsConstraint: np.MaxPodsConstraint,
				Name:              pointer.String(namegen.RandStringLower(5)),
			}
			updateNodePoolsList = append(updateNodePoolsList, newNodepool)
		}
	}
	upgradedCluster.GKEConfig.NodePools = updateNodePoolsList

	cluster, err := client.Management.Cluster.Update(cluster, &upgradedCluster)
	if err != nil {
		return nil, err
	}

	if checkClusterConfig {
		// Check if the desired config is set correctly
		Expect(len(cluster.GKEConfig.NodePools)).Should(BeNumerically("==", currentNodePoolNumber+increaseBy))
		for i, np := range cluster.GKEConfig.NodePools {
			Expect(np.Name).To(Equal(updateNodePoolsList[i].Name))
		}
	}

	if wait {
		err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
		Expect(err).To(BeNil())
	}

	if checkClusterConfig {
		// Check if the desired config has been applied in Rancher
		Eventually(func() int {
			ginkgo.GinkgoLogr.Info("Waiting for the total nodepool count to increase in GKEStatus.UpstreamSpec ...")
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			return len(cluster.GKEStatus.UpstreamSpec.NodePools)
		}, tools.SetTimeout(12*time.Minute), 10*time.Second).Should(BeNumerically("==", currentNodePoolNumber+increaseBy))

		for i, np := range cluster.GKEStatus.UpstreamSpec.NodePools {
			Expect(np.Name).To(Equal(updateNodePoolsList[i].Name))
		}
	}
	return cluster, nil
}

// DeleteNodePool deletes a nodepool from the list
// if wait is set to true, it waits until the update is complete; if checkClusterConfig is true, it validates the update
// TODO: Modify this method to delete a custom qty of nodepool, perhaps by adding an `decreaseBy int` arg
func DeleteNodePool(cluster *management.Cluster, client *rancher.Client, wait, checkClusterConfig bool) (*management.Cluster, error) {
	currentNodePoolNumber := len(cluster.GKEConfig.NodePools)
	upgradedCluster := new(management.Cluster)
	upgradedCluster.Name = cluster.Name
	upgradedCluster.GKEConfig = cluster.GKEConfig
	updatedNodePoolsList := cluster.GKEConfig.NodePools[1:]
	upgradedCluster.GKEConfig.NodePools = updatedNodePoolsList

	cluster, err := client.Management.Cluster.Update(cluster, &upgradedCluster)
	if err != nil {
		return nil, err
	}

	if checkClusterConfig {
		// Check if the desired config is set correctly
		Expect(len(cluster.GKEConfig.NodePools)).Should(BeNumerically("==", currentNodePoolNumber-1))
		for i, np := range cluster.GKEConfig.NodePools {
			Expect(np.Name).To(Equal(updatedNodePoolsList[i].Name))
		}
	}
	if wait {
		err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
		Expect(err).To(BeNil())
	}
	if checkClusterConfig {

		// Check if the desired config has been applied in Rancher
		Eventually(func() int {
			ginkgo.GinkgoLogr.Info("Waiting for the total nodepool count to decrease in GKEStatus.UpstreamSpec ...")
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			return len(cluster.GKEStatus.UpstreamSpec.NodePools)
		}, tools.SetTimeout(12*time.Minute), 10*time.Second).Should(BeNumerically("==", currentNodePoolNumber-1))
		for i, np := range cluster.GKEStatus.UpstreamSpec.NodePools {
			Expect(np.Name).To(Equal(updatedNodePoolsList[i].Name))
		}
	}

	return cluster, nil
}

// ScaleNodePool modifies the number of initialNodeCount of all the nodepools as defined by nodeCount
// if wait is set to true, it waits until the update is complete; if checkClusterConfig is true, it validates the update
func ScaleNodePool(cluster *management.Cluster, client *rancher.Client, nodeCount int64, wait, checkClusterConfig bool) (*management.Cluster, error) {
	upgradedCluster := new(management.Cluster)
	upgradedCluster.Name = cluster.Name
	upgradedCluster.GKEConfig = cluster.GKEConfig
	for i := range upgradedCluster.GKEConfig.NodePools {
		upgradedCluster.GKEConfig.NodePools[i].InitialNodeCount = pointer.Int64(nodeCount)
	}

	cluster, err := client.Management.Cluster.Update(cluster, &upgradedCluster)
	if err != nil {
		return nil, err
	}
	if checkClusterConfig {
		// Check if the desired config is set correctly
		for i := range cluster.GKEConfig.NodePools {
			Expect(*cluster.GKEConfig.NodePools[i].InitialNodeCount).To(BeNumerically("==", nodeCount))
		}
	}

	if wait {
		err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
		Expect(err).To(BeNil())
	}

	if checkClusterConfig {
		// check that the desired config is applied on Rancher
		Eventually(func() bool {
			ginkgo.GinkgoLogr.Info("Waiting for the node count change to appear in GKEStatus.UpstreamSpec ...")
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			for i := range cluster.GKEStatus.UpstreamSpec.NodePools {
				if *cluster.GKEStatus.UpstreamSpec.NodePools[i].InitialNodeCount != nodeCount {
					return false
				}
			}
			return true
		}, tools.SetTimeout(12*time.Minute), 10*time.Second).Should(BeTrue())
	}

	return cluster, nil
}

// UpdateMonitoringAndLoggingService updates the monitoring and loggingService of a GKE cluster
// if wait is set to true, it waits until the update is complete; if checkClusterConfig is true, it validates the update
func UpdateMonitoringAndLoggingService(cluster *management.Cluster, client *rancher.Client, monitoringService, loggingService string, wait, checkClusterConfig bool) (*management.Cluster, error) {
	upgradedCluster := new(management.Cluster)
	upgradedCluster.Name = cluster.Name
	upgradedCluster.GKEConfig = cluster.GKEConfig

	upgradedCluster.GKEConfig.LoggingService = &loggingService
	upgradedCluster.GKEConfig.MonitoringService = &monitoringService

	cluster, err := client.Management.Cluster.Update(cluster, &upgradedCluster)
	if err != nil {
		return nil, err
	}
	if checkClusterConfig {
		Expect(*cluster.GKEConfig.MonitoringService).To(BeEquivalentTo(monitoringService))
		Expect(*cluster.GKEConfig.LoggingService).To(BeEquivalentTo(loggingService))
	}
	if wait {
		err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
		Expect(err).To(BeNil())
	}
	if checkClusterConfig {
		Eventually(func() bool {
			ginkgo.GinkgoLogr.Info("Waiting for the node count change to appear in GKEStatus.UpstreamSpec ...")
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			return *cluster.GKEStatus.UpstreamSpec.MonitoringService == monitoringService && *cluster.GKEStatus.UpstreamSpec.LoggingService == loggingService
		}, tools.SetTimeout(12*time.Minute), 10*time.Second).Should(BeTrue())
	}
	return cluster, nil
}

// UpdateAutoScaling updates the management.GKENodePoolAutoscaling for all the node pools of a GKE cluster
// if wait is set to true, it waits until the update is complete; if checkClusterConfig is true, it validates the update
// TODO: Facilitate passing minCount and maxCount values when autoscaling is enabled.
func UpdateAutoScaling(cluster *management.Cluster, client *rancher.Client, enabled, wait, checkClusterConfig bool) (*management.Cluster, error) {
	upgradedCluster := new(management.Cluster)
	upgradedCluster.Name = cluster.Name
	upgradedCluster.GKEConfig = cluster.GKEConfig
	for i := range upgradedCluster.GKEConfig.NodePools {
		upgradedCluster.GKEConfig.NodePools[i].Autoscaling = &management.GKENodePoolAutoscaling{
			Enabled: enabled,
		}
	}
	cluster, err := client.Management.Cluster.Update(cluster, &upgradedCluster)
	if err != nil {
		return nil, err
	}
	if checkClusterConfig {
		for _, np := range cluster.GKEConfig.NodePools {
			Expect(np.Autoscaling.Enabled).To(BeEquivalentTo(enabled))
		}
	}
	if wait {
		err = clusters.WaitClusterToBeUpgraded(client, cluster.ID)
		Expect(err).To(BeNil())
	}
	if checkClusterConfig {
		Eventually(func() bool {
			ginkgo.GinkgoLogr.Info("Waiting for the node count change to appear in GKEStatus.UpstreamSpec ...")
			cluster, err = client.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			for _, np := range cluster.GKEStatus.UpstreamSpec.NodePools {
				if np.Autoscaling.Enabled != enabled {
					return false
				}
			}
			return true
		}, tools.SetTimeout(12*time.Minute), 10*time.Second).Should(BeTrue())
	}
	return cluster, nil
}

// ListGKEAvailableVersions is a function to list and return only available GKE versions for a specific cluster.
func ListGKEAvailableVersions(client *rancher.Client, clusterID string) ([]string, error) {
	// kubernetesversions.ListGKEAvailableVersions expects cluster.Version.GitVersion to be available, which it is not sometimes, so we fetch the cluster again to ensure it has all the available data
	cluster, err := client.Management.Cluster.ByID(clusterID)
	if err != nil {
		return nil, err
	}
	availableVersions, err := kubernetesversions.ListGKEAvailableVersions(client, cluster)
	if err != nil {
		return nil, err
	}
	return helpers.FilterUIUnsupportedVersions(availableVersions, client), nil
}

// ListSingleVariantGKEAvailableVersions returns a list of single variants of minor versions
// For e.g 1.27.5-gke.1700, 1.26.6-gke.2100, 1.25.8-gke.200
func ListSingleVariantGKEAvailableVersions(client *rancher.Client, projectID, cloudCredentialID, zone, region string) (availableVersions []string, err error) {
	availableVersions, err = kubernetesversions.ListGKEAllVersions(client, projectID, cloudCredentialID, zone, region)
	if err != nil {
		return nil, err
	}
	var singleVersionList []string
	var oldMinor uint64
	for _, version := range availableVersions {
		semVersion := semver.MustParse(version)
		if currentMinor := semVersion.Minor(); oldMinor != currentMinor {
			singleVersionList = append(singleVersionList, version)
			oldMinor = currentMinor
		}
	}
	return helpers.FilterUIUnsupportedVersions(singleVersionList, client), nil
}

// GetK8sVersionVariantGKE returns a variant of a given minor K8s version
func GetK8sVersionVariantGKE(minorVersion string, client *rancher.Client, projectID, cloudCrendetialID, zone, region string) (string, error) {
	versions, err := ListSingleVariantGKEAvailableVersions(client, projectID, cloudCrendetialID, zone, region)
	if err != nil {
		return "", err
	}

	for _, version := range versions {
		if strings.Contains(version, minorVersion) {
			return version, nil
		}
	}
	return "", fmt.Errorf("version %s not found", minorVersion)
}

// Create Google GKE cluster using gcloud CLI
func CreateGKEClusterOnGCloud(zone string, clusterName string, project string, k8sVersion string, extraArgs ...string) error {

	labels := GetLabels()
	labelsAsString := k8slabels.SelectorFromSet(labels).String()

	// creating GKE using gcloud changes the kubeconfig to use GKE; this can be problematic for test cases that need to use local cluster;
	// this workaround helps to keep the original kubeconfig
	currentKubeconfig := os.Getenv("KUBECONFIG")
	defer os.Setenv("KUBECONFIG", currentKubeconfig)

	helpers.SetTempKubeConfig(clusterName)

	fmt.Println("Creating GKE cluster ...")
	args := []string{"container", "clusters", "create", clusterName, "--project", project, "--zone", zone, "--cluster-version", k8sVersion, "--labels", labelsAsString, "--network", "default", "--release-channel", "None", "--machine-type", "n2-standard-2", "--disk-size", "100", "--num-nodes", "1", "--no-enable-master-authorized-networks"}
	args = append(args, extraArgs...)
	fmt.Printf("Running command: gcloud %v\n", args)
	out, err := proc.RunW("gcloud", args...)
	if err != nil {
		return errors.Wrap(err, "Failed to create cluster: "+out)
	}

	fmt.Println("Created GKE cluster: ", clusterName)

	return nil
}

// Complete cleanup steps for Google GKE
func DeleteGKEClusterOnGCloud(zone, project, clusterName string) error {
	currentKubeconfig := os.Getenv("KUBECONFIG")
	downstreamKubeconfig := os.Getenv(helpers.DownstreamKubeconfig(clusterName))
	defer func() {
		_ = os.Setenv("KUBECONFIG", currentKubeconfig)
		_ = os.Remove(downstreamKubeconfig) // clean up
	}()
	_ = os.Setenv("KUBECONFIG", downstreamKubeconfig)

	fmt.Println("Deleting GKE cluster ...")
	args := []string{"container", "clusters", "delete", clusterName, "--zone", zone, "--quiet", "--project", project}
	fmt.Printf("Running command: gcloud %v\n", args)
	out, err := proc.RunW("gcloud", args...)
	if err != nil {
		return errors.Wrap(err, "Failed to delete cluster: "+out)
	}

	fmt.Println("Deleted GKE cluster: ", clusterName)

	return nil
}

func ImportGKEHostedCluster(client *rancher.Client, displayName, cloudCredentialID string, enableClusterAlerting, enableClusterMonitoring, enableNetworkPolicy, windowsPreferedCluster bool, labels map[string]string) (*management.Cluster, error) {
	gkeHostCluster := GkeHostClusterConfig(displayName, cloudCredentialID)
	cluster := &management.Cluster{
		DockerRootDir:           "/var/lib/docker",
		GKEConfig:               gkeHostCluster,
		Name:                    displayName,
		EnableClusterAlerting:   enableClusterAlerting,
		EnableClusterMonitoring: enableClusterMonitoring,
		EnableNetworkPolicy:     &enableNetworkPolicy,
		Labels:                  labels,
		WindowsPreferedCluster:  windowsPreferedCluster,
	}

	clusterResp, err := client.Management.Cluster.Create(cluster)
	if err != nil {
		return nil, err
	}
	return clusterResp, err
}

func GkeHostClusterConfig(displayName, cloudCredentialID string) *management.GKEClusterConfigSpec {
	var gkeClusterConfig ImportClusterConfig
	config.LoadConfig("gkeClusterConfig", &gkeClusterConfig)

	return &management.GKEClusterConfigSpec{
		GoogleCredentialSecret: cloudCredentialID,
		ClusterName:            displayName,
		Imported:               gkeClusterConfig.Imported,
		Zone:                   gkeClusterConfig.Zone,
		ProjectID:              gkeClusterConfig.ProjectID,
	}
}

func GkeHostNodeConfig() []management.GKENodePoolConfig {
	var nodeConfig management.GKEClusterConfigSpec
	config.LoadConfig("gkeClusterConfig", &nodeConfig)

	return nodeConfig.NodePools
}

type ImportClusterConfig struct {
	ProjectID string                          `json:"projectID" yaml:"projectID"`
	Zone      string                          `json:"zone" yaml:"zone"`
	Imported  bool                            `json:"imported" yaml:"imported"`
	NodePools []*management.GKENodePoolConfig `json:"nodePools" yaml:"nodePools"`
	Labels    *map[string]string              `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// defaultGKE returns the default GKE version used by Rancher
func defaultGKE(client *rancher.Client, projectID, cloudCredentialID, zone, region string) (defaultGKE string, err error) {
	url := fmt.Sprintf("%s://%s/meta/gkeVersions", "https", client.RancherConfig.Host)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("Authorization", "Bearer "+client.RancherConfig.AdminToken)

	q := req.URL.Query()
	q.Add("cloudCredentialId", cloudCredentialID)

	if zone != "" {
		q.Add("zone", zone)
	} else if region != "" {
		q.Add("region", region)
	}

	q.Add("projectId", projectID)
	req.URL.RawQuery = q.Encode()

	resp, err := client.Management.APIBaseClient.Ops.Client.Do(req)
	if err != nil {
		return
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	var mapResponse map[string]interface{}
	if err = json.Unmarshal(bodyBytes, &mapResponse); err != nil {
		return
	}

	defaultGKE = mapResponse["defaultClusterVersion"].(string)

	return
}

// GetK8sVersion returns the k8s version to be used by the test;
// this value can either be a variant of envvar DOWNSTREAM_K8S_MINOR_VERSION or the default UI value returned by DefaultGKE
// or the second-highest minor k8s version if forUpgrade is true; which it is in case of k8s upgrade tests.
func GetK8sVersion(client *rancher.Client, projectID, cloudCredentialID, zone, region string, forUpgrade bool) (string, error) {
	if k8sMinorVersion := helpers.DownstreamK8sMinorVersion; k8sMinorVersion != "" {
		return GetK8sVersionVariantGKE(k8sMinorVersion, client, projectID, cloudCredentialID, zone, region)
	}

	if !forUpgrade {
		return defaultGKE(client, projectID, cloudCredentialID, zone, region)
	}

	allVariants, err := ListSingleVariantGKEAvailableVersions(client, projectID, cloudCredentialID, zone, region)
	if err != nil {
		return "", err
	}

	maxValue := helpers.HighestK8sMinorVersionSupportedByUI(client)
	for _, v := range allVariants {
		if comparator := helpers.VersionCompare(v, maxValue); comparator == -1 {
			return v, nil
		}
	}

	return "", nil
}
