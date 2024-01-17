package helpers

import (
	"fmt"
	"os"
	"time"

	. "github.com/onsi/gomega"
	"github.com/rancher/rancher/tests/framework/clients/rancher"
	management "github.com/rancher/rancher/tests/framework/clients/rancher/generated/management/v3"
	"github.com/rancher/rancher/tests/framework/extensions/cloudcredentials"
	"github.com/rancher/rancher/tests/framework/extensions/cloudcredentials/aws"
	"github.com/rancher/rancher/tests/framework/extensions/cloudcredentials/azure"
	"github.com/rancher/rancher/tests/framework/extensions/cloudcredentials/google"
	"github.com/rancher/rancher/tests/framework/extensions/clusters"
	"github.com/rancher/rancher/tests/framework/extensions/pipeline"
	"github.com/rancher/rancher/tests/framework/pkg/config"
	"github.com/rancher/rancher/tests/framework/pkg/session"
	"github.com/rancher/rancher/tests/framework/pkg/wait"
	"github.com/rancher/rancher/tests/v2prov/defaults"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	Timeout = 30 * time.Minute
)

var (
	rancherPassword = os.Getenv("RANCHER_PASSWORD")
	rancherHostname = os.Getenv("RANCHER_HOSTNAME")
	cloudCredential *cloudcredentials.CloudCredential
	rancherConfig   *rancher.Config
)

type Context struct {
	CloudCred     *cloudcredentials.CloudCredential
	RancherClient *rancher.Client
	Session       *session.Session
}

func CommonBeforeSuite(cloud string) (Context, error) {

	rancherConfig := new(rancher.Config)
	Eventually(rancherConfig, "10s").ShouldNot(BeNil())

	config.LoadAndUpdateConfig(rancher.ConfigurationFileKey, rancherConfig, func() {
		rancherConfig.Host = rancherHostname
	})

	token, err := pipeline.CreateAdminToken(rancherPassword, rancherConfig)
	Expect(err).To(BeNil())

	config.LoadAndUpdateConfig(rancher.ConfigurationFileKey, rancherConfig, func() {
		rancherConfig.AdminToken = token
	})

	testSession := session.NewSession()
	rancherClient, err := rancher.NewClient(rancherConfig.AdminToken, testSession)
	Expect(err).To(BeNil())

	setting := new(management.Setting)
	resp, err := rancherClient.Management.Setting.ByID("server-url")
	Expect(err).To(BeNil())

	setting.Source = "env"
	setting.Value = fmt.Sprintf("https://%s", rancherHostname)
	resp, err = rancherClient.Management.Setting.Update(resp, setting)
	Expect(err).To(BeNil())

	switch cloud {
	case "aks":
		credentialConfig := new(cloudcredentials.AzureCredentialConfig)
		config.LoadAndUpdateConfig("azureCredentials", credentialConfig, func() {
			credentialConfig.ClientID = os.Getenv("AKS_CLIENT_ID")
			credentialConfig.SubscriptionID = os.Getenv("AKS_SUBSCRIPTION_ID")
			credentialConfig.ClientSecret = os.Getenv("AKS_CLIENT_SECRET")
		})
		cloudCredential, err = azure.CreateAzureCloudCredentials(rancherClient)
		Expect(err).To(BeNil())

		azureClusterConfig := new(management.AKSClusterConfigSpec)
		// provisioning test cases rely on config file to fetch the location information
		// this is necessary so that there is a single source of truth for provisioning and import test cases
		config.LoadAndUpdateConfig("aksClusterConfig", azureClusterConfig, func() {
			azureClusterConfig.ResourceLocation = GetAKSLocation()
		})
	case "eks":
		credentialConfig := new(cloudcredentials.AmazonEC2CredentialConfig)
		config.LoadAndUpdateConfig("awsCredentials", credentialConfig, func() {
			credentialConfig.AccessKey = os.Getenv("AWS_ACCESS_KEY_ID")
			credentialConfig.SecretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
			credentialConfig.DefaultRegion = GetEKSRegion()
		})
		cloudCredential, err = aws.CreateAWSCloudCredentials(rancherClient)
		Expect(err).To(BeNil())
		eksClusterConfig := new(management.EKSClusterConfigSpec)
		config.LoadAndUpdateConfig("eksClusterConfig", eksClusterConfig, func() {
			eksClusterConfig.Region = GetEKSRegion()
		})
	case "gke":
		credentialConfig := new(cloudcredentials.GoogleCredentialConfig)
		config.LoadAndUpdateConfig("googleCredentials", credentialConfig, func() {
			credentialConfig.AuthEncodedJSON = os.Getenv("GCP_CREDENTIALS")
		})
		cloudCredential, err = google.CreateGoogleCloudCredentials(rancherClient)
		Expect(err).To(BeNil())
		gkeClusterConfig := new(management.GKEClusterConfigSpec)
		config.LoadAndUpdateConfig("gkeClusterConfig", gkeClusterConfig, func() {
			gkeClusterConfig.Zone = GetGKEZone()
			gkeClusterConfig.ProjectID = GetGKEProjectID()
		})
	}

	return Context{
		CloudCred:     cloudCredential,
		RancherClient: rancherClient,
		Session:       testSession,
	}, nil
}

// WaitUntilClusterIsReady waits until the cluster is in a Ready state,
// fetch the cluster again once it's ready so that it has everything up to date and then return it.
// For e.g. once the cluster has been updated, it contains information such as Version.GitVersion which it does not have before it's ready
func WaitUntilClusterIsReady(cluster *management.Cluster, client *rancher.Client) (*management.Cluster, error) {
	opts := metav1.ListOptions{FieldSelector: "metadata.name=" + cluster.ID, TimeoutSeconds: &defaults.WatchTimeoutSeconds}
	watchInterface, err := client.GetManagementWatchInterface(management.ClusterType, opts)
	if err != nil {
		return nil, err
	}

	watchFunc := clusters.IsHostedProvisioningClusterReady

	err = wait.WatchWait(watchInterface, watchFunc)
	if err != nil {
		return nil, err
	}
	return client.Management.Cluster.ByID(cluster.ID)
}

// GetGKEZone fetches the value of GKE zone;
// it first obtains the value from env var GKE_ZONE, if the value is empty, it fetches the information from config file(cattle_config-import.yaml/cattle_config-provisioning.yaml)
// if none of the sources can provide a value, it returns the default value
func GetGKEZone() string {
	zone := os.Getenv("GKE_ZONE")
	if zone == "" {
		gkeConfig := new(management.GKEClusterConfigSpec)
		config.LoadConfig("gkeClusterConfig", gkeConfig)
		if gkeConfig.Zone != "" {
			zone = gkeConfig.Zone
		}
		if zone == "" {
			zone = "asia-south2-c"
		}
	}
	return zone
}

// GetAKSLocation fetches the value of AKS Region;
// it first obtains the value from env var AKS_REGION, if the value is empty, it fetches the information from config file(cattle_config-import.yaml/cattle_config-provisioning.yaml)
// if none of the sources can provide a value, it returns the default value
func GetAKSLocation() string {
	region := os.Getenv("AKS_REGION")
	if region == "" {
		aksClusterConfig := new(management.AKSClusterConfigSpec)
		config.LoadConfig("aksClusterConfig", aksClusterConfig)
		region = aksClusterConfig.ResourceLocation
		if region == "" {
			region = "centralindia"
		}
	}
	return region
}

// GetEKSRegion fetches the value of EKS Region;
// it first obtains the value from env var EKS_REGION, if the value is empty, it fetches the information from config file(cattle_config-import.yaml/cattle_config-provisioning.yaml)
// if none of the sources can provide a value, it returns the default value
func GetEKSRegion() string {
	region := os.Getenv("EKS_REGION")
	if region == "" {
		eksClusterConfig := new(management.EKSClusterConfigSpec)
		config.LoadConfig("eksClusterConfig", eksClusterConfig)
		region = eksClusterConfig.Region
		if region == "" {
			region = "ap-south-1"
		}
	}
	return region
}

// GetGKEProjectID returns the value of GKE project by fetching the value of env var GKE_PROJECT_ID
func GetGKEProjectID() string {
	return os.Getenv("GKE_PROJECT_ID")
}
