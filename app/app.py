# Copyright (c) 2022 SIGHUP s.r.l All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

import os
from datetime import datetime
from functools import wraps
from io import BytesIO
from logging import getLevelName, getLogger
from logging.config import dictConfig
from urllib.parse import urljoin

from flask import (
    Flask,
    jsonify,
    redirect,
    render_template,
    request,
    send_file,
    send_from_directory,
    session,
)
from flask_cors import CORS
from flask_pyoidc import OIDCAuthentication
from flask_pyoidc.provider_configuration import (
    ClientMetadata,
    ProviderConfiguration,
    ProviderMetadata,
)
from flask_pyoidc.user_session import UserSession
from kubernetes import client, config
from kubernetes.client.rest import ApiException
from kubernetes.config.config_exception import ConfigException
from urllib3.exceptions import MaxRetryError, NewConnectionError

app = Flask(__name__, static_folder="static-content", template_folder="templates")

# setup logging
if "gunicorn" in os.environ.get("SERVER_SOFTWARE", ""):
    # let's use gunicorn logger.
    gunicorn_logger = getLogger("gunicorn.error")
    # print(gunicorn_logger)
    app.logger.handlers = gunicorn_logger.handlers
    gunicorn_logger.setLevel(os.environ.get("GPM_LOG_LEVEL", "INFO"))
    app.logger.setLevel(gunicorn_logger.level)
    app.logger.info(
        f"gunicorn log level is set to: {getLevelName(gunicorn_logger.level)}"
    )
    app.logger.info(
        f"application log level is set to: {getLevelName(app.logger.level)}"
    )
else:
    # we're running through flask directly, use standard logger
    dictConfig(
        {
            "version": 1,
            "formatters": {
                "default": {"format": "[%(asctime)s] %(levelname)s: %(message)s"}
            },
            "handlers": {
                "wsgi": {
                    "class": "logging.StreamHandler",
                    "stream": "ext://flask.logging.wsgi_errors_stream",
                    "formatter": "default",
                }
            },
            "root": {
                "level": os.environ.get("GPM_LOG_LEVEL", "DEBUG"),
                "handlers": ["wsgi"],
            },
        }
    )

# Update app config with env vars
app.config.update(
    {
        "SERVER_NAME": os.environ.get("GPM_SERVER_NAME"),
        "SECRET_KEY": os.environ.get("GPM_SECRET_KEY", "g8k1p3rp0l1c7m4n4g3r"),
        "KUBERNETES": os.environ.get("KUBERNETES_SERVICE_HOST"),
        "PREFERRED_URL_SCHEME": os.environ.get("GPM_PREFERRED_URL_SCHEME", "http"),
        "AUTH_ENABLED": os.environ.get("GPM_AUTH_ENABLED"),
        "OIDC_REDIRECT_URI": urljoin(
            os.environ.get("GPM_OIDC_REDIRECT_DOMAIN", ""), "oidc-auth"
        ),
        "APP_ENV": os.environ.get("FLASK_ENV"),
    }
)

if app.config.get("APP_ENV") == "development":
    app.logger.info("running Flask in development mode")
    CORS(app)

if app.config.get("AUTH_ENABLED") == "OIDC":
    app.logger.info("AUTHENTICATION ENABLED WITH %s" % app.config.get("AUTH_ENABLED"))

    if not os.environ.get("GPM_OIDC_REDIRECT_DOMAIN"):
        app.logger.error(
            "Authentication is enabled with OIDC but GPM_OIDC_REDIRECT_DOMAIN environment variable has not been set."
        )

    provider_metadata = ProviderMetadata(
        issuer=os.environ.get("GPM_OIDC_ISSUER"),
        authorization_endpoint=os.environ.get("GPM_OIDC_AUTHORIZATION_ENDPOINT"),
        jwks_uri=os.environ.get("GPM_OIDC_JWKS_URI"),
        token_endpoint=os.environ.get("GPM_OIDC_TOKEN_ENDPOINT"),
        token_introspection_endpoint=os.environ.get("GPM_OIDC_INTROSPECTION_ENDPOINT"),
        userinfo_endpoint=os.environ.get("GPM_OIDC_USERINFO_ENDPOINT"),
        end_session_endpoint=os.environ.get("GPM_OIDC_END_SESSION_ENDPOINT"),
    )

    provider_config = ProviderConfiguration(
        issuer=os.environ.get("GPM_OIDC_ISSUER"),
        provider_metadata=provider_metadata,
        session_refresh_interval_seconds=10,
        client_metadata=ClientMetadata(
            client_id=os.environ.get("GPM_OIDC_CLIENT_ID"),
            client_secret=os.environ.get("GPM_OIDC_CLIENT_SECRET"),
        ),
    )

    auth = OIDCAuthentication({"oidc": provider_config}, app)
else:
    app.logger.info("RUNNING WITH AUTHENTICATION DISABLED")

# This snippet tries to detect if the app is running on a K8S cluster or locally
try:
    app.logger.info(
        f"Attempting init with KUBECONFIG from path '{config.kube_config.KUBE_CONFIG_DEFAULT_LOCATION}'"
    )
    config.load_kube_config()
    app.logger.info(
        f"KUBECONFIG '{config.kube_config.KUBE_CONFIG_DEFAULT_LOCATION}' successfuly loaded."
    )
    app.config["MODE"] = "KUBECONFIG"
except config.ConfigException as e:
    if app.config.get("KUBERNETES"):
        app.logger.debug(f"KUBECONFIG loading failed. Got error: {e}")
        app.logger.info(
            "KUBECONFIG loading failed but KUBERNETES_SERVICE_HOST environment variable found, "
            "assuming to be running inside a Kubernetes cluster"
        )
        config.load_incluster_config()
        app.logger.info("In cluster configuration loaded successfully.")
        app.config["MODE"] = "CLUSTER"
    else:
        app.logger.error(
            "CRITICAL - environment variable KUBERNETES_SERVICE_HOST was not found and loading KUBECONFIG from"
            f" '{config.kube_config.KUBE_CONFIG_DEFAULT_LOCATION}' failed with error: {e}"
        )
        exit(1)


# We have to do this ugly thing in order to apply conditionally the login
# decorator only when it is enabled from the config.
def login_required_conditional(f):
    @wraps(f)
    def decorated_function(*args, **kwargs):
        if app.config.get("AUTH_ENABLED") == "OIDC":
            path = kwargs.get("path")
            if path is not None and (
              path.startswith("static/")
              or path in ["logout", "favicon", "manifests.json"]
            ):
                return f(*args, **kwargs)
            return auth.oidc_auth("oidc")(f)(*args, **kwargs)
        return f(*args, **kwargs)

    return decorated_function


def get_api(context=None):
    """
    This function tries to detect if the app is running on a K8S cluster or locally
    and returns the corresponding API object to be used to query the API server.
    """
    if app.config.get("MODE") == "KUBECONFIG":
        app.logger.debug("entering KUBECONFIG MODE and getting API objects")
        return {
            "cm": client.CustomObjectsApi(
                config.new_client_from_config(context=context)
            ),
            "apis": client.ApisApi(config.new_client_from_config(context=context)),
        }
    elif app.config.get("MODE") == "CLUSTER":
        app.logger.debug("entering CLUSTER MODE and getting API objects")
        return {
            "cm": client.CustomObjectsApi(),
            "apis": client.ApisApi(),
        }


def get_k8s_contexts():
    """
    Helper function to return a list of the available Kubernetes contexts
    when using a kubeconfig file.
    When running in a cluster returns None.
    """
    if app.config.get("MODE") == "KUBECONFIG":
        contexts = config.list_kube_config_contexts()
    else:
        contexts = None
    return contexts


@app.route("/api/v1/contexts/")
def get_contexts():
    """
    Returns a list of the available Kubernetes contexts
    """
    return jsonify(get_k8s_contexts())


@app.route("/api/v1/auth/")
def get_auth():
    """
    Returns if the authentication is active or not
    """
    return jsonify({"auth_enabled": app.config.get("AUTH_ENABLED") == "OIDC"})


@app.route("/api/v1/constraints/")
@app.route("/api/v1/constraints/<context>/")
@login_required_conditional
def get_constraints(context=None):
    """Constraints view"""
    try:
        api = get_api(context)
        all_constraints = api["cm"].get_cluster_custom_object(
            group="constraints.gatekeeper.sh",
            version="v1beta1",
            plural="",
            name="",
        )
    except NewConnectionError as e:
        return {
            "error": "Could not connect to Kubernetes Cluster",
            "action": "Is the current Kubeconfig context valid?",
            "description": str(e),
        }, 500
    except MaxRetryError as e:
        return {
            "error": "Could not connect to Kubernetes Cluster",
            "action": "Is the current Kubeconfig context valid?",
            "description": str(e),
        }, 500
    except ApiException as e:
        if e.status == 404:
            return jsonify([])
        else:
            return {
                "error": "We had a problem while asking the API for Gatekeeper Constraint objects",
                "action": "Is Gatekeeper deployed in the cluster?",
                "description": str(e),
            }, 500
    except ConfigException as e:
        return {
            "error": "Can't connect to cluster due to an invalid kubeconfig file",
            "action": "Please verify your kubeconfig file and location",
            "description": str(e),
        }, 500
    else:
        # For some reason, the previous query returns a lot of objects that we
        # are not interested. We need to filter the ones that we do care about.
        constraints = []
        for c in all_constraints["resources"]:
            if c.get("categories"):
                if "constraint" in c.get("categories"):
                    c = api["cm"].get_cluster_custom_object(
                        group="constraints.gatekeeper.sh",
                        version="v1beta1",
                        plural=c["name"],
                        name="",
                    )
                    for i in c["items"]:
                        constraints.append(i)
        constraints = sorted(
            constraints,
            key=lambda x: x.get("status").get("totalViolations") or -1
            if x.get("status")
            else -1,
            reverse=True,
        )
        if request.args.get("report"):
            buffer = BytesIO()
            report_file = render_template(
                "constraints-report.html",
                constraints=constraints,
                title="Constraints",
                hide_sidebar=len(constraints) == 0,
                current_context=context,
                contexts=get_k8s_contexts(),
                timestamp=datetime.now().strftime("%a, %x %X"),
            )
            buffer.write(report_file.encode("utf-8"))
            buffer.seek(0)
            return send_file(
                buffer,
                as_attachment=True,
                download_name="constraints-report.html",
                mimetype="text/html",
            )
        else:
            return jsonify(constraints)


@app.route("/api/v1/constrainttemplates/")
@app.route("/api/v1/constrainttemplates/<context>/")
@login_required_conditional
def get_constrainttemplates(context=None):
    """Constraint Templates View"""
    try:
        api = get_api(context)
        currentversion = "v1beta1"
        for a in api["apis"].get_api_versions().groups:
            if a.name == "templates.gatekeeper.sh":
                currentversion = a.preferred_version.version
                break

        constrainttemplates = (
            api["cm"]
            .get_cluster_custom_object(
                group="templates.gatekeeper.sh",
                version=currentversion,
                plural="constrainttemplates",
                name="",
            )
            .get("items")
        )
        constraints_by_constrainttemplates = {}
        for ct in constrainttemplates:
            constraints_by_constrainttemplates[ct["metadata"]["name"]] = (
                api["cm"]
                .list_cluster_custom_object(
                    group="constraints.gatekeeper.sh",
                    version="v1beta1",
                    plural=ct["metadata"]["name"],
                )
                .get("items")
            )
    except NewConnectionError as e:
        return {
            "error": "Could not connect to Kubernetes Cluster",
            "action": "Is the current Kubeconfig context valid?",
            "description": str(e),
        }, 500
    except MaxRetryError as e:
        return {
            "error": "Could not connect to Kubernetes Cluster",
            "action": "Is the current Kubeconfig context valid?",
            "description": str(e),
        }, 500
    except ApiException as e:
        if e.status == 404:
            return {
                "constrainttemplates": [],
                "constraints_by_constrainttemplates": {},
            }
        else:
            return {
                "error": "We had a problem while asking the API for Gatekeeper Constraint Templates objects",
                "action": "Is Gatekeeper deployed in the cluster?",
                "description": str(e),
            }, 500
    except ConfigException as e:
        return {
            "error": "Can't connect to cluster due to an invalid kubeconfig file",
            "action": "Please verify your kubeconfig file and location",
            "description": str(e),
        }, 500
    else:
        return {
            "constrainttemplates": constrainttemplates,
            "constraints_by_constrainttemplates": constraints_by_constrainttemplates,
        }


@app.route("/api/v1/configs/")
@app.route("/api/v1/configs/<context>/")
@login_required_conditional
def get_gatekeeperconfigs(context=None):
    """Gatekeeper Configs View"""
    try:
        api = get_api(context)
        configs = api["cm"].get_cluster_custom_object(
            group="config.gatekeeper.sh",
            version="v1alpha1",
            plural="configs",
            name="",
        )
    except NewConnectionError as e:
        return {
            "error": "Could not connect to Kubernetes Cluster",
            "action": "Is the current Kubeconfig context valid?",
            "description": str(e),
        }, 500
    except MaxRetryError as e:
        return {
            "error": "Could not connect to Kubernetes Cluster",
            "action": "Is the current Kubeconfig context valid?",
            "description": str(e),
        }, 500
    except ApiException as e:
        if e.status == 404:
            return jsonify([])
        else:
            return {
                "error": "We had a problem while asking the API for Gatekeeper Configuration objects",
                "action": "Is Gatekeeper deployed in the cluster?",
                "description": str(e),
            }, 500
    except ConfigException as e:
        return {
            "error": "Can't connect to cluster due to an invalid kubeconfig file",
            "action": "Please verify your kubeconfig file and location",
            "description": str(e),
        }, 500
    else:
        return jsonify(configs.get("items"))


@app.route("/health")
def health():
    """Health check endpoint for probes"""
    return {"message": "OK"}


# Only set up this routes if authentication has been enabled
if app.config.get("AUTH_ENABLED") == "OIDC":

    @app.route("/logout", methods=["POST"])
    @auth.oidc_logout
    def logout():
        """End session locally"""
        return {}

    @app.route("/logout", methods=["GET"])
    def logout_view():
        """End session locally"""
        return send_from_directory(app.static_folder, "index.html")

    @auth.error_view
    def error(error=None, error_description=None):
        """View to handle OIDC errors and show them properly"""
        if error == "login_required":
            user_session = UserSession(session)
            app.logger.debug(
                f"session has expired for user {user_session.userinfo}. Cleaning session locally."
            )
            user_session.clear()
            # we should probably redirect to a "You've been logged out, please log in again" page instead
            app.logger.debug(
                "redirecting to previous destination that will request login"
            )
            return redirect(user_session._session_storage.get("destination", "/"))
        return {
            "error": f"OIDC Error: {error}",
            "action": "Something is wrong with your OIDC session. Please try to logout and login again",
            "description": error_description,
        }, 401


@app.route("/", defaults={"path": ""})
@app.route("/<path:path>")
@login_required_conditional
def index(path):
    if path != "" and os.path.exists(os.path.join(app.static_folder, path)):
        return send_from_directory(app.static_folder, path)
    else:
        return send_from_directory(app.static_folder, "index.html")
