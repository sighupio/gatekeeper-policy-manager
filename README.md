<!-- markdownlint-disable MD033 -->
<h1>
    <img src="docs/assets/logo.svg" align="left" width="90" style="margin-right: 15px"/>
    Gatekeeper Policy Manager (GPM)
</h1>
<!-- markdownlint-enable MD033 -->

[![Build Status](https://ci.sighup.io/api/badges/sighupio/gatekeeper-policy-manager/status.svg)](https://ci.sighup.io/sighupio/gatekeeper-policy-manager)
![GPM Release](https://img.shields.io/badge/GPM-v1.0.3-blue)
![Helm Chart Release](https://img.shields.io/badge/Helm%20Chart-v0.4.1-blue)
![Slack](https://img.shields.io/badge/slack-@kubernetes/fury-yellow.svg?logo=slack)
![License](https://img.shields.io/github/license/sighupio/gatekeeper-policy-manager)

**Gatekeeper Policy Manager** is a simple *read-only* web UI for viewing OPA Gatekeeper policies' status in a Kubernetes Cluster.

The target Kubernetes Cluster can be the same where GPM is running or some other [remote cluster(s) using a `kubeconfig` file](#multi-cluster-support). You can also run GPM [locally in a client machine](#running-locally) and connect to a remote cluster.

GPM can display all the defined **Constraint Templates** with their rego code, all the Gatekeeper Configuration CRDs, and all the **Constraints** with their current status, violations, enforcement action, matches definitions, etc.

[You can see some screenshots below](#screenshots).

## Requirements

You'll need OPA Gatekeeper running in your cluster and at least some constraint templates and constraints defined to take advantage of this tool.

ℹ You can easily deploy Gatekeeper to your cluster using the (also open source) [Kubernetes Fury OPA](https://github.com/sighupio/fury-kubernetes-opa) module.

## Deploying GPM

### Deploy using Kustomize

To deploy Gatekeeper Policy Manager to your cluster, apply the provided [`kustomization`](kustomization.yaml) file running the following command:

```shell
kubectl apply -k .
```

By default, this will create a deployment and a service both with the name `gatekeper-policy-manager` in the `gatekeeper-system` namespace. We invite you to take a look into the `kustomization.yaml` file to do further configuration.

> The app can be run as a POD in a Kubernetes cluster or locally with a `kubeconfig` file. It will try its best to autodetect the correct configuration.

Once you've deployed the application, if you haven't set up an ingress, you can access the web UI using port-forward:

```bash
kubectl -n gatekeeper-system port-forward  svc/gatekeeper-policy-manager 8080:80
```

Then access it with your browser by visiting [http://127.0.0.1:8080](http://127.0.0.1:8080).

### Deploy using Helm

It is also possible to deploy GPM using the [provided Helm Chart](./chart).

First create a values file, for example `my-values.yaml`, with your custom values for the release. See the [chart's readme](./chart/README.md) and the [default values.yaml](./chart/values.yaml) for more information.

Then, execute:

```bash
helm repo add gpm https://sighupio.github.io/gatekeeper-policy-manager
helm upgrade --install --namespace gatekeeper-system --set image.tag=v1.0.3 --values my-values.yaml gatekeeper-policy-manager gpm/gatekeeper-policy-manager
```

> don't forget to replace `my-values.yaml` with the path to your values file.

## Running locally

GPM can also be run locally using Docker (or any other container runtime) and a `kubeconfig`. Assuming that the `kubeconfig` file you want to use is located at `~/.kube/config` the command to run GPM locally would be:

```bash
docker run -v ~/.kube/config:/home/nonroot/.kube/config -p 8080:8080 quay.io/sighup/gatekeeper-policy-manager:v1.0.3
```

Then access it with your browser by visiting [http://127.0.0.1:8080](http://127.0.0.1:8080).

> You can also run the app binary directly, see the [development section](#development) for further information.

## Configuration

GPM is a stateless application, but it can be configured using environment variables. The possible configurations are:

| Env Var Name                      | Description                                                                                                                                                                                                                       | Default                |
| --------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------- |
| `GPM_AUTH_ENABLED`                | Enable Authentication current options: "Anonymous", "OIDC"                                                                                                                                                                        | Anonymous              |
| `GPM_SECRET_KEY`                  | The secret key used to generate tokens. **Change this value in production**.                                                                                                                                                      | `g8k1p3rp0l1c7m4n4g3r` |
| `GPM_PREFERRED_URL_SCHEME`        | URL scheme to be used while generating links.                                                                                                                                                                                     | `http`                 |
| `GPM_OIDC_REDIRECT_DOMAIN`        | The server name under the app is being exposed. This is where the client will be redirected after authenticating                                                                                                                  |                        |
| `GPM_OIDC_ISSUER`                 | OIDC Issuer hostname                                                                                                                                                                                                              |                        |
| `GPM_OIDC_AUTHORIZATION_ENDPOINT` | OIDC Authorizatoin Endpoint                                                                                                                                                                                                       |                        |
| `GPM_OIDC_JWKS_URI`               | OIDC JWKS URI                                                                                                                                                                                                                     |                        |
| `GPM_OIDC_TOKEN_ENDPOINT`         | OIDC TOKEN Endpoint                                                                                                                                                                                                               |                        |
| `GPM_OIDC_INTROSPECTION_ENDPOINT` | OIDC Introspection Enpoint                                                                                                                                                                                                        |                        |
| `GPM_OIDC_USERINFO_ENDPOINT`      | OIDC Userinfo Endpoint                                                                                                                                                                                                            |                        |
| `GPM_OIDC_END_SESSION_ENDPOINT`   | OIDC End Session Endpoint                                                                                                                                                                                                         |                        |
| `GPM_OIDC_CLIENT_ID`              | The Client ID used to authenticate against the OIDC Provider                                                                                                                                                                      |                        |
| `GPM_OIDC_CLIENT_SECRET`          | The Client Secret used to authenticate against the OIDC Provider                                                                                                                                                                  |                        |
| `GPM_LOG_LEVEL`                   | Log level (see [python logging docs](https://docs.python.org/2/library/logging.html#levels) for available levels)                                                                                                                 | `INFO`                 |
| `KUBECONFIG`                      | Path to a [kubeconfig](https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/) file, if provided while running inside a cluster this configuration file will be used instead of the cluster's API. |

> ⚠️ Please notice that OIDC Authentication is in beta state. It has been tested to work with Keycloak as a provider.
>
> These environment variables are already provided and ready to be set in the [`manifests/enable-oidc.yaml`](manifests/enable-oidc.yaml) file.

### Multi-cluster support

Since `v1.0.3` GPM supports viewing information from more than one cluster. Multi-cluster support is achieved by using a `kubeconfig` with more than one context, where each context points to a different cluster. GPM will let you choose the context (cluster) right from the UI.

If you want to run GPM in a cluster but with multi-cluster support, it is as easy as

1. Mounting a `kubeconfig` file in GPM's pod(s) with the cluster access configuration.
2. Setting the environment variable `KUBECONFIG` value with the path to the mounted `kubeconfig` file. Or you can simply mount it in `/home/nonroot/.kube/config` and GPM will detect it automatically.

> ⚠️ Please remember that the user for the clusters should have the right permissions. You can use the [`manifests/rabc.yaml`](manifests/rbac.yaml) file as reference.
>
> Also note that the cluster where GPM is running should be able to reach the other clusters, i.e. network connectivity.

When you run GPM locally, you are already using a `kubeconfig` file to connect to the clusters, you should see all your defined contexts and be able to switch between them easily from the UI.

#### AWS IAM Authentication

If you want to use a Kubeconfig with IAM Authentication, you'll need to customize GPM's container image because the IAM authentication uses external AWS binaries that are not included by default in the image.

You can customize the container image with a `Dockerfile` like the following:

```Dockerfile
FROM curlimages/curl:7.81.0 as downloader
RUN curl https://github.com/kubernetes-sigs/aws-iam-authenticator/releases/download/v0.5.5/aws-iam-authenticator_0.5.5_linux_amd64 --output /tmp/aws-iam-authenticator
RUN chmod +x /tmp/aws-iam-authenticator

FROM quay.io/sighup/gatekeeper-policy-manager:v1.0.3
COPY --from=downloader --chown=root:root /tmp/aws-iam-authenticator /usr/local/bin/
```

You may need to add also the `aws` CLI for debugging purposes, you can use the same approach as before.

> ℹ️ Make sure that your `kubeconfig` has the `apiVersion` set as `client.authentication.k8s.io/v1beta1`
>
> You can read more [in this issue](https://github.com/sighupio/gatekeeper-policy-manager/issues/330).

## Screenshots

![welcome](screenshots/01-home.png)

![Constraint Templates view](screenshots/02-constrainttemplates.png)

![Constraint Templates view rego code](screenshots/03-constrainttemplates.png)

![Constraint view](screenshots/04-constraints.png)

![Constraint view 2](screenshots/05-constraints.png)

![Constraint Report 3](screenshots/06-constraints.png)

![Configurations view 2](screenshots/07-configs.png)

![Cluster Selector](screenshots/08-multicluster.png)

## Development

GPM is written in Go using the Echo framework for the backend and React with Elastic UI and the Fury theme for the frontend.

To develop GPM, the following commands should get you ready to start hacking:

```bash
# Build Frontend and copy over to static folder
$ pushd web-client
$ yarn install && yarn build
$ cp -r build/* ../static-content/
$ popd
# Install the Backend dependencies
$ go mod download
# Run the development server
$ go run main.go
```

> 💡 Access to a Kubernetes cluster with OPA Gatekeeper deployed is recommended to debug the application.
>
> You'll need an OIDC provider to test the OIDC authentication. You can use our [fury-kubernetes-keycloak](https://github.com/sighupio/fury-kubernetes-keycloak) module.

## Roadmap

The following is a wishlist of features that we would like to add to GPM (in no particular order):

- [x] List the constraints that are currently using a `ConstraintTemplate`
- [ ] Polished OIDC authentication
- [ ] LDAP authentication
- [x] Better syntax highlighting for the rego code snippets
- [x] Root-less docker image
- [x] Multi-cluster view
- [x] Rewrite backend in Golang (WIP)
- [ ] Minimal write capabilities?

Please, let us know if you are using GPM and what features would you like to have by creating an issue here on GitHub 💪🏻
