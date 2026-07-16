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

## Local Verification

Verified end-to-end on a local `kind` cluster. Samples live in `config/samples/`.

```sh
# Setup: cluster, CRDs, and run the controller locally (keep it running)
kind create cluster --name nsclass-test
make install
make run
```

**1. Join a class → resources created**

```sh
kubectl apply -f config/samples/public-network-nsclass.yaml
kubectl apply -f config/samples/internal-network-nsclass.yaml
kubectl apply -f config/samples/web-portal-namespace.yaml
kubectl get networkpolicy,configmap -n web-portal   # allow-all, public-config created
```

**2. Switch class → old deleted, new created**

```sh
kubectl label namespace web-portal namespaceclass.akuity.io/name=internal-network --overwrite
kubectl get networkpolicy,configmap -n web-portal   # public-* gone, internal-* created
```

**3. Update a class → existing namespaces sync**

```sh
# add a resource to internal-network-nsclass.yaml, then:
kubectl apply -f config/samples/internal-network-nsclass.yaml   # new resource appears in web-portal
```

**4. Cluster-scoped resources are skipped** — adding a `ClusterRole` to a class logs
`skipping cluster-scoped resource ...` and does not create it.

```sh
# inspect the ledger of managed resources:
kubectl get ns web-portal -o jsonpath='{.metadata.annotations.namespaceclass\.akuity\.io/managed-resources}'
```

## Prerequisites

- Go 1.24+
- Docker (for running a local `kind` cluster)
- `kind`, `kubectl`
