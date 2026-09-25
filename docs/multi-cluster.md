# Multi-cluster support

GPM can show information from more than one cluster. To use this, provide a `kubeconfig` with more than one context. Each context points to a different cluster. GPM lets you choose the context (cluster) from the UI.

To run GPM in a cluster with multi-cluster support, do these steps:

1. Mount a `kubeconfig` file with the cluster access configuration in the GPM pods.
2. Set the `KUBECONFIG` environment variable to the path of the mounted `kubeconfig` file. Or mount it at `/home/nonroot/.kube/config`, and GPM detects it automatically.

> [!IMPORTANT]
> The user for the clusters must have the correct permissions. Use the [`manifests/rbac.yaml`](../manifests/rbac.yaml) file as a reference.
>
> The cluster where GPM runs must reach the other clusters. This needs network connectivity.

When you run GPM locally, you already use a `kubeconfig` file to connect to the clusters. You see all your contexts and can switch between them from the UI.

## AWS IAM Authentication

To use a kubeconfig with IAM authentication, you must customize the GPM container image. The IAM authentication uses external AWS binaries. The image does not include them by default.

You can customize the container image with a `Dockerfile` like this one:

```Dockerfile
FROM curlimages/curl:7.81.0 as downloader
RUN curl https://github.com/kubernetes-sigs/aws-iam-authenticator/releases/download/v0.5.5/aws-iam-authenticator_0.5.5_linux_amd64 --output /tmp/aws-iam-authenticator
RUN chmod +x /tmp/aws-iam-authenticator

FROM quay.io/sighup/gatekeeper-policy-manager:v2.1.0
COPY --from=downloader --chown=root:root /tmp/aws-iam-authenticator /usr/local/bin/
```

You can also add the `aws` CLI for debugging. Use the same approach as before.

> [!NOTE]
> Make sure that your `kubeconfig` has the `apiVersion` set as `client.authentication.k8s.io/v1beta1`
>
> You can read more [in this issue](https://github.com/sighupio/gatekeeper-policy-manager/issues/330).
