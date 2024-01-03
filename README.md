# HOSTED PROVIDERS E2E

## How to run a test locally:

### Environment Variables:
Following are the common environment variables that need to be exported for running a test:
1. RANCHER_HOSTNAME - Public DNS where rancher is running. E.g. ec2-1-2-3-4.ap-south-1.compute.amazonaws.com or 1.2.3.4.sslip.io
2. RANCHER_PASSWORD - Admin Password for login. We currently only test with 'admin' user.
3. CATTLE_TEST_CONFIG: Config file containing cluster and cloud credential information, for e.g. cattle-config-provisioning.yaml and cattle-config-import.yaml in the root directory.
4. PROVIDER: Type of the hosted provider you want to test. Acceptable values - gke, eks, aks

To run GKE:
1. GCP_CREDENTIALS - a Service Account with a JSON private key and provide the JSON here. These IAM roles are required:
   - Compute Engine: Compute Viewer (roles/compute.viewer)
   - Project: Viewer (roles/viewer)
   - Kubernetes Engine: Kubernetes Engine Admin (roles/container.admin)
   - Service Accounts: Service Account User (roles/iam.serviceAccountUser)

2. GKE_PROJECT_ID - Name of the Google Cloud Project

To run EKS:
1. AWS_ACCESS_KEY_ID - AWS Access Key
2. AWS_SECRET_ACCESS_KEY - AWS Secret Key
3. EKS_REGION - Default Region (default: us-west-2)

To run AKS:
1. AKS_CLIENT_ID - Azure Client ID [Check Microsoft Entra ID to create or fetch value from an existing one](https://learn.microsoft.com/en-us/entra/identity-platform/howto-create-service-principal-portal)
2. AKS_CLIENT_SECRET - Azure Client Secret [Check Microsoft Entra ID to create or fetch value from an existing one](https://learn.microsoft.com/en-us/entra/identity-platform/howto-create-service-principal-portal)
3. AKS_SUBSCRIPTION_ID - Azure Subscription ID (In this case it is similar to a Google Cloud Project, but the value is an ID). [Check Azure Subscriptions](https://learn.microsoft.com/en-us/microsoft-365/enterprise/subscriptions-licenses-accounts-and-tenants-for-microsoft-cloud-offerings?view=o365-worldwide#subscriptions)



### Makefile targets to run tests
1. `make e2e-provisioning-tests` - Covers the _P0Provisioning_ test suite for a given ${PROVIDER}
2. `make e2e-import-tests` - Covers the _P0Importing_ test suite for a given ${PROVIDER}
3. `make e2e-support-matrix-importing-tests` - Covers the _SupportMatrixImporting_ test suite for a given ${PROVIDER}
4. `make e2e-support-matrix-provisioning-tests` - Covers the _SupportMatrixProvisioning_ test suite for a given ${PROVIDER}

Run `make help` to know about other targets.

### Example
```shell
PROVIDER=gke RANCHER_HOSTNAME=ec2-1-2-3-4.ap-south-1.compute.amazonaws.com CATTLE_TEST_CONFIG=/home/pvala/go/src/github.com/rancher/hosted-providers-e2e/hosted/gke/config make e2e-provisioning-tests
```
