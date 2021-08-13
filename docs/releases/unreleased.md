# v0.5.0

## Changes from v0.4.2

- Added support for switching the current context from the UI, enabling basic multicluster support.
- Added "report" view for constraints violations.
- Improved code to make it more "flasky".
- Changed base docker image to a simpler one that doesn't run as root.

### Bugfixes

- Fixed a bug in the way ConstraintTemplates' Parameters Schema was shown when having a property that itself has more than one property. Properties will appear more than once inside other properties.

## Updated dependencies

- astroid 2.5.6 -> 2.6.6
- certifi 2020.12.5 -> 2021.5.30
- cffi 1.14.5 -> 1.14.6
- click 7.1.2 -> 8.0.1
- Flask 1.1.2 -> 2.0.1
- google-auth 1.30.0 -> 1.34.0
- gunicorn 20.1.0 (new)
- importlib-metadata 4.0.1 -> 4.6.3
- importlib-resources 5.1.2 -> 5.2.2
- isort 5.8.0 -> 5.9.3
- itsdangerous 1.1.0 -> 2.0.1
- Jinja2 2.11.3 -> 3.0.1
- kubernetes 12.0.1 -> 17.17.0
- MarkupSafe 1.1.1 -> 2.0.1
- oauthlib 3.1.0 -> 3.1.1
- oic 1.2.1 -> 1.3.0
- pylint 2.8.2 -> 2.9.6
- python-dateutil 2.8.1 -> 2.8.2
- requests 2.25.1 -> 2.26.0
- six 1.15.0 -> 1.16.0
- typing-extensions 3.7.4.3 -> 3.10.0.0
- urllib3 1.26.4 -> 1.26.6
- websocket-client 0.58.0 -> 1.2.1
- Werkzeug 1.0.1 -> 2.0.1
- zipp 3.4.1 -> 3.5.0

## Breaking changes

No breaking changes.
