# Helm Chart for Gatekeper Policy Manager(GPM)
A Helm chart that deploys Gatekeeper Policy Manager on Kubernetes.

## Before you start

Before you install this chart you must create a namespace for it, this is due to the order in which the resources in the charts are applied (Helm collects all of the resources in a given Chart and it's dependencies, groups them by resource type, and then installs them in a predefined order.

## Installation

[Helm](https://helm.sh) must be installed to use the charts.
Please refer to Helm's [documentation](https://helm.sh/docs/) to get started.

If you have custom options or values you want to override:
Navigate to the chart/ folder and 

**On Helm3**
```bash
  helm install gpm . --namespace gpm -f my-gpm-values.yaml
```
* Specific versions of the chart can be installed using the `--version` option, with the default being the latest release.
* You can also use `--dry-run --debug` flags to see the computed values.

As part of this chart many different pods and services are installed which all
have varying resource requirements.

## Chart Values
Refer to values.yaml template to override the defaults. The values can be overriden at runtime using `--set-string` option. 

For example : 
By setting the value of securityContext.runAsUser to "", K8s chooses a valid User.
`--set-string securityContext.runAsUser=""`