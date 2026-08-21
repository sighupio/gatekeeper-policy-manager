# Gatekeeper Policy Manager Helm Chart - v0.19.0

A Helm chart for Gatekeeper Policy Manager, a simple to use, read-only web UI for viewing OPA Gatekeeper policies' status in a Kubernetes Cluster.

## Configuration options

The following table lists the configurable parameters of the Gatekeeper Policy Manager chart and their default values.

| Parameter | Description | Default |
| --------- | ----------- | ------- |
| `replicaCount` | How many GPM pods to run. GPM holds no state, so more than one is safe. | 1 |
| `image.repository` | Container image to run. | "quay.io/sighup/gatekeeper-policy-manager" |
| `image.pullPolicy` | When the kubelet pulls the image. | "IfNotPresent" |
| `image.tag` | Image tag. Defaults to the chart's appVersion when empty. | "v2.0.0" |
| `command` | Overrides the image entrypoint. | null |
| `args` | Overrides the arguments passed to the entrypoint. | null |
| `imagePullSecrets` | Secrets that hold the credentials for a private registry. | [] |
| `nameOverride` | Replaces the chart name in the generated resource names. | "" |
| `fullnameOverride` | Replaces the whole generated resource name. | "" |
| `serviceAccount.create` | Create a ServiceAccount for GPM. | true |
| `serviceAccount.annotations` | Annotations for the ServiceAccount, for example a cloud IAM role. | {} |
| `serviceAccount.name` | Name of the ServiceAccount to use. Generated when empty. | "gatekeeper-policy-manager" |
| `podAnnotations` | Annotations added to the GPM pods. | {} |
| `podLabels` | Labels added to the GPM pods. | {} |
| `podSecurityContext.runAsNonRoot` | Refuse to start the pod as root. | true |
| `securityContext.runAsNonRoot` | Refuse to start the container as root. | true |
| `securityContext.privileged` | Run the container privileged. GPM never needs this. | false |
| `securityContext.allowPrivilegeEscalation` | Let a process gain more privileges than its parent. | false |
| `securityContext.seccompProfile.type` | Seccomp profile for the container. | "RuntimeDefault" |
| `securityContext.capabilities.drop` | Linux capabilities to drop. GPM needs none. | ["ALL"] |
| `service.annotations` | Annotations for the Service, for example load balancer settings. | {} |
| `service.type` | Service type that exposes GPM inside the cluster. | "ClusterIP" |
| `service.port` | Port the Service listens on. | 80 |
| `ingress.enabled` | Create an Ingress for GPM. | false |
| `ingress.annotations` | Annotations for the Ingress, for example the ingress class. | {} |
| `ingress.labels` | Labels for the Ingress. | {} |
| `ingress.hosts` | Hosts the Ingress answers on, with their paths. | [{"host": "gpm.local", "paths": []}] |
| `ingress.tls` | TLS certificates for the Ingress hosts. | [] |
| `resources.requests.cpu` | CPU the scheduler reserves for GPM. | "100m" |
| `resources.requests.memory` | Memory the scheduler reserves for GPM. | "128Mi" |
| `resources.limits.cpu` | CPU ceiling for GPM. | "500m" |
| `resources.limits.memory` | Memory ceiling for GPM. The pod is killed above it. | "256Mi" |
| `nodeSelector` | Node labels the pod must match. | {} |
| `tolerations` | Taints the pod tolerates. | [] |
| `affinity` | Affinity and anti-affinity rules for the pod. | {} |
| `topologySpreadConstraints` | How the pods spread over zones or nodes. | [] |
| `config.preferredURLScheme` | Set to https when GPM is served over TLS, so the session cookie is marked Secure. | "http" |
| `config.logLevel` | How much GPM writes to its log. | "info" |
| `config.eventsSource` | Which Gatekeeper components' events the Events view shows. | null |
| `config.eventsNamespace` | Read events from this namespace only. Every namespace when empty. | null |
| `config.secretKey` | Key that signs and encrypts the session cookie. Required with OIDC. | null |
| `config.secretRef` | Name of an existing Secret holding the session key, instead of secretKey. | null |
| `config.rbacFiltering.enabled` | Show each person only the views and objects their Kubernetes account can read. | false |
| `config.rbacFiltering.usernameClaim` | ID-token claim holding the username the API server knows. | null |
| `config.rbacFiltering.usernamePrefix` | Prefix the API server's --oidc-username-prefix adds, for example oidc:. | null |
| `config.rbacFiltering.groupsClaim` | ID-token claim listing the person's groups. | null |
| `config.rbacFiltering.groupsPrefix` | Prefix the API server's --oidc-groups-prefix adds. | null |
| `config.multiCluster.enabled` | Read more than one cluster, from the kubeconfig below. | false |
| `config.multiCluster.kubeconfig` | Kubeconfig naming one context per cluster GPM reads. | "apiVersion: v1\nclusters:\n- cluster:\n    certificate-authority-data: REDACTED\n    server: https://127.0.0.1:54216\n  name: kind-kind\ncontexts:\n- context:\n    cluster: kind-kind\n    user: kind-kind\n  name: kind-kind\ncurrent-context: kind-kind\nkind: Config\npreferences: {}\nusers:\n- name: kind-kind\n  user:\n    client-certificate-data: REDACTED\n    client-key-data: REDACTED\n" |
| `config.oidc.enabled` | Require a login through an OIDC provider. | false |
| `config.oidc.issuer` | Issuer URL. GPM discovers the rest of the provider's configuration from it. | null |
| `config.oidc.redirectDomain` | Public address of GPM. The provider sends people back to it. | null |
| `config.oidc.clientID` | Client ID registered with the provider. | null |
| `config.oidc.clientSecret` | Client secret, when the client is confidential. | null |
| `config.oidc.scopes` | Extra scopes for the login request. openid, profile and email are always asked for. | null |
| `config.oidc.authorizationEndpoint` | Authorization endpoint. Setting any endpoint turns discovery off. | null |
| `config.oidc.jwksURI` | JWKS URI. See the note on the authorization endpoint. | null |
| `config.oidc.tokenEndpoint` | Token endpoint. See the note on the authorization endpoint. | null |
| `config.oidc.introspectionEndpoint` | Accepted for compatibility with GPM 1.x. Not used. | null |
| `config.oidc.userinfoEndpoint` | Accepted for compatibility with GPM 1.x. Not used. | null |
| `config.oidc.endSessionEndpoint` | End session endpoint. A logout from GPM then ends the provider session too. | null |
| `extraEnvs` | Extra environment variables for the GPM container. | [] |
| `rbac.create` | Create the RBAC resources GPM needs to read the cluster. | true |
| `clusterRole.create` | Create the ClusterRole. Turn off to bind one you manage yourself. | true |
| `clusterRole.name` | Name of the ClusterRole to create or to bind. | "gatekeeper-policy-manager-crd-view" |
| `livenessProbe.enabled` | Restart the container when the liveness probe fails. | true |
| `livenessProbe.httpGet.path` | Path the liveness probe requests. | "/health" |
| `livenessProbe.httpGet.port` | Port the liveness probe requests. | "http" |
| `livenessProbe.initialDelaySeconds` | Wait this long before the first liveness probe. | 10 |
| `livenessProbe.periodSeconds` | Seconds between liveness probes. | 10 |
| `livenessProbe.timeoutSeconds` | Seconds a liveness probe waits for an answer. | 1 |
| `livenessProbe.successThreshold` | Successes needed to count the container as live again. | 1 |
| `livenessProbe.failureThreshold` | Failures before the container is restarted. | 3 |
| `readinessProbe.enabled` | Keep the pod out of the Service until the readiness probe passes. | true |
| `readinessProbe.httpGet.path` | Path the readiness probe requests. | "/health" |
| `readinessProbe.httpGet.port` | Port the readiness probe requests. | "http" |
| `readinessProbe.initialDelaySeconds` | Wait this long before the first readiness probe. | 5 |
| `readinessProbe.periodSeconds` | Seconds between readiness probes. | 5 |
| `readinessProbe.timeoutSeconds` | Seconds a readiness probe waits for an answer. | 1 |
| `readinessProbe.successThreshold` | Successes needed to count the pod as ready. | 1 |
| `readinessProbe.failureThreshold` | Failures before the pod is taken out of the Service. | 3 |

