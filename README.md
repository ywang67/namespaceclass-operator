# namespaceclass-operator

A Kubernetes operator that lets cluster admins define reusable **namespace classes**.
A `NamespaceClass` is a cluster-scoped resource that declares a set of resources
(NetworkPolicies, ConfigMaps, ResourceQuotas, ...) which should automatically exist
in any namespace of that class. A namespace opts into a class via a label; the
operator keeps the namespace's managed resources in sync with the class definition.

## Description

Admins create `NamespaceClass` objects, each listing the resources a namespace of
that class should have:

```yaml
apiVersion: namespaceclass.akuity.io/v1alpha1
kind: NamespaceClass
metadata:
  name: public-network
spec:
  resources:
    - apiVersion: networking.k8s.io/v1
      kind: NetworkPolicy
      metadata:
        name: allow-all
      spec:
        podSelector: {}
        ingress:
          - {}
```

A namespace joins a class with the label `namespaceclass.akuity.io/name: <class>`:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: web-portal
  labels:
    namespaceclass.akuity.io/name: public-network
```

The controller then creates `allow-all` inside `web-portal`, and keeps it in sync
as the namespace switches classes or the class definition changes.

### Design

- **Reconcile target is the Namespace, not the NamespaceClass.** The desired state
  ("this namespace should contain exactly the class's resources") belongs to each
  namespace, and one class maps to many namespaces. The controller uses
  `For(&Namespace{})` plus `Watches(&NamespaceClass{})`: a class change is mapped to
  all namespaces referencing it, so both triggers converge on "reconcile a namespace".
- **Bookkeeping via an annotation ledger.** After reconciling, the controller records
  the resources it created in the namespace annotation
  `namespaceclass.akuity.io/managed-resources`. On the next reconcile it diffs the
  ledger (what it created last time) against the current class (what it should create
  now) and deletes the difference. This is how switching classes and shrinking a class
  correctly delete stale resources — the current class alone doesn't reveal what used
  to exist.
- **Arbitrary resource kinds.** Resources are stored as `runtime.RawExtension` and
  applied via an unstructured client, so any kind is supported. To grant the operator
  permission for unknown kinds, its ClusterRole uses wildcard RBAC
  (`groups=*, resources=*`). In production this should be narrowed to the kinds
  actually used.
- **Namespaced resources only.** A NamespaceClass provisions per-namespace resources,
  so cluster-scoped resources (ClusterRole, PersistentVolume, ...) are intentionally
  **skipped** with a warning: they would be shared across every namespace of the class,
  and deleting one on a class switch would break the others. The controller detects
  scope at runtime via the RESTMapper.

## Local Verification

The scenarios below were verified end-to-end on a local `kind` cluster. They map
directly to the four requirements.

```sh
# 0. Cluster + CRDs + run the controller locally
kind create cluster --name nsclass-test
make install
make run          # leave this running in its own terminal
```

The sample manifests live in `config/samples/`:
`public-network-nsclass.yaml`, `internal-network-nsclass.yaml`, `web-portal-namespace.yaml`.

**1. Create resources when a namespace joins a class**

```sh
kubectl apply -f config/samples/public-network-nsclass.yaml
kubectl apply -f config/samples/internal-network-nsclass.yaml
kubectl apply -f config/samples/web-portal-namespace.yaml

kubectl get networkpolicy,configmap -n web-portal
# => allow-all (NetworkPolicy) and public-config (ConfigMap) are created
```

**2. Switch classes (create new + delete old)**

```sh
kubectl label namespace web-portal namespaceclass.akuity.io/name=internal-network --overwrite

kubectl get networkpolicy,configmap -n web-portal
# => public-network's resources are deleted, internal-network's are created
```

**3. Update a class definition (existing namespaces sync)**

Edit `internal-network-nsclass.yaml` to add another resource, then:

```sh
kubectl apply -f config/samples/internal-network-nsclass.yaml
# without touching the namespace, the new resource appears in web-portal
```

**4. Cluster-scoped resources are skipped**

Adding a `ClusterRole` to a class and applying it produces a controller log
`skipping cluster-scoped resource ...`, and the ClusterRole is not created.

Inspect the ledger at any point:

```sh
kubectl get ns web-portal \
  -o jsonpath='{.metadata.annotations.namespaceclass\.akuity\.io/managed-resources}'
```

## Getting Started

### Prerequisites
- go version v1.24.6+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/namespaceclass-operator:tag
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
make deploy IMG=<some-registry>/namespaceclass-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

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
make build-installer IMG=<some-registry>/namespaceclass-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/namespaceclass-operator/<tag or branch>/dist/install.yaml
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
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

