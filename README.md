<div align="center">
  <p align="center">
    <a href="https://github.com/DevOpsHiveHQ/kube-rbac-extractor" style="display: block; padding: 1em 0;">
      <img width="265px" alt="K8s RBAC Extractor Logo" border="0" src="img/kube-rbac-extractor-logo.svg"/>
    </a>
  </p>

  <h1>Kubernetes RBAC Extractor</h1>

  <p><b>
  A CLI tool generates Kubernetes RBAC Role/ClusterRole from K8s resources (manifests), applying the principle of least privilege (PoLP) for restricted security access.
  </b></p>

[![CI](https://img.shields.io/github/actions/workflow/status/DevOpsHiveHQ/kube-rbac-extractor/.github%2Fworkflows%2Fgo-ci.yml?logo=github&label=CI&color=31c653)](https://github.com/DevOpsHiveHQ/kube-rbac-extractor/actions/workflows/go-ci.yml?query=branch%3Amain)
[![Go Report Card](https://goreportcard.com/badge/github.com/DevOpsHiveHQ/kube-rbac-extractor)](https://goreportcard.com/report/github.com/DevOpsHiveHQ/kube-rbac-extractor)
[![GitHub Release](https://img.shields.io/github/v/release/DevOpsHiveHQ/kube-rbac-extractor?logo=github)](https://github.com/DevOpsHiveHQ/kube-rbac-extractor/releases)
[![Docker](https://img.shields.io/badge/Docker-available-blue?logo=docker&logoColor=white)](https://github.com/DevOpsHiveHQ/kube-rbac-extractor/pkgs/container/kustomize-generator-merger)
[![Go Reference](https://pkg.go.dev/badge/github.com/DevOpsHiveHQ/kube-rbac-extractor.svg)](https://pkg.go.dev/github.com/DevOpsHiveHQ/kube-rbac-extractor)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/DevOpsHiveHQ/kube-rbac-extractor/pulls)

</div>

## Why?

For some use cases when a tight security access is required, users should only have access to the resources they need to interact with.

`kube-rbac-extractor` was created as no other tool that generates the Kubernetes RBAC Role/ClusterRole from K8s resources (manifests) without interacting with the K8s API server.

For example, you can use `kube-rbac-extractor` to limit the user's access to the kinds used in a specific Helm chart.

## Installation

Download pre-compiled binary from [GitHub releases](https://github.com/DevOpsHiveHQ/kube-rbac-extractor/releases) page, or use Docker image:

```
ghcr.io/devopshivehq/kube-rbac-extractor:latest
```

## Usage

```
Usage of kube-rbac-extractor:
  --access string
    	Access type: read, write, admin (default "read")
  --cluster
    	Generate ClusterRole instead of Role
  --extra-schema string
    	Path to extra kinds RBAC schema JSON file for custom resources
  --name string
    	Metadata name for the Role/ClusterRole (default "access")
  --namespace string
    	Namespace for Role (ignored for ClusterRole)
  --resource-names
    	Include resourceNames from manifest metadata.name in the rules
  --role-binding-subjects string
    	Generate RoleBinding/ClusterRoleBinding using comma-separated list of subjects to bind the role to
      (e.g., User:alice,Group:devs,ServiceAccount:ns:sa)
```

## Example

Run:

```shell
helm template dev oci://registry-1.docker.io/bitnamicharts/postgresql | 
  kube-rbac-extractor --access read --namespace dev --name developer-access
```

Output:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: developer-access
  namespace: dev
rules:
  - apiGroups:
    - networking.k8s.io
    resources:
    - networkpolicies
    verbs:
    - get
    - list
    - watch
  - apiGroups:
    - policy
    resources:
    - poddisruptionbudgets
    verbs:
    - get
    - list
    - watch
  - apiGroups:
    - ""
    resources:
    - serviceaccounts
    verbs:
    - get
    - list
    - watch
  - apiGroups:
    - ""
    resources:
    - secrets
    verbs:
    - get
    - list
    - watch
  - apiGroups:
    - ""
    resources:
    - services
    verbs:
    - get
    - list
    - watch
  - apiGroups:
    - apps
    resources:
    - statefulsets
    verbs:
    - get
    - list
    - watch
```

## License

Merger is an open-source software licensed under the MIT license. For more details, check the [LICENSE](LICENSE) file.
