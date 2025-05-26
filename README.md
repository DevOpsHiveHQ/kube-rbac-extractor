<div align="center">
  <p align="center">
    <a href="https://github.com/DevOpsHiveHQ/kube-rbac-extractor" style="display: block; padding: 1em 0;">
      <img width="265px" alt="Kustomize Merger Logo" border="0" src="img/kube-rbac-extractor-logo.svg"/>
    </a>
  </p>

  <h1>Kubernetes RBAC Extractor</h1>

  <p><b>
  A CLI tool generates Kubernetes RBAC Role/ClusterRole from K8s resources (manifests), applying the principle of least privilege (PoLP) for restricted security access.
  </b></p>
</div>

## Why?

For some use cases when a tight security access is required, users should only have access to the resources they need to interact with.

`kube-rbac-extractor` was created as no other tool that generates the Kubernetes RBAC Role/ClusterRole from K8s resources (manifests) without interacting with the K8s API server.

For example, you can use `kube-rbac-extractor` to limit the user's access to the kinds used in a specific Helm chart.

## Usage

```
Usage of kube-rbac-extractor:
  --access string
    	Access type: read, write, admin (default "read")
  --cluster
    	Generate ClusterRole instead of Role
  --extra-schema string
    	Path to extra kinds schema RBAC JSON file for custom resources
  --name string
    	Metadata name for the Role/ClusterRole (default "access")
  --namespace string
    	Namespace for Role (ignored for ClusterRole)
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

## Using the Docker image

You can build and run the docker image as follows:

```shell
docker build . -t kube-rbac-extractor

helm template dev oci://registry-1.docker.io/bitnamicharts/postgresql | 
  docker run --rm kube-rbac-extractor --access read --namespace dev --name developer-access
```

## License

Merger is an open-source software licensed under the MIT license. For more details, check the [LICENSE](LICENSE) file.
