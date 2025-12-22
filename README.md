# gsm-operator (from Kubebuilder)
GSMSecret is a static materialization operator that creates a Kubernetes
Secret from a Google Secret Manager (GSM) secret.

Purpose:
  - Bridge GSM to native Kubernetes Secrets in environments (e.g. Autopilot)
    where CSI drivers or node plugins cannot be used.
  - Allow workloads to consume GSM-managed secrets via standard Kubernetes
    mechanisms such as envFrom, env.valueFrom.secretKeyRef, or Secret volumes.

Behavior:
  - On creation (or spec change) of a GSMSecret resource, the operator fetches
    the specified GSM secret version and creates or updates a Kubernetes Secret.
  - No continuous sync or polling is performed; GSM changes are not propagated
    unless the GSMSecret resource itself is modified or recreated.
  - The operator runs entirely in the control plane using Workload Identity
    and does not install node-level binaries.

Tradeoffs:
  - Secrets are static once materialized.
  - Secret rotation requires an explicit user action (e.g. version bump or
    resource recreation).

`gsm-operator` manages `GSMSecret` custom resources that materialize Google Secret Manager entries into Kubernetes `Secret` objects.

## Configuration

To configure environment variables used by the setup examples:

```sh
cp env.sample .env          # copy the template
# Modify the file
. .env
```

## Basic Functionality

Example `GSMSecret`:

```yaml
apiVersion: secrets.pize.com/v1alpha1
kind: GSMSecret
metadata:
  name: my-gsm-secrets
  namespace: gsmsecret-test-ns
  annotations:
    # Optional unless not set on the operator by env var WIFAUDIENCE
    secrets.pize.com/wif-audience: "//iam.googleapis.com/projects/${oidc_project_number}/locations/global/workloadIdentityPools/gsm-operator-pool/providers/gsm-operator-provider" # oidc_project_number is defined below
    # secrets.pize.com/ksa: "custom-ksa" # optional: override Kubernetes SA used for WIF
    # secrets.pize.com/gsa: "my-gsa@example.iam.gserviceaccount.com" # optional: override GCP SA when impersonation is enabled
spec:
  targetSecret:
    name: my-secret             # name of K8s Secret
  gsmSecrets:
    - key: MY_ENVVAR
      projectId: "gcp-proj-id"  # GSM Secret project ID
      secretId: my-secret       # GSM secret name
      version: "1"              # recommend pinning a version for true “static”
```

Creates a secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-secret
type: Opaque
data:
  MY_ENVVAR: c2VjcmV0LXZhbHVl # base64 encoded
```

Usage:

```yaml
...
    envFrom:
    - secretRef:
        name: my-secret
...
```

### Reconciliation Triggers

The controller uses predicates to optimize when reconciliation occurs, avoiding unnecessary work:

| Change Type | Triggers Reconcile? |
|-------------|---------------------|
| `GSMSecret` `.spec` changes | Yes |
| `secrets.pize.com/ksa` annotation changed | Yes |
| `secrets.pize.com/gsa` annotation changed | Yes |
| `secrets.pize.com/wif-audience` annotation changed | Yes |
| `secrets.pize.com/release` annotation changed | Yes |
| `GSMSecret` status-only update | No |
| `GSMSecret` label changes | No |
| Other annotation changes (e.g., `kubectl.kubernetes.io/last-applied-configuration`) | No |
| Owned `Secret` data/type changed | Yes |
| Owned `Secret` metadata-only update | No |

The controller also requeues every 5 minutes (FIXME) to pick up changes in Google Secret Manager.

### OIDC and wifAudience

The Operator functions as an identity broker using a Dynamic Impersonation pattern. Instead of using its own broad permissions, the Operator explicitly requests a short-lived token for the tenant's Kubernetes Service Account (`default` by default). It then exchanges this token via Google STS (OIDC) to access Secret Manager resources scoped specifically to that tenant identity. Because GKE's "Native" Workload Identity is a managed implementation designed to be "magic" and opaque (i.e., The native GKE integration does not expose a public Workload Identity Pool Provider resource for manual token exchange), we have to leverage Workload Identity Pools for non-trivial security.

Build Workload Identity Pool & Provider:

```sh
### 1. Get Project Numbers
cluster_project_number=$(gcloud projects describe "${CLUSTER_PROJECT_ID}" --format='value(projectNumber)')
oidc_project_number=$(gcloud projects describe "${OIDC_PROJECT_ID}" --format='value(projectNumber)')

### 2. Get Cluster OIDC Issuer URL
cluster_oidc_url="https://container.googleapis.com/v1/projects/${CLUSTER_PROJECT_ID}/locations/${CLUSTER_REGION}/clusters/${CLUSTER_NAME}"

### 3. Create Pool & Provider
gcloud iam workload-identity-pools create gsm-operator-pool \
    --location="global" \
    --display-name="GSM Operator Pool" \
    --project="${OIDC_PROJECT_ID}"

gcloud iam workload-identity-pools providers create-oidc gsm-operator-provider \
    --location="global" \
    --workload-identity-pool="gsm-operator-pool" \
    --issuer-uri="${cluster_oidc_url}" \
    --attribute-mapping="google.subject=assertion.sub" \
    --project="${OIDC_PROJECT_ID}"

### 4. Output wifAudience
export WIF_AUDIENCE="//iam.googleapis.com/projects/${oidc_project_number}/locations/global/workloadIdentityPools/gsm-operator-pool/providers/gsm-operator-provider"
echo "wifAudience is $WIF_AUDIENCE"

### 5. principal
echo IAM Binding Guidance
echo "principal://iam.googleapis.com/projects/${oidc_project_number}/locations/global/workloadIdentityPools/gsm-operator-pool/subject/system:serviceaccount:gsmsecret-test-ns:default"

echo "... or Bind this Principal to your GSA if using Service Account impersonation (not recommended nor implemented yet):"
echo "principal://iam.googleapis.com/projects/${oidc_project_number}/locations/global/workloadIdentityPools/gsm-operator-pool/subject/system:serviceaccount:gsmsecret-test-ns:default"
```

To **validate** that the Workload Identity Pool and Provider were created:

```sh
gcloud iam workload-identity-pools describe gsm-operator-pool \
    --location="global" \
    --project="${OIDC_PROJECT_ID}"

gcloud iam workload-identity-pools providers describe gsm-operator-provider \
    --location="global" \
    --workload-identity-pool="gsm-operator-pool" \
    --project="${OIDC_PROJECT_ID}"
```

To **remove** the Workload Identity Pool and Provider created above:

```sh
gcloud iam workload-identity-pools providers delete gsm-operator-provider \
    --location="global" \
    --workload-identity-pool="gsm-operator-pool" \
    --project="${OIDC_PROJECT_ID}"

gcloud iam workload-identity-pools delete gsm-operator-pool \
    --location="global" \
    --project="${OIDC_PROJECT_ID}"
```

# Install & Run Sample

### Prerequisites
- go version v1.24.6+
- docker version 17.03+.
- docker buildx v0.30.1+ (if using ARM)
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
***Prerequisites:**

1. The artifact registry exists.
2. You have permission to write to the registry, deploy to GKE, etc.
3. Kubectl context is configured: `kubectl config use-context <my-context>` and `kubectl config set-context --current --namespace="gsmsecret-test-ns"` 
4. OIDC, IAM, & Secrets have been configured.

***Assumptions***

The sample assumes GCP project `${SECRETS_PROJECT_ID}`, namespace `gsmsecret-test-ns` on `${CLUSTER_NAME}`, and a secret called `bogus-test` (created in the prerequisites above).

**Required for test** Simple configuration assumed with the test install:

1. Create a GSM Secret and grant access:

```sh
# Create a secret
printf "testing123" | gcloud secrets create bogus-test \
    --data-file=- \
    --project=${SECRETS_PROJECT_ID} \
    --replication-policy=automatic

# Grant access if not using GSA impersonation
gcloud secrets add-iam-policy-binding bogus-test \
    --project=${SECRETS_PROJECT_ID} \
    --role="roles/secretmanager.secretAccessor" \
    --member="principal://iam.googleapis.com/projects/${oidc_project_number}/locations/global/workloadIdentityPools/gsm-operator-pool/subject/system:serviceaccount:gsmsecret-test-ns:default"
```

**Optional** if using GSA impersonation, then:

1. Setup the SA, grant IAM role, and add the IAM binding to the secret:

```sh
project_id="<project where this GSA lives>"
sa_email="my-sa@${project_id}.iam.gserviceaccount.com"

# Create the GSA
gcloud iam service-accounts create my-sa \
  --project=${project_id} \
  --display-name="my-sa"

# The OIDC principal
principal="principal://iam.googleapis.com/projects/${oidc_project_number}/locations/global/workloadIdentityPools/gsm-operator-pool/subject/system:serviceaccount:gsmsecret-test-ns:default"

# (Optional sanity check) confirm the GSA exists
gcloud iam service-accounts describe "${sa_email}" --project="${project_id}"

gcloud iam service-accounts add-iam-policy-binding "${sa_email}" \
  --project="${project_id}" \
  --role="roles/iam.serviceAccountTokenCreator" \
  --member="${principal}"

# Verify the binding landed
gcloud iam service-accounts get-iam-policy "roles/iam.serviceAccountTokenCreator" \
  --project="${project_id}" \
  --format="yaml"

# Grant the GSA access to the GSM Secret
gcloud secrets add-iam-policy-binding bogus-test \
    --project=${SECRETS_PROJECT_ID} \
    --role="roles/secretmanager.secretAccessor" \
    --member="serviceAccount:${sa_email}"
```

2. Add the annotation `secrets.pize.com/gsa: "${sa_email}"` to `config/samples/secrets.pize.com_v1alpha1_gsmsecret.yaml` on `GSMSecret`.

### Setup

**Build and push your image to the location specified by `IMG`:**

**For arm64**, to build **only** `linux/amd64`, override `PLATFORMS` (see `Makefile` `docker-buildx` target):

```sh
make docker-buildx IMG=${REGISTRY}/gsm-operator:${TAG} PLATFORMS=linux/amd64
```

Otherwise:

```sh
make docker-build docker-push IMG=${REGISTRY}/gsm-operator:${TAG}
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=${REGISTRY}/gsm-operator:${TAG}
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**

1. You can apply the samples (examples) from the config/sample:

```sh
envsubst < config/samples/secrets.pize.com_v1alpha1_gsmsecret.yaml | kubectl apply -f -
```

2. Verify the secret was created:

```sh
kubectl get Secret my-secret  -o yaml
```

>**NOTE**: Ensure that the samples have default values to test them out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
envsubst < config/samples/secrets.pize.com_v1alpha1_gsmsecret.yaml | kubectl delete -f -
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**Undeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=${REGISTRY}/gsm-operator:${TAG}
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/zeraholladay/gsm-operator/main/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v2-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing
TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2025 Zera Holladay.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

