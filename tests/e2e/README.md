# GPM UI E2E Tests

This folder contains the test defintions for testing UI regressions in GPM's frontend.

The best way to use locally these tests is to run them in a docker container, otherwise you might get different rendering results.

> Notice that you'll need to have a working instance of GPM accessible at `http://localhost:8080`

1. Get the container running:

```console
docker run --rm -it -v $PWD:/app mcr.microsoft.com/playwright:v1.30.0-focal
```

2. Install all the dependencies:

```console
cd app/tests/e2e
yarn install
```

3. (optional) create new baseline for the tests:

```console
yarn gen:snapshot
```

4. compare current status with the baseline:

```console
yarn test
```
