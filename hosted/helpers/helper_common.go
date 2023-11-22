package helpers

import (
	. "github.com/onsi/gomega"

	"github.com/rancher/rancher/tests/framework/clients/rancher"
	management "github.com/rancher/rancher/tests/framework/clients/rancher/generated/management/v3"
	"github.com/rancher/rancher/tests/framework/extensions/cloudcredentials"
	"github.com/rancher/rancher/tests/framework/extensions/cloudcredentials/aws"
	"github.com/rancher/rancher/tests/framework/extensions/cloudcredentials/azure"
	"github.com/rancher/rancher/tests/framework/extensions/cloudcredentials/google"
	"github.com/rancher/rancher/tests/framework/extensions/clusters"
	"github.com/rancher/rancher/tests/framework/pkg/session"
	"github.com/rancher/rancher/tests/framework/pkg/wait"
	"github.com/rancher/rancher/tests/v2prov/defaults"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

const (
	Timeout = 30 * time.Minute
)

type Context struct {
	CloudCred     *cloudcredentials.CloudCredential
	RancherClient *rancher.Client
	Session       *session.Session
}

func CommonBeforeSuite(cloud string) Context {
	testSession := session.NewSession()
	var cloudCredential *cloudcredentials.CloudCredential

	rancherClient, err := rancher.NewClient("", testSession)
	Expect(err).To(BeNil())

	switch cloud {
	case "aks":
		cloudCredential, err = azure.CreateAzureCloudCredentials(rancherClient)
		Expect(err).To(BeNil())
	case "eks":
		cloudCredential, err = aws.CreateAWSCloudCredentials(rancherClient)
		Expect(err).To(BeNil())
	case "gke":
		cloudCredential, err = google.CreateGoogleCloudCredentials(rancherClient)
		Expect(err).To(BeNil())
	}

	return Context{
		CloudCred:     cloudCredential,
		RancherClient: rancherClient,
		Session:       testSession,
	}
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
