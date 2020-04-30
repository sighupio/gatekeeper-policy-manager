# Gatekeper Policy Manager (GPM)

Gatekeper Policy Manager is a simple **read only** web UI for viewing OPA Gatekeeper policies' status in a Kubernetes Cluster.

It can display all the defined **Constraint Templates** with their rego code, and all the **Contraints** with it's current status, violations, enforcement action, matches definition, etc.

## Deploying GPM

In order to deploy Gatekeper Policy Manager to your cluster, apply the provided kustomization file running the following command:

```shell
$ kubectl apply -k .
```

By default, this will create a deployment and a service both with the name `gatekeper-policy-manager` in the `gatekeeper-system` namespace. We invite you to take a look into the `kustomization.yaml` file to do further configuration.

> The app can be run as a POD in a Kubernetes cluster or locally with a kubeconfig file. It will try it best to autodetect the correct configuration.

## Configuration

GPM is an stateless application, but it can be configured using environment variables. The possible configurations are:

Env Var Name | Description | Default
-------------|-------------|--------
`GPM_AUTH_ENABLED` | Enable Authentication current options: "Anonymous", "OIDC" | Anonymous
`GPM_SECRET_KEY` | The secret key used to generate tokens. **Change this value in production**. | `g8k1p3rp0l1c7m4n4g3r`
`GPM_PREFERRED_URL_SCHEME` | URL scheme to be used while generating links. | `http`
`GPM_OIDC_REDIRECT_DOMAIN` | The server name under the app is being exposed. This is where the client will be redirected after authenticating |
`GPM_OIDC_ISSUER` | OIDC Issuer hostname |
`GPM_OIDC_AUTHORIZATION_ENDPOINT` | OIDC Authorizatoin Endpoint |
`GPM_OIDC_JWKS_URI` | OIDC JWKS URI |
`GPM_OIDC_TOKEN_ENDPOINT` | OIDC TOKEN Endpoint |
`GPM_OIDC_INTROSPECTION_ENDPOINT` | OIDC Introspection Enpoint |
`GPM_OIDC_USERINFO_ENDPOINT` | OIDC Userinfo Endpoint |
`GPM_OIDC_END_SESSION_ENDPOINT` | OIDC End Session Endpoint |
`GPM_OIDC_CLIENT_ID` | The Client ID used to authenticate against the OIDC Provider |
`GPM_OIDC_CLIENT_SECRET` | The Client Secret used to authenticate against the OIDC Provider |
`GPM_LOG_LEVEL` | Log level (see [python logging docs](https://docs.python.org/2/library/logging.html#levels) for available levels) | `INFO`

> ⚠️ Please notice that OIDC Authentication is in beta state. It has been tested to work wit Keycloak as a provider.

> These environment variables are already provided and ready to be set in the [`manifests/enable-oidc.yaml`](manifests/enable-oidc.yaml) file.

## Screenshots

![welcome](screenshots/01-home.png)

![Constraint Templates view](screenshots/02-constrainttemplates.png)

![Constraint Templates view rego code](screenshots/03-constrainttemplates.png)

![Constraint view](screenshots/04-constraints.png)

![Constraint view 2](screenshots/05-constraints.png)

## Development

GPM is written in Python using the Flask framework for the backend and Fromantic-UI for the frontend. In order to develop GPM, you'll must create a Python 3 virtual environment, install all the dependencies specified in the provided `requirements.txt` and you are good to go.

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

> Access to a kubernetes cluster with Gatekeeper deployed is recomended to debug the application.

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
