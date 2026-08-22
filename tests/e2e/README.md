# GPM UI E2E Tests

This folder contains the test definitions for testing UI regressions in GPM's frontend.

The best way to use locally these tests is to run them in a docker container, otherwise, you might get different rendering results.

> Notice that you'll need to have a working instance of GPM accessible at `http://localhost:8080`
>
> For example with: kubectl port-forward -n gatekeeper-system svc/gatekeeper-policy-manager 8080:80

1. Get the container running. The tag must match the `@playwright/test` version in
   `package.json`, because the image ships the browsers for that version:

```console
docker run --rm -it --add-host=host.docker.internal:host-gateway \
  -e GPM_BASE_URL=http://host.docker.internal:8080 \
  -v "$PWD":/app -w /app/tests/e2e \
  mcr.microsoft.com/playwright:v1.62.1 bash
```

> `--network=host` does not reach a host port-forward on macOS. The bridge network and
> `host.docker.internal` work on macOS and on Linux.

2. Install all the dependencies:

```console
yarn install
```

3. compare current status with the baseline:

```console
yarn test
```

4. (optional) create new baseline for the tests:

```console
yarn gen:snapshot
```

5. re-compare current status with the new baseline:

```console
yarn test
```

## Before you regenerate a baseline

Three things about the e2e cluster decide whether a new baseline is correct anywhere but your machine.

### The kapp labels must stay out of the pixels

`tests/tests.sh` deploys the fixture with kapp. kapp labels every object it manages with
`kapp.k14s.io/app`, and the value is a timestamp that changes on each deploy. GPM shows that label
in the "Full YAML definition" block of each card. Those blocks are collapsed, so no baseline holds
the label today. A spec that opens one and photographs it gets a different picture on every run.

### The Pod row's name is masked, and must stay masked

`local-path-provisioner` is the one fixture object with a generated name, for example
`local-path-provisioner-855c7b7774-nth6s`. The ReplicaSet hash and the suffix differ on every fresh
cluster, so a baseline that contains that name passes for you and fails in CI. `resources.spec.ts`
masks `[id*="--Pod--"] .rname`. It masks the name cell only, not the row: the row is a fixed-height
grid, and everything else in the fixture is a named object that belongs in the pixels.

### Do not delete the local-path-provisioner Pod

That Pod runs only because it started **before** the constraints did. `liveness-probe` denies, and
the Pod has no `livenessProbe`, so the replacement that the ReplicaSet creates after a delete is
rejected by the webhook. The Pod does not come back on its own. The namespace then shows one row
instead of two, and a baseline made in that state is wrong for CI, where a fresh cluster always has
the Pod.

To bring it back, move the constraint out of the way for one rollout:

```console
kubectl patch k8slivenessprobe liveness-probe --type merge -p '{"spec":{"enforcementAction":"dryrun"}}'
kubectl -n local-path-storage rollout restart deploy/local-path-provisioner
kubectl -n local-path-storage rollout status deploy/local-path-provisioner
kubectl patch k8slivenessprobe liveness-probe --type merge -p '{"spec":{"enforcementAction":"deny"}}'
```

The Pod returns with a new name. That is expected, and the mask above covers it.

