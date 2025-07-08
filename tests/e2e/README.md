# GPM UI E2E Tests

This folder contains the test definitions for testing UI regressions in GPM's frontend.

The best way to use locally these tests is to run them in a docker container, otherwise, you might get different rendering results.

> Notice that you'll need to have a working instance of GPM accessible at `http://localhost:8080`
>
> For example with:
>
> ```bash
> kubectl port-forward -n gatekeeper-system svc/gatekeeper-policy-manager 8080:80
> ```

1. Get the container running:

```console
docker run --rm -it --network=host -v $PWD:/app mcr.microsoft.com/playwright:v1.30.0-focal
```

2. Install all the dependencies:

```console
cd app/tests/e2e
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
