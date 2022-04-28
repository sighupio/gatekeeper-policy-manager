# Copyright (c) 2022 SIGHUP s.r.l All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

import os
from datetime import datetime
from functools import wraps
from io import BytesIO
from logging.config import dictConfig
from urllib.parse import urljoin

from flask import Flask, jsonify, render_template, request, send_file
from flask_pyoidc import OIDCAuthentication
from flask_pyoidc.provider_configuration import (
    ClientMetadata,
    ProviderConfiguration,
    ProviderMetadata,
)
from flask_cors import CORS
from kubernetes import client, config
from kubernetes.client.rest import ApiException
from kubernetes.config.config_exception import ConfigException
from urllib3.exceptions import MaxRetryError, NewConnectionError

# Set up logging
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
            "level": os.environ.get("GPM_LOG_LEVEL", "INFO"),
            "handlers": ["wsgi"],
        },
    }
)

app = Flask(__name__)

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
        "APP_ENV": os.environ.get("FLASK_ENV")
    }
)

if app.config.get("APP_ENV") == "development":
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


def dict_to_li(my_dict, html):
    """Recursive function to convert dict items into <li> html tags"""
    app.logger.debug(f"looping for {my_dict} and html is:\n{html}")
    if my_dict is None:
        return html
    for k, v in my_dict.items():
        app.logger.debug("Processing %s, %s" % (k, v))
        if isinstance(v, dict):
            html += "<li>%s:</li>\n%s" % (k, dict_to_ul(v))
        else:
            html += "<li>%s: %s</li>" % (k, v)
    html += "</ul>"
    return html


@app.template_filter("dict_to_ul")
def dict_to_ul(s):
    """
    Helper to convert recursively dict elements to an html unsorted list
    """
    app.logger.debug("Flattening %s" % s)
    result = '<ul style="padding-left:2em">'
    result = dict_to_li(s, result)
    app.logger.debug("Result of flattening: %s" % result)
    return str(result)


# We have to do this ugly thing in order to apply conditionally the login
# decorator only when it is enabled from the oonfig.
def login_required_conditional(f):
    @wraps(f)
    def decorated_function(*args, **kwargs):
        if app.config.get("AUTH_ENABLED") == "OIDC":
            return auth.oidc_auth("oidc")(f)(*args, **kwargs)
        return f(*args, **kwargs)

    return decorated_function


def get_api(context=None):
    """
    This function tries to detect if the app is running on a K8S cluster or locally
    and returns the corresponding API object to be used to query the API server.
    """
    if app.config.get("MODE") == "KUBECONFIG":
        return client.CustomObjectsApi(config.new_client_from_config(context=context))
    elif app.config.get("MODE") == "CLUSTER":
        return client.CustomObjectsApi()


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


@app.route("/")
@app.route("/<context>")
def index(context=None):
    """Welcome page view"""
    return render_template(
        "index.html",
        title="Welcome!",
        current_context=context,
        contexts=get_k8s_contexts(),
    )

@app.route("/api/v1/contexts/")
@login_required_conditional
def get_contexts():
    """
    Returns a list of the available Kubernetes contexts
    """
    return jsonify(get_k8s_contexts())


@app.route("/api/v1/constraints/")
@app.route("/api/v1/constraints/<context>/")
@login_required_conditional
def get_constraints(context=None):
    """Constraints view"""
    try:
        api = get_api(context)
        all_constraints = api.get_cluster_custom_object(
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
            return []
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
        # For some reason, the previous query returns a lot of objects that we
        # are not interested. We need to filter the ones that we do care about.
        constraints = []
        for c in all_constraints["resources"]:
            if c.get("categories"):
                if "constraint" in c.get("categories"):
                    c = api.get_cluster_custom_object(
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
                attachment_filename="constraints-report.html",
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
        constrainttemplates = api.get_cluster_custom_object(
            group="templates.gatekeeper.sh",
            version="v1beta1",
            plural="constrainttemplates",
            name="",
        )
        constraints_by_constrainttemplates = {}
        for ct in constrainttemplates.get("items"):
            constraints_by_constrainttemplates[
                ct["metadata"]["name"]
            ] = api.list_cluster_custom_object(
                group="constraints.gatekeeper.sh",
                version="v1beta1",
                plural=ct["metadata"]["name"],
            ).get(
                "items"
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
                "constraints_by_constrainttemplates":{},
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
@app.route("/api/v1/configs/<context>")
@login_required_conditional
def get_gatekeeperconfigs(context=None):
    """Gatekeeper Configs View"""
    try:
        api = get_api(context)
        configs = api.get_cluster_custom_object(
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
        return configs.get("items")


@app.route("/health")
def health():
    """Health check endpoint for probes"""
    return {"message": "OK"}


# Only set up this routes if authentication has been enabled
if app.config.get("AUTH_ENABLED") == "OIDC":

    @app.route("/logout")
    @auth.oidc_logout
    def logout():
        """End session locally"""
        return {}

    @auth.error_view
    def error(error=None, error_description=None):
        """View to handle OIDC errors and show them properly"""
        return {
            "error": "OIDC Error: " + error,
            "action": "Something is wrong with your OIDC session. Please try to logout and login again",
            "description": error_description,
        }, 401
