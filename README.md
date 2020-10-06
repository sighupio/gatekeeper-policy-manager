# Gatekeeper Policy Manager (GPM)

Gatekeeper Policy Manager is a simple **read-only** web UI for viewing OPA Gatekeeper policies' status in a Kubernetes Cluster.

It can display all the defined **Constraint Templates** with their rego code, and all the **Contraints** with its current status, violations, enforcement action, matched definitions, etc.

## Requirements

You'll need OPA Gatekeeper running in your cluster and at least some constraint templates and constraints defined to take advantage of this tool.

ℹ You can easily deploy Gatekeeper to your cluster using the (also open source) [Fury Kubernetes OPA](https://github.com/sighupio/fury-kubernetes-opa) module.

## Deploying GPM

To deploy Gatekeeper Policy Manager to your cluster, apply the provided `kustomization` file running the following command:

```shell
kubectl apply -k .
```

By default, this will create a deployment and a service both with the name `gatekeper-policy-manager` in the `gatekeeper-system` namespace. We invite you to take a look into the `kustomization.yaml` file to do further configuration.

> The app can be run as a POD in a Kubernetes cluster or locally with a `kubeconfig` file. It will try it best to autodetect the correct configuration.

Once you've deployed the application, if you haven't set up an ingress, you can access the web-UI using port-forward:

```bash
kubectl -n gatekeeper-system port-forward  svc/gatekeeper-policy-manager 8080:80
````

Then access it with your browser on: [http://127.0.0.1:8080](http://127.0.0.1:8080)

## Running locally

GPM can also be run locally using docker and a `kubeconfig`, assuming that the `kubeconfig` file you want to use is located at `~/.kube/config` the command to run GPM locally would be:

```bash
docker run -v ~/.kube/config:/root/.kube/config -p 8080:8080 quay.io/sighup/gatekeeper-policy-manager:0.3
```

Then access it with your browser on: [http://127.0.0.1:8080](http://127.0.0.1:8080)

> You can also run the flask app directly, see the development section for further information.

## Configuration

GPM is a stateless application, but it can be configured using environment variables. The possible configurations are:

| Env Var Name                      | Description                                                                                                       | Default                |
| --------------------------------- | ----------------------------------------------------------------------------------------------------------------- | ---------------------- |
| `GPM_AUTH_ENABLED`                | Enable Authentication current options: "Anonymous", "OIDC"                                                        | Anonymous              |
| `GPM_SECRET_KEY`                  | The secret key used to generate tokens. **Change this value in production**.                                      | `g8k1p3rp0l1c7m4n4g3r` |
| `GPM_PREFERRED_URL_SCHEME`        | URL scheme to be used while generating links.                                                                     | `http`                 |
| `GPM_OIDC_REDIRECT_DOMAIN`        | The server name under the app is being exposed. This is where the client will be redirected after authenticating  |
| `GPM_OIDC_ISSUER`                 | OIDC Issuer hostname                                                                                              |
| `GPM_OIDC_AUTHORIZATION_ENDPOINT` | OIDC Authorizatoin Endpoint                                                                                       |
| `GPM_OIDC_JWKS_URI`               | OIDC JWKS URI                                                                                                     |
| `GPM_OIDC_TOKEN_ENDPOINT`         | OIDC TOKEN Endpoint                                                                                               |
| `GPM_OIDC_INTROSPECTION_ENDPOINT` | OIDC Introspection Enpoint                                                                                        |
| `GPM_OIDC_USERINFO_ENDPOINT`      | OIDC Userinfo Endpoint                                                                                            |
| `GPM_OIDC_END_SESSION_ENDPOINT`   | OIDC End Session Endpoint                                                                                         |
| `GPM_OIDC_CLIENT_ID`              | The Client ID used to authenticate against the OIDC Provider                                                      |
| `GPM_OIDC_CLIENT_SECRET`          | The Client Secret used to authenticate against the OIDC Provider                                                  |
| `GPM_LOG_LEVEL`                   | Log level (see [python logging docs](https://docs.python.org/2/library/logging.html#levels) for available levels) | `INFO`                 |

> ⚠️ Please notice that OIDC Authentication is in beta state. It has been tested to work with Keycloak as a provider.
>
> These environment variables are already provided and ready to be set in the [`manifests/enable-oidc.yaml`](manifests/enable-oidc.yaml) file.

## Screenshots

![welcome](screenshots/01-home.png)

![Constraint Templates view](screenshots/02-constrainttemplates.png)

![Constraint Templates view rego code](screenshots/03-constrainttemplates.png)

![Constraint view](screenshots/04-constraints.png)

![Constraint view 2](screenshots/05-constraints.png)

![Constraint view 3](screenshots/06-constraints.png)

![Configurations view 2](screenshots/07-configs.png)

## Development

GPM is written in Python using the Flask framework for the backend and Fromantic-UI for the frontend. To develop GPM, you'll create a Python 3 virtual environment, install all the dependencies specified in the provided `requirements.txt` and you are good to go.

The following commands should get you up and running:

```bash
# Create a virtualenv
$ python3 -m venv env
# Activate it
$ source ./env/bin/activate
# Install all the dependencies
$ pip install -r app/requirements.txt
# Run the development server
$ FLASK_APP=app/app.py flask run
```

> Access to a Kubernetes cluster with Gatekeeper deployed is recommended to debug the application.
>
> You'll need an OIDC provider to test the OIDC authentication. You can use our [fury-kubernetes-keycloak](https://github.com/sighupio/fury-kubernetes-keycloak) module.

## Roadmap

The following is a wishlist of features that we would like to add to GPM (in no particular order):

- List the constraints that are currently using a `ConstraintTemplate`
- Polished OIDC authentication
- LDAP authentication
- Better syntax highlighting for the rego code snippets
- Root-less docker image
- Multi-cluster view
- Minimal write capabilities?
- Refactor app in Golang?

Please, let us know if you are using GPM and what features would you like to have by creating an issue in GitHub
