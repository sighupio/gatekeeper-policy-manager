# Gatekeeper Policy Manager Helm Chart - v0.14.0

A Helm chart for Gatekeeper Policy Manager, a simple to use, read-only web UI for viewing OPA Gatekeeper policies' status in a Kubernetes Cluster.

## Configuration options

The following table lists the configurable parameters of the Gatekeeper Policy Manager chart and their default values.

| Parameter | Description | Default |
| --------- | ----------- | ------- |
| `replicaCount` |  | 1 |
| `image.repository` |  | "quay.io/sighup/gatekeeper-policy-manager" |
| `image.pullPolicy` |  | "IfNotPresent" |
| `image.tag` |  | "v1.0.14" |
| `command` |  | null |
| `args` |  | null |
| `imagePullSecrets` |  | [] |
| `nameOverride` |  | "" |
| `fullnameOverride` |  | "" |
| `serviceAccount.create` |  | true |
| `serviceAccount.annotations` |  | {} |
| `serviceAccount.name` |  | "gatekeeper-policy-manager" |
| `podAnnotations` |  | {} |
| `podLabels` |  | {} |
| `podSecurityContext.runAsNonRoot` |  | true |
| `securityContext.runAsNonRoot` |  | true |
| `securityContext.privileged` |  | false |
| `securityContext.allowPrivilegeEscalation` |  | false |
| `securityContext.seccompProfile.type` |  | "RuntimeDefault" |
| `securityContext.capabilities.drop` |  | ["ALL"] |
| `service.annotations` |  | {} |
| `service.type` |  | "ClusterIP" |
| `service.port` |  | 80 |
| `ingress.enabled` |  | false |
| `ingress.annotations` |  | {} |
| `ingress.labels` |  | {} |
| `ingress.hosts` |  | [{"host": "gpm.local", "paths": []}] |
| `ingress.tls` |  | [] |
| `resources.requests.cpu` |  | "100m" |
| `resources.requests.memory` |  | "128Mi" |
| `resources.limits.cpu` |  | "500m" |
| `resources.limits.memory` |  | "256Mi" |
| `autoscaling.enabled` |  | false |
| `autoscaling.minReplicas` |  | 1 |
| `autoscaling.maxReplicas` |  | 5 |
| `autoscaling.targetCPUUtilizationPercentage` |  | 80 |
| `autoscaling.targetMemoryUtilizationPercentage` |  | 80 |
| `autoscaling.behavior` |  | {} |
| `autoscaling.metrics` |  | [] |
| `nodeSelector` |  | {} |
| `tolerations` |  | [] |
| `affinity` |  | {} |
| `topologySpreadConstraints` |  | [] |
| `config.preferredURLScheme` |  | "http" |
| `config.logLevel` |  | "info" |
| `config.secretKey` |  | null |
| `config.secretRef` |  | null |
| `config.multiCluster.enabled` |  | false |
| `config.multiCluster.kubeconfig` |  | "apiVersion: v1\nclusters:\n- cluster:\n    certificate-authority-data: REDACTED\n    server: https://127.0.0.1:54216\n  name: kind-kind\ncontexts:\n- context:\n    cluster: kind-kind\n    user: kind-kind\n  name: kind-kind\ncurrent-context: kind-kind\nkind: Config\npreferences: {}\nusers:\n- name: kind-kind\n  user:\n    client-certificate-data: REDACTED\n    client-key-data: REDACTED\n" |
| `config.oidc.enabled` |  | false |
| `config.oidc.issuer` |  | null |
| `config.oidc.redirectDomain` |  | null |
| `config.oidc.clientID` |  | null |
| `config.oidc.clientSecret` |  | null |
| `config.oidc.authorizationEndpoint` |  | null |
| `config.oidc.jwksURI` |  | null |
| `config.oidc.tokenEndpoint` |  | null |
| `config.oidc.introspectionEndpoint` |  | null |
| `config.oidc.userinfoEndpoint` |  | null |
| `config.oidc.endSessionEndpoint` |  | null |
| `extraEnvs` |  | [] |
| `rbac.create` |  | true |
| `clusterRole.create` |  | true |
| `clusterRole.name` |  | "gatekeeper-policy-manager-crd-view" |
| `livenessProbe.enabled` |  | true |
| `livenessProbe.httpGet.path` |  | "/health" |
| `livenessProbe.httpGet.port` |  | "http" |
| `livenessProbe.initialDelaySeconds` |  | 10 |
| `livenessProbe.periodSeconds` |  | 10 |
| `livenessProbe.timeoutSeconds` |  | 1 |
| `livenessProbe.successThreshold` |  | 1 |
| `livenessProbe.failureThreshold` |  | 3 |
| `readinessProbe.enabled` |  | true |
| `readinessProbe.httpGet.path` |  | "/health" |
| `readinessProbe.httpGet.port` |  | "http" |
| `readinessProbe.initialDelaySeconds` |  | 5 |
| `readinessProbe.periodSeconds` |  | 5 |
| `readinessProbe.timeoutSeconds` |  | 1 |
| `readinessProbe.successThreshold` |  | 1 |
| `readinessProbe.failureThreshold` |  | 3 |

