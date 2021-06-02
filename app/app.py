# Copyright (c) 2020 SIGHUP s.r.l All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

import os
from functools import wraps
from logging.config import dictConfig

from flask import Flask, render_template
from flask_pyoidc import OIDCAuthentication
from flask_pyoidc.provider_configuration import (
    ClientMetadata,
    ProviderConfiguration,
    ProviderMetadata,
)
from kubernetes import client, config
from kubernetes.client.rest import ApiException
from kubernetes.config.config_exception import ConfigException
from urllib3.exceptions import MaxRetryError, NewConnectionError

from urllib.parse import urljoin

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
            os.environ.get("GPM_OIDC_REDIRECT_DOMAIN"), "oidc-auth"
        ),
    }
)

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


def dict_to_li(my_dict, html):
    """Recursive function to convert dict items into <li> html tags"""
    if my_dict is None:
        return html
    for k, v in my_dict.items():
        app.logger.debug(
            "Processing %s, %s of type %s and length %s" % (k, v, type(v), len(v))
        )
        if isinstance(v, dict) and len(v) > 1:
            html += "<li>%s:</li> %s" % (k, dict_to_li(v, html))
        elif isinstance(v, dict) and len(v) == 1:
            for x, y in v.items():
                html += (
                    '<li>%s:</li><ul style="padding-left:2em"><li>%s: %s</li></ul>'
                    % (k, x, y)
                )
        else:
            html += "<li>%s: %s</li>" % (k, v)
    html += "</ul>"
    return html


# We have to do this ugly thing in order to apply conditionally the login
# decorator only when it is enabled from the oonfig.
def login_required_conditional(f):
    @wraps(f)
    def decorated_function(*args, **kwargs):
        if app.config.get("AUTH_ENABLED") == "OIDC":
            return auth.oidc_auth("oidc")(f)(*args, **kwargs)
        return f(*args, **kwargs)

    return decorated_function


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


def get_api():
    """
    This function tries to detect if the app is running on a K8S cluster or locally
    and returns the corresponding API object to be used to query the API server.
    """
    if app.config.get("KUBERNETES"):
        app.logger.info(
            "KUBERNETES_SERVICE_HOST found, assuming to be running inside Kubernetes cluster"
        )
        config.load_incluster_config()
    else:
        app.logger.info(
            "KUBERNETES_SERVICE_HOST environment variable not found. Falling back to kubeconfig"
        )
        config.load_kube_config()
    return client.CustomObjectsApi()


@app.route("/")
def index():
    """Welcome page view"""
    return render_template("index.html", title="Welcome!")


@app.route("/constraints/")
@login_required_conditional
def get_constraints():
    """Constraints view"""
    try:
        api = get_api()
        all_constraints = api.get_cluster_custom_object(
            group="constraints.gatekeeper.sh", version="v1beta1", plural="", name="",
        )
    except NewConnectionError as e:
        return render_template(
            "message.html",
            type="error",
            title="Error",
            message="Could not connect to Kubernetes Cluster",
            action="Is the current Kubeconfig context valid?",
            description=e,
        )
    except MaxRetryError as e:
        return render_template(
            "message.html",
            type="error",
            title="Error",
            message="Could not connect to Kubernetes Cluster",
            action="Is the current Kubeconfig context valid?",
            description=e,
        )
    except ApiException as e:
        if e.status == 404:
            return render_template(
                "constraints.html",
                constraints=[],
                title="Constraints",
                hide_sidebar=True,
            )
        else:
            return render_template(
                "message.html",
                type="error",
                title="Error",
                message="We had a problem while asking the API for Gatekeeper objects",
                action="Is Gatekeeper deployed in the cluster?",
                description=e,
            )
    except ConfigException as e:
        return render_template(
            "message.html",
            type="error",
            title="Error",
            message="Can't connect to cluster due to an invalid kubeconfig file",
            action="Please verify your kubeconfig file and location",
            description=e,
        )
    else:
        # For some reasing, the previous query returns a lot of objects that we
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
        # We pass to the template all the constratins sorted by ammount of violations
        return render_template(
            "constraints.html",
            constraints=sorted(
                constraints,
                reverse=True,
                key=lambda x: x.get("status").get("totalViolations") or -1
                if x.get("status")
                else -1,
            ),
            title="Constraints",
            hide_sidebar=len(constraints) == 0,
        )


@app.route("/constrainttemplates/")
@login_required_conditional
def get_constrainttemplates():
    """Constraint Templates View"""
    try:
        api = get_api()
        constrainttemplates = api.get_cluster_custom_object(
            group="templates.gatekeeper.sh",
            version="v1beta1",
            plural="constrainttemplates",
            name="",
        )
    except NewConnectionError as e:
        return render_template(
            "message.html",
            type="error",
            title="Error",
            error="Could not connect to Kubernetes Cluster",
            action="Is the current Kubeconfig context valid?",
            description=e,
        )
    except MaxRetryError as e:
        return render_template(
            "message.html",
            type="error",
            title="Error",
            error="Could not connect to Kubernetes Cluster",
            action="Is the current Kubeconfig context valid?",
            description=e,
        )
    except ApiException as e:
        if e.status == 404:
            return render_template(
                "constrainttemplates.html",
                constrainttemplates=[],
                title="Constraints",
                hide_sidebar=True,
            )
        else:
            return render_template(
                "message.html",
                type="error",
                title="Error",
                message="We had a problem while asking the API for Gatekeeper Constraint Templates objects",
                action="Is Gatekeeper deployed in the cluster?",
                description=e,
            )
    except ConfigException as e:
        return render_template(
            "message.html",
            type="error",
            title="Error",
            message="Can't connect to cluster due to an invalid kubeconfig file",
            action="Please verify your kubeconfig file and location",
            description=e,
        )
    else:
        return render_template(
            "constrainttemplates.html",
            constrainttemplates=constrainttemplates,
            title="Constraint Templates",
            hide_sidebar=len(constrainttemplates["items"]) == 0,
        )


@app.route("/configs/")
@login_required_conditional
def get_gatekeeperconfigs():
    """Gatekeeper Configs View"""
    try:
        api = get_api()
        configs = api.get_cluster_custom_object(
            group="config.gatekeeper.sh", version="v1alpha1", plural="configs", name="",
        )
    except NewConnectionError as e:
        return render_template(
            "message.html",
            type="error",
            title="Error",
            error="Could not connect to Kubernetes Cluster",
            action="Is the current Kubeconfig context valid?",
            description=e,
        )
    except MaxRetryError as e:
        return render_template(
            "message.html",
            type="error",
            title="Error",
            error="Could not connect to Kubernetes Cluster",
            action="Is the current Kubeconfig context valid?",
            description=e,
        )
    except ApiException as e:
        if e.status == 404:
            return render_template(
                "configs.html",
                gatekeeper_configs=[],
                title="Gatekeeper Configurations",
                hide_sidebar=True,
            )
        else:
            return render_template(
                "message.html",
                type="error",
                title="Error",
                message="We had a problem while asking the API for Gatekeeper Config objects",
                action="Is Gatekeeper deployed in the cluster?",
                description=e,
            )
    except ConfigException as e:
        return render_template(
            "message.html",
            type="error",
            title="Error",
            message="Can't connect to cluster due to an invalid kubeconfig file",
            action="Please verify your kubeconfig file and location",
            description=e,
        )
    else:
        return render_template(
            "configs.html",
            gatekeeper_configs=configs.get("items"),
            title="Gatekeeper Configurations",
            hide_sidebar=len(configs["items"]) == 0,
        )


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
        return render_template(
            "message.html",
            type="success",
            message="Logout successful",
            action="",  # noqa: E501
        )

    @auth.error_view
    def error(error=None, error_description=None):
        """View to handle OIDC errors and show them properly"""
        return render_template(
            "message.html",
            type="error",
            message="OIDC Error: " + error,
            action='Something is wrong with your OIDC session. Please try to <a href="/logout">logout</a> and login again',  # noqa: E501
            description=error_description,
        )
