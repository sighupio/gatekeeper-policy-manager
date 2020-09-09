import os
from logging.config import dictConfig
from kubernetes import client, config
from kubernetes.client.rest import ApiException
from flask import Flask, render_template, session, jsonify
from urllib3.exceptions import NewConnectionError, MaxRetryError
from flask_pyoidc import OIDCAuthentication
from flask_pyoidc.provider_configuration import ProviderConfiguration, ProviderMetadata, ClientMetadata
from flask_pyoidc.user_session import UserSession
from functools import wraps

# Set up logging
dictConfig({
    'version': 1,
    'formatters': {'default': {
        'format': '[%(asctime)s] %(levelname)s in %(module)s: %(message)s',
    }},
    'handlers': {'wsgi': {
        'class': 'logging.StreamHandler',
        'stream': 'ext://flask.logging.wsgi_errors_stream',
        'formatter': 'default'
    }},
    'root': {
        'level': os.environ.get('GPM_LOG_LEVEL', 'INFO'),
        'handlers': ['wsgi']
    }
})


# We have to do this ugly thing in order to apply conditionally the login
# decorator only when it is enabled from the oonfig.
def login_required_conditional(f):
    @wraps(f)
    def decorated_function(*args, **kwargs):
        if app.config.get('AUTH_ENABLED') == 'OIDC':
            return auth.oidc_auth('oidc')(f)(*args, **kwargs)
        return f(*args, **kwargs)
    return decorated_function


app = Flask(__name__)

# Update app config with env vars
app.config.update({
                   'SERVER_NAME': os.environ.get("GPM_SERVER_NAME"),
                   'SECRET_KEY': os.environ.get("GPM_SECRET_KEY", 'g8k1p3rp0l1c7m4n4g3r'),
                   'KUBERNETES': os.environ.get("KUBERNETES_SERVICE_HOST"),
                   'PREFERRED_URL_SCHEME': os.environ.get('GPM_PREFERRED_URL_SCHEME', 'http'),
                   'AUTH_ENABLED': os.environ.get('GPM_AUTH_ENABLED'),
                   'OIDC_REDIRECT_DOMAIN': os.environ.get('GPM_OIDC_REDIRECT_DOMAIN')
                   })

if app.config.get('AUTH_ENABLED') == 'OIDC':
    app.logger.info('AUTHENTICATION ENABLED WITH %s' % app.config.get('AUTH_ENABLED'))
    provider_metadata = ProviderMetadata(issuer=os.environ.get('GPM_OIDC_ISSUER'),
                                         authorization_endpoint=os.environ.get('GPM_OIDC_AUTHORIZATION_ENDPOINT'),
                                         jwks_uri=os.environ.get('GPM_OIDC_JWKS_URI'),
                                         token_endpoint=os.environ.get('GPM_OIDC_TOKEN_ENDPOINT'),
                                         token_introspection_endpoint=os.environ.get('GPM_OIDC_INTROSPECTION_ENDPOINT'),
                                         userinfo_endpoint=os.environ.get('GPM_OIDC_USERINFO_ENDPOINT'),
                                         end_session_endpoint=os.environ.get('GPM_OIDC_END_SESSION_ENDPOINT'),
                                         )

    provider_config = ProviderConfiguration(issuer=os.environ.get('GPM_OIDC_ISSUER'),
                                            provider_metadata=provider_metadata,
                                            session_refresh_interval_seconds=10,
                                            client_metadata=ClientMetadata(client_id=os.environ.get('GPM_OIDC_CLIENT_ID'),
                                                                           client_secret=os.environ.get('GPM_OIDC_CLIENT_SECRET')
                                                                           )
                                            )

    auth = OIDCAuthentication({'oidc': provider_config}, app)
else:
    app.logger.info('RUNNING WITH AUTHENTICATION DISABLED')


def dict_to_li(my_dict, html):
    """Recursive function to convert dict items into <li> html tags"""
    if my_dict is None:
        return html
    for k, v in my_dict.items():
        app.logger.debug("Processing %s, %s" % (k, v))
        if not isinstance(v, dict):
            html += '<li>%s: %s</li>' % (k, v)
        else:
            html += '<li>%s: %s</li>' % (k, dict_to_li(v, html))
    html += "</ul>"
    return html


@app.template_filter('dict_to_ul')
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
        app.logger.info("KUBERNETES_SERVICE_HOST found, assuming to be running inside Kubernetes cluster")
        config.load_incluster_config()
    else:
        app.logger.info("KUBERNETES_SERVICE_HOST environment variable not found. Falling back to kubeconfig")
        config.load_kube_config()
    return client.CustomObjectsApi()


@app.route('/')
def index():
    """Welcome page view"""
    return render_template("index.html",
                           title="Welcome!"
                           )


@app.route('/constraints/')
@login_required_conditional
def get_constraints():
    """Contraints view"""
    api = get_api()
    try:
        all_constraints = api.get_cluster_custom_object(group="constraints.gatekeeper.sh",
                                                        version="v1beta1",
                                                        plural="",
                                                        name="",
                                                        )
    except NewConnectionError as e:
        return render_template("message.html",
                               type="error",
                               title="Error",
                               message="Could not connect to Kubernetes Cluster",
                               action="Is the current Kubeconfig context valid?",
                               description=e
                               )
    except MaxRetryError as e:
        return render_template("message.html",
                               type="error",
                               title="Error",
                               message="Could not connect to Kubernetes Cluster",
                               action="Is the current Kubeconfig context valid?",
                               description=e
                               )
    except ApiException as e:
        return render_template("message.html",
                               type="error",
                               title="Error",
                               message="We had a problem while asking the API for Gatekeeper objects",
                               action="Is Gatekeeper deployed in the cluster?",
                               description=e
                               )
    else:
        # For some reasing, the previous query returns a lot of objects that we
        # are not interested. We need to filter the ones that we do care about.
        constraints = []
        for c in all_constraints["resources"]:
            if c.get("categories"):
                if "constraint" in c.get("categories"):
                    c = api.get_cluster_custom_object(group="constraints.gatekeeper.sh",
                                                      version="v1beta1",
                                                      plural=c["name"],
                                                      name="",
                                                      )
                    for i in c["items"]:
                        constraints.append(i)
        if len(constraints) > 0:
            # We pass to the template all the constratins sorted by ammount of violations
            return render_template("constraints.html",
                                   constraints=sorted(constraints,
                                                      reverse=True,
                                                      key=lambda x: x.get("status").get("totalViolations") or -1
                                                      ),
                                   title="Constraints")
        else:
            return render_template("message.html",
                                   type="error",
                                   title="Error",
                                   message="We had a problem while asking the API for Gatekeeper Constraints objects",
                                   action="There are no Constraints found",
                                   )


@app.route('/constrainttemplates/')
@login_required_conditional
def get_constrainttemplates():
    """Constraint Templates View"""
    api = get_api()

    try:
        constrainttemplates = api.get_cluster_custom_object(group="templates.gatekeeper.sh",
                                                            version="v1beta1",
                                                            plural="constrainttemplates",
                                                            name="",
                                                            )
    except NewConnectionError as e:
        return render_template("error.html",
                               title="Error",
                               error="Could not connect to Kubernetes Cluster",
                               action="Is the current Kubeconfig context valid?",
                               description=e
                               )
    except MaxRetryError as e:
        return render_template("error.html",
                               title="Error",
                               error="Could not connect to Kubernetes Cluster",
                               action="Is the current Kubeconfig context valid?",
                               description=e
                               )
    except ApiException as e:
        return render_template("message.html",
                               type="error",
                               title="Error",
                               message="We had a problem while asking the API for Gatekeeper objects",
                               action="Is Gatekeeper deployed in the cluster?",
                               description=e
                               )
    else:
        return render_template("constrainttemplates.html",
                               constrainttemplates=constrainttemplates,
                               title="Constraint Templates"
                               )


@app.route('/health')
def health():
    """Health check endpoint for probes"""
    return {'message': 'OK'}


# Only set up this routes if authentication has been enabled
if app.config.get('AUTH_ENABLED') == 'OIDC':
    @app.route('/logout')
    @auth.oidc_logout
    def logout():
        """End session locally"""
        return render_template("message.html",
                               type="success",
                               message="Logout successful",
                               action='<a href="/" class="ui huge primary button">Go back to the home <i class="right arrow icon"></i></a>',
                               )

    @auth.error_view
    def error(error=None, error_description=None):
        """View to handle OIDC errors and show them properly"""
        return render_template("message.html",
                               type="error",
                               message="OIDC Error: " + error,
                               action='Something is wrong with your OIDC session. Please try to <a href="/logout">logout</a> and login again',
                               description=error_description
                               )

    # This can be useful to debug OIDC login
    # @app.route('/login')
    # @auth.oidc_auth('oidc')
    # def login():
    #     user_session = UserSession(session)
    #     return jsonify(access_token=user_session.access_token,
    #                    id_token=user_session.id_token,
    #                    userinfo=user_session.userinfo)
