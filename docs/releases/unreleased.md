# v0.5.1

Gatekeeper Policy Manager version 0.5.1

## Changes from v0.5.0

- Increased default memory limits in Kubernetes manifests and the Helm chart due to reports of missbehaviour with the previous values.
- Update to Python 3.10
- Set explicitly the replica count for Kubernetes deployment
- Fix error on Constraint Templates view when the Template doesn't have a `Properties` key under the OpenAPIv3 Schema.
- Switched `master` to `main`

### Updated dependencies

Backend:

- Python 3.9 -> 3.10
- cachetools 4.2.2 -> 4.2.4
- certifi 2021.5.30 -> 2021.10.8
- cffi 1.14.6 -> 1.15.0
- charset-normalizer 2.0.4 -> 2.0.7
- click 8.0.1 -> 8.0.3
- cryptography 3.4.7 -> 36.0.1
- Flask-pyoidc 3.7.0 -> 3.8.0
- google-auth 2.0.0 -> 2.3.3
- idna 3.2 -> 3.3
- importlib-resources 5.2.2 -> 5.4.0
- Jinja2 3.0.1 -> 3.0.3
- Mako 1.1.4 -> 1.1.5
- pycparser 2.20 -> 2.21
- pycryptodomex 3.10.1 -> 3.12.0
- PyYAML 5.4.1 -> 6.0
- requests 2.26.0 -> 2.27.1
- rsa 4.7.2 -> 4.8
- typing-extensions 3.10.0.0 -> 4.0.1
- urllib3 1.26.6 -> 1.26.8
- websocket-client 1.2.1 -> 1.2.3
- Werkzeug 2.0.1 -> 2.0.2
- zipp 3.5.0 -> 3.7.0

Frontend:

- fomantic-ui 2.8.7 -> 2.8.8

### Breaking changes

No breaking changes.
