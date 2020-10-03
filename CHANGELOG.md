# Gatekeeper Policy Manager Changelog

## v0.3

- Added support for offline frontend usage.
- Added favicon.
- Updated base image and pinned OS-level dependencies packages versions.
- Added message when there are no Constraint Templates instead of showing an empty view.
- Fixed crash when constraints don't have any match criteria defined.
- Improved error handling in general

## v0.2

We are pleased to announce the release of Gatekeeper Policy Manager v0.2, changes in this new release:

- Improved layout of violations: now the violations of a Constraint are shown as a table instead of a list, this improves the readability when the count of violations is high. Also, we show a message now when there are no violations.
- Added missing "Scope" to the Constraints match criteria view.
- Show the line numbers in the rego code view ports.
- Added support for Gatekeeper "config" CRDs. Now you can view all the config CRDs as you would with the Constraints and Constraint Templates.

### Upgrade path

Just change the image tag to v0.2 or run `kustomize build | kubectl apply -f -`

We hope you enjoy it!

## v0.1

First Release.
