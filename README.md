<!-- markdownlint-disable MD033 -->
<h1>
    <img src="docs/assets/logo.svg" align="left" width="90" style="margin-right: 15px"/>
    Gatekeeper Policy Manager (GPM)
</h1>
<!-- markdownlint-enable MD033 -->

![GPM Release](https://img.shields.io/github/v/tag/sighupio/gatekeeper-policy-manager?filter=v*&sort=semver&label=GPM&color=blue)
![Helm Chart Release](https://img.shields.io/badge/dynamic/yaml?url=https%3A%2F%2Fraw.githubusercontent.com%2Fsighupio%2Fgatekeeper-policy-manager%2Fmain%2Fchart%2FChart.yaml&query=%24.version&label=Helm%20Chart&prefix=v&color=blue)
![License](https://img.shields.io/github/license/sighupio/gatekeeper-policy-manager)

**Gatekeeper Policy Manager** is a *read-only* web UI that shows the status of OPA Gatekeeper policies in a Kubernetes cluster.

The target Kubernetes cluster can be the same one where GPM runs, or [a remote cluster that GPM connects to with a `kubeconfig` file](docs/multi-cluster.md). You can also run GPM [locally on a client machine](#running-locally) and connect to a remote cluster.

GPM lets you see in detail:

- **Constraint Templates** with their rego code.
- **Constraints** with their current status, violations, enforcement action, matches definitions, etc.
- **Mutations** defined and their details.
- **Events** emitted by OPA Gatekeeper (alpha feature).
- Gatekeeper **Configuration** custom resource values.

[You can see some screenshots below ⤵](#screenshots).

## Requirements

GPM needs OPA Gatekeeper in your cluster. It also needs some constraint templates and constraints. Without them, GPM has nothing to show.

> [!TIP]
> You can deploy Gatekeeper to your cluster with the [SIGHUP Distribution Policy Module](https://github.com/sighupio/module-policy) (also open source).

## Deploying GPM

### Deploy using Kustomize

To deploy Gatekeeper Policy Manager to your cluster, apply the [`kustomization`](kustomization.yaml) file with this command:

```shell
kubectl apply -k https://github.com/sighupio/gatekeeper-policy-manager
```

By default, this creates a deployment and a service named `gatekeeper-policy-manager` in the `gatekeeper-system` namespace. To configure more, see the `kustomization.yaml` file.

> [!NOTE]
> GPM can run as a Pod in a Kubernetes cluster, or locally with a `kubeconfig` file. It autodetects the correct configuration.

If you did not configure an ingress, use port-forward to access the web UI:

```bash
kubectl -n gatekeeper-system port-forward  svc/gatekeeper-policy-manager 8080:80
```

Then open [http://127.0.0.1:8080](http://127.0.0.1:8080) in your browser.

### Deploy using Helm

You can also deploy GPM with the [Helm chart](./chart).

First create a values file, for example `my-values.yaml`, with your custom values for the release. See the [chart's readme](./chart/README.md) and the [default values.yaml](./chart/values.yaml) for more information.

From `v2.1.0` the chart is published as an OCI artifact on `quay.io`, next to the container image. There is no `helm repo add` step any more. You need Helm 3.8 or later, which supports OCI registries. Then execute:

```bash
helm upgrade --install --namespace gatekeeper-system --set image.tag=v2.1.0 --values my-values.yaml gatekeeper-policy-manager oci://quay.io/sighup/charts/gatekeeper-policy-manager --version 0.20.0
```

> [!IMPORTANT]
> Replace `my-values.yaml` with the path to your values file, and `--version 0.20.0` with the chart version you want.

## Running locally

You can also run GPM locally with Docker (or another container runtime) and a `kubeconfig`. If the `kubeconfig` file is at `~/.kube/config`, run this command:

```bash
docker run -v ~/.kube/config:/home/nonroot/.kube/config -p 8080:8080 quay.io/sighup/gatekeeper-policy-manager:v2.1.0
```

Then open [http://127.0.0.1:8080](http://127.0.0.1:8080) in your browser.

You can also run the app binary directly. See the [development section](#development) for more information.

## Configuration

GPM is stateless. You configure it with environment variables. The most common ones are:

| Env Var Name | Description | Default |
| --- | --- | --- |
| `GPM_LOG_LEVEL` | Log level (`DEBUG`, `INFO`, `WARN`, `ERROR`) | `INFO` |
| `GPM_AUTH_ENABLED` | Set to `OIDC` or `JWT` to protect GPM. | `Anonymous` |
| `KUBECONFIG` | Path to a `kubeconfig` file. If you set it in a cluster, GPM uses it instead of the cluster's API. | `$HOME/.kube/config` |

For more information, read these documents:

- [Configuration](docs/configuration.md): all the environment variables, subpath deployment, and events RBAC.
- [Authentication](docs/authentication.md): OIDC login, or an authenticating proxy such as Pomerium.
- [RBAC-aligned views](docs/rbac-aligned-views.md): show each person only what their Kubernetes account can read.
- [Multi-cluster support](docs/multi-cluster.md): one GPM for more than one cluster, and AWS IAM authentication.

## Screenshots

<!-- markdownlint-disable MD033 -->
<a href="screenshots/home.png"><img src="screenshots/home.png" width="250"/></a>
<a href="screenshots/constraint-templates-01.png"><img src="screenshots/constraint-templates-01.png" width="250"/></a>
<a href="screenshots/constraint-templates-02.png"><img src="screenshots/constraint-templates-02.png" width="250"/></a>
<a href="screenshots/constraints-01.png"><img src="screenshots/constraints-01.png" width="250"/></a>
<a href="screenshots/constraints-02.png"><img src="screenshots/constraints-02.png" width="250"/></a>
<a href="screenshots/violations-report.png"><img src="screenshots/violations-report.png" width="250"/></a>
<a href="screenshots/mutations.png"><img src="screenshots/mutations.png" width="250"/></a>
<a href="screenshots/events.png"><img src="screenshots/events.png" width="250"/></a>
<a href="screenshots/configurations.png"><img src="screenshots/configurations.png" width="250"/></a>
<!-- markdownlint-enable MD033 -->

## Development

GPM is written in Go. It uses the Echo framework and renders the UI on the server with the standard
library's `html/template`, plus a small amount of Alpine.js for interactivity. There is no separate
frontend build.

Alpine.js is vendored as `static/ssr/alpine.min.js`. Its version is pinned in `package.json` so
Dependabot tracks it; after a bump, run `mise run vendor-alpine` to refresh the vendored file (the
`check-alpine-version` task, part of `mise run lint`, fails if the two drift).

To develop GPM, run these commands:

```bash
# Install the dependencies
$ go mod download
# Run the development server
$ GPM_LOG_LEVEL=DEBUG go run .
```

> [!TIP]
> A Kubernetes cluster with OPA Gatekeeper deployed helps you debug the application.

## Contributing

Let us know if you use GPM and which features you want. Create an issue here on GitHub 💪🏻

To contribute, pick one of the open issues and work on it. It is better to tell us first on the issue.

When you are happy with your work, open a Pull Request.

> We try to stick to [conventional commits](https://www.conventionalcommits.org/en/v1.0.0/) when writing commit messages.
