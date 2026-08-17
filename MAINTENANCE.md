# Gatekeeper Policy Manager Maintenance Guide

This document is for GPM's maintainers. Here you will find intructions on how to perform maintenance-related activities in the project, like releasing a new version of the app, releasing a new version of the helm chart, etc.

## Releasing a new version of GPM

Releasing a new version of GPM is done automatically with our CI, to trigger the release process follow the next steps:

1. Be sure that the current state of the `main` branch is ready to be released.
2. Be sure that you don't have any local modifications to the files.
3. [Bump the chart version](#releasing-a-new-helm-chart-version).
4. Create the release notes.
5. Commit all changes.
6. Run [`bumpversion`](https://github.com/c4urself/bump2version/#installation) to update the version strings automatically everywhere.

For example, assuming the latest version is 1.0.0, to release a new patch version run:

```bash
bumpversion --dry-run --verbose --new-version 1.0.1 bugfix
```

or to release a new minor:

```bash
bumpversion --dry-run --verbose --new-version 1.1.0 minor
```

> Notice that the command includes a `--dry-run` flag, drop it to actually perform the change. You can drop the `--verbose` flag too.

7. `bumpversion` will create some commits and tags. Push the commits and then the tag:

```bash
git push
git push origin <TAG>
```

## Releasing a new Helm Chart version

You should update the version in `/chart/Chart.yaml` file each time you do a release of GPM and/or when the chart content gets updated.

To release a new Helm Chart version:

1. Update the `/chart/Chart.yaml` file accordingly (i.e. bumping the version of the Chart)

2. Update the `/chart/README.md` file if you made changes to the chart. You can use [`frigate`](https://frigate.readthedocs.io/) to do it automatically:

```bash
cd chart
frigate gen . > README.md
```

> Notice that `frigate` will use the template in the file `/chart/.frigate` for formatting.

3. Tag and push the commit. This can be done as part of the release of a version of GPM or independently.

> The `release-helm-chart` pipeline packages the chart and pushes it as an OCI artifact to
> `oci://quay.io/sighup/charts/gatekeeper-policy-manager`, next to the container image on quay.io. It
> runs on any tag except release candidates (`v**-rc**`), and only after the `release` pipeline
> succeeds, so a failed GPM build cannot publish a chart that references it.
>
> If you want to release just the chart, use a tag like `helm-chart-<version>` and relax the
> dependency on the `release` pipeline.
>
> ⚠️ The first push of a new repository to quay creates it as **private**. Set
> `quay.io/sighup/charts/gatekeeper-policy-manager` to public once, or `helm install` needs
> authentication.
