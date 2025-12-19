# gsm-operator
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

Example `GSMSecret`:

```yaml
apiVersion: secrets.wayfair.com/v1alpha1
kind: GSMSecret
metadata:
  name: my-gsm-secrets
  namespace: app-namespace
spec:
  targetSecret:
    name: my-secret             # name of K8s Secret
  gsmSecrets:
    - key: MY_ENVVAR
      projectId: "wf-gcp-prod"  # GSM Secret project ID
      secretId: my-secret       # GSM secret name
      version: "latest"         # recommend pinning a version for true “static”
# status:
#   observedGeneration: 1
#   conditions:
#     - type: Ready
#       status: "True"
#       reason: Materialized
#       message: "Created Kubernetes Secret from GSM version 7"
```

Creates a secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-secret
type: Opaque
data:
  MY_ENVVAR: c2VjcmV0LXZhbHVl
```

Usage:

```yaml
...
    envFrom:
    - secretRef:
        name: my-secret
...
```

## Getting Started

### Prerequisites
- go version v1.24.6+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
***Prerequisites:**

1. The artifact registry exists.
2. You have permission to write to the registry.
3. Kyverno must be able to read the registry:

```sh
gcloud artifacts repositories add-iam-policy-binding gsm-operator \
    --project=wf-gcp-us-plat-gar-dev \
    --location=us \
    --role="roles/artifactregistry.reader" \
    --member="principal://iam.googleapis.com/projects/659149818238/locations/global/workloadIdentityPools/wf-gcp-us-plat-k8s-dev.svc.id.goog/subject/ns/kyverno/sa/kyverno-admission-controller"
```

**Setup:**

```sh
registry="us-docker.pkg.dev/wf-gcp-us-plat-gar-dev/gsm-operator"
tag=$(git rev-parse --short HEAD)
docker login us-docker.pkg.dev
```

**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=${registry}/gsm-operator:${tag}
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
kubectl config use-context "plat-dev-c1-dsm1"
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
kubectl config use-context "plat-dev-c1-dsm1"
make deploy IMG=${registry}/gsm-operator:${tag}
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**

The sample assumes GCP project `wf-gcp-us-cloud-plats-mcp-dev`, namespace `gsmsecret-test-ns` on `plat-dev-c1-dsm1`, and a secret called `bogus-test`.

1. Create a bogus secret if it does not exist:

```sh
printf "testing123" | gcloud secrets create bogus-test \
    --data-file=- \
    --project=wf-gcp-us-cloud-plats-mcp-dev \
    --replication-policy=automatic
```

2. Grant `gsmsecret-test-ns` permission to access the secret:

```sh
gcloud secrets add-iam-policy-binding bogus-test \
    --project=wf-gcp-us-cloud-plats-mcp-dev \
    --role="roles/secretmanager.secretAccessor" \
    --member="principal://iam.googleapis.com/projects/659149818238/locations/global/workloadIdentityPools/wf-gcp-us-plat-k8s-dev.svc.id.goog/subject/ns/gsmsecret-test-ns/sa/default"
```

3. You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=${registry}/gsm-operator:${tag}
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/gsm-operator/<tag or branch>/dist/install.yaml
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

