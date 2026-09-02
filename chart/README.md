# Helm Values

## Global Settings

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| global.imagePullSecrets | list | `[]` | Image pull secrets applied to Unifabric controller and agent pods. |
| global.registry | string | `""` | Registry prepended to chart-managed images when an image-specific registry is not set. |

## Topology Discovery

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| topoDiscovery.scaleOut.mode | string | `"unifabric-roce"` | Scale-out discovery mode. Options: nv-topograph (NVIDIA Topograph) or unifabric-roce (built-in LLDP discovery). |
| topoDiscovery.scaleOut.nodeLabel.keyTemplate | string | `"scale-out.unifabric.io/tier-{{ .Tier }}"` | Go text/template for scale-out topology label keys. It must contain exactly one {{ .Tier }} action. |
| topoDiscovery.scaleUp.mode | string | `"manual"` | Scale-up discovery mode. Options: nv-topograph (NVIDIA Topograph) or manual (labels are managed externally). |
| topoDiscovery.scaleUp.nodeLabel.keyTemplate | string | `"scale-up.unifabric.io/tier-{{ .Tier }}"` | Go text/template for scale-up topology label keys. It must contain exactly one {{ .Tier }} action. |
| topoDiscovery.storage.mode | string | `"unifabric-roce"` | Storage discovery mode. Currently supported: unifabric-roce (RoCE storage). |
| topoDiscovery.storage.nodeLabel.keyTemplate | string | `"storage.unifabric.io/tier-{{ .Tier }}"` | Go text/template for storage topology label keys. It must contain exactly one {{ .Tier }} action. |

`topoDiscovery.*.mode` is the only topology-writer switch. The former
`topologyLabels`, `internalTopologyLabelWriter.enabled`,
`switchTopologyDiscovery`, `switchDiscovery`, `switchAgent`,
`nodeTopologyDiscovery`, `nodeDiscovery`, and `nvidiaTopograph.enable` values
are rejected with migration errors instead of being silently ignored. The
former component-level NVIDIA blocks are replaced by the shared
`nvidiaTopograph.image` and GPU-node values.

## Switch Subscription

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| switchSubscription.defaultGrpcPort | int | `8090` | Default gRPC port used when a Switch resource does not set spec.grpcPort. |
| switchSubscription.ignorePortPatterns | list | `["mgmt*","Management*","oob*"]` | Glob patterns for switch local ports ignored before topology graph construction. |
| switchSubscription.mtls.controllerSecretName | string | `"switch-controller-mtls-controller"` | Secret mounted into the controller containing tls.crt, tls.key, and peer.crt. |
| switchSubscription.mtls.mode | string | `"auto"` | Certificate mode: auto generates Helm-managed Secrets, existing uses pre-created Secrets, and disabled uses plaintext gRPC. |
| switchSubscription.mtls.switchAgentSecretName | string | `"switch-controller-mtls-agent"` | Secret exported for switch agents containing tls.crt, tls.key, and peer.crt. |

Per-switch addresses and port overrides belong to `Switch.spec`. gRPC transport
tuning uses Controller defaults, while `switchSubscription.mtls.mode` selects
auto-generated, existing, or disabled mTLS.

## FabricNode Reporting and Observability

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| fabricNode.initialScanDelay | string | `"1m"` | Delay before the first agent scan, allowing lldpd to learn neighbors. |
| fabricNode.refreshInterval | string | `"1m"` | Interval between agent refreshes of local RDMA interfaces and LLDP neighbors. |
| fabricNode.scaleOutInterfaceSelector | string | `""` | RDMA interfaces included in scale-out topology and metrics. Supports selectors such as cidr=172.17.0.0/16 or interface=eth*,!eth9. |
| fabricNode.scaleUpInterfaceSelector | string | `""` | RDMA interfaces classified as scale-up and excluded from scale-out topology. Leave empty when no dedicated scale-up RDMA network exists. |
| fabricNode.storageInterfaceSelector | string | `""` | RDMA interfaces classified as storage and excluded from scale-out topology. Leave empty when no dedicated storage RDMA network exists. |
| nodeMetrics.enabled | bool | `true` | Enable RDMA metrics collection and expose the agent metrics Service. |
| nodeMetrics.path | string | `"/metrics"` | HTTP path served by the agent metrics endpoint. |
| nodeMetrics.port | int | `8082` | Container port used by the agent metrics endpoint. |
| nodeMetrics.service.port | int | `8086` | Service port used by Prometheus to scrape agent RDMA metrics. |
| nodeMetrics.service.type | string | `"ClusterIP"` | Kubernetes Service type for agent RDMA metrics. |
| nodeMetrics.serviceMonitor.enabled | bool | `true` | Create a Prometheus Operator ServiceMonitor for agent RDMA metrics. |
| nodeMetrics.serviceMonitor.interval | string | `"15s"` | Prometheus scrape interval for agent RDMA metrics. |
| nodeMetrics.serviceMonitor.labels | object | `{}` | Extra labels added to the ServiceMonitor for Prometheus selection. |
| nodeMetrics.serviceMonitor.path | string | `"/metrics"` | HTTP path scraped by the ServiceMonitor. |
| nodeMetrics.serviceMonitor.scrapeTimeout | string | `"10s"` | Prometheus scrape timeout for agent RDMA metrics. |
| grafanaDashboard.allowCrossNamespaceImport | bool | `true` | Allow Grafana Operator to import GrafanaDashboard resources across namespaces. |
| grafanaDashboard.enabled | bool | `true` | Render the bundled RDMA cluster, node, pod, and workload dashboards. |
| grafanaDashboard.instanceSelector | object | `{}` | Grafana Operator instance selector for importing GrafanaDashboard resources. |
| grafanaDashboard.kind | string | `"GrafanaDashboard"` | Dashboard resource kind. Use ConfigMap for Grafana sidecar import, or GrafanaDashboard for Grafana Operator. |
| grafanaDashboard.labels | object | `{}` | Extra labels added to rendered dashboard resources. |

## sFlow Processing

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| sflow.affinity | object | `{}` | Affinity rules applied to the sFlow collector Pod. |
| sflow.clickhouse.address | string | `""` | ClickHouse native protocol address used by the sFlow collector. |
| sflow.clickhouse.database | string | `"default"` | ClickHouse database containing the flows_raw table. |
| sflow.clickhouse.managed.affinity | object | `{}` | Affinity rules applied to the managed ClickHouse Pod. |
| sflow.clickhouse.managed.containerSecurityContext | object | `{}` | Container security context applied to the managed ClickHouse container. |
| sflow.clickhouse.managed.enabled | bool | `false` | Deploy a single-node ClickHouse instance for sFlow flow records. |
| sflow.clickhouse.managed.image.pullPolicy | string | `"IfNotPresent"` | Container image pull policy for the managed ClickHouse server. |
| sflow.clickhouse.managed.image.registry | string | `"docker.io"` | Container image registry for the managed ClickHouse server. |
| sflow.clickhouse.managed.image.repository | string | `"clickhouse/clickhouse-server"` | Container image repository for the managed ClickHouse server. |
| sflow.clickhouse.managed.image.tag | string | `"26.3"` | Container image tag for the managed ClickHouse server. |
| sflow.clickhouse.managed.nodeSelector | object | `{}` | Node selector applied to the managed ClickHouse Pod. |
| sflow.clickhouse.managed.persistence.accessModes | list | `["ReadWriteOnce"]` | Access modes for the managed ClickHouse PVC. |
| sflow.clickhouse.managed.persistence.enabled | bool | `true` | Persist managed ClickHouse data. When false, data uses emptyDir. |
| sflow.clickhouse.managed.persistence.hostPath.path | string | `""` | Host path used when persistence.type=hostPath. The ClickHouse Pod must stay on a node that has this path. |
| sflow.clickhouse.managed.persistence.hostPath.type | string | `"DirectoryOrCreate"` | Kubernetes hostPath type used when persistence.type=hostPath. |
| sflow.clickhouse.managed.persistence.size | string | `"20Gi"` | Requested size for managed ClickHouse data. |
| sflow.clickhouse.managed.persistence.storageClassName | string | `""` | StorageClass for the managed ClickHouse PVC. Empty uses the cluster default. |
| sflow.clickhouse.managed.persistence.type | string | `"pvc"` | Persistence backend for managed ClickHouse data when enabled. Supported values: pvc, hostPath. |
| sflow.clickhouse.managed.podAnnotations | object | `{}` | Additional annotations added to the managed ClickHouse Pod. |
| sflow.clickhouse.managed.podLabels | object | `{}` | Additional labels added to the managed ClickHouse Pod. |
| sflow.clickhouse.managed.podSecurityContext | object | `{}` | Pod security context applied to the managed ClickHouse Pod. |
| sflow.clickhouse.managed.resources.limits.cpu | string | `"2"` | CPU limit for the managed ClickHouse container. |
| sflow.clickhouse.managed.resources.limits.memory | string | `"4Gi"` | Memory limit for the managed ClickHouse container. |
| sflow.clickhouse.managed.resources.requests.cpu | string | `"500m"` | CPU requested by the managed ClickHouse container. |
| sflow.clickhouse.managed.resources.requests.memory | string | `"1Gi"` | Memory requested by the managed ClickHouse container. |
| sflow.clickhouse.managed.service.httpPort | int | `8123` | HTTP port exposed by the managed ClickHouse Service. |
| sflow.clickhouse.managed.service.nativePort | int | `9000` | Native protocol port exposed by the managed ClickHouse Service. |
| sflow.clickhouse.managed.tolerations | list | `[]` | Tolerations applied to the managed ClickHouse Pod. |
| sflow.clickhouse.password | string | `""` | Inline ClickHouse password. Prefer passwordSecret for production installs. |
| sflow.clickhouse.passwordSecret.key | string | `""` | Secret key containing the ClickHouse password. Defaults to "password" when a Secret name is provided. |
| sflow.clickhouse.passwordSecret.name | string | `""` | Secret name containing the ClickHouse password. |
| sflow.clickhouse.schema.lock.leaseDuration | string | `"10s"` | Duration before another sFlow replica can take over a stale schema migration Lease. |
| sflow.clickhouse.schema.lock.name | string | `"unifabric-sflow-clickhouse-schema"` | Kubernetes Lease name used to serialize ClickHouse schema migrations across sFlow replicas. |
| sflow.clickhouse.schema.lock.namespace | string | `""` | Kubernetes namespace for the schema migration Lease. Defaults to the chart release namespace in rendered config. |
| sflow.clickhouse.schema.lock.retryInterval | string | `"2s"` | Retry interval used while waiting for the schema migration Lease. |
| sflow.clickhouse.schema.retentionDays | int | `3` | Retention window in days for the flows_raw table. Must be at least 1. The collector reconciles this table TTL during startup. |
| sflow.clickhouse.table | string | `"flows_raw"` | ClickHouse table that stores enriched sFlow rows. |
| sflow.clickhouse.username | string | `"default"` | ClickHouse username used by the sFlow collector. |
| sflow.config.logLevel | string | `"info"` | Log level used by the sFlow collector. |
| sflow.containerSecurityContext.allowPrivilegeEscalation | bool | `false` | Allow privilege escalation in the sFlow collector container. |
| sflow.containerSecurityContext.capabilities.drop | list | `["ALL"]` | Linux capabilities dropped from the sFlow collector container. |
| sflow.containerSecurityContext.readOnlyRootFilesystem | bool | `true` | Mount the sFlow collector root filesystem as read-only. |
| sflow.enabled | bool | `false` | Enable the sFlow collector deployment and UDP service. |
| sflow.healthProbe.bindAddress | string | `":8085"` | Health probe bind address used by the sFlow collector. |
| sflow.healthProbe.port | int | `8085` | Health probe container port exposed by the sFlow collector. |
| sflow.image.pullPolicy | string | `"IfNotPresent"` | Container image pull policy for the sFlow collector. |
| sflow.image.registry | string | `"ghcr.io"` | Container image registry for the sFlow collector. |
| sflow.image.repository | string | `"unifabric-io/unifabric-sflow"` | Container image repository for the sFlow collector. |
| sflow.image.tag | string | `""` | Container image tag for the sFlow collector. Defaults to the chart appVersion when empty. |
| sflow.listen.bindAddress | string | `":6343"` | UDP bind address used by the sFlow collector container. |
| sflow.listen.port | int | `6343` | UDP container port used by the sFlow collector. |
| sflow.metrics.bindAddress | string | `":8084"` | Metrics bind address used by the sFlow collector. |
| sflow.metrics.path | string | `"/metrics"` | Metrics path exposed by the sFlow collector. |
| sflow.metrics.port | int | `8084` | Metrics container port exposed by the sFlow collector. |
| sflow.nodeSelector | object | `{}` | Node selector applied to the sFlow collector Pod. |
| sflow.podAnnotations | object | `{}` | Additional annotations added to the sFlow collector Pod. |
| sflow.podLabels | object | `{}` | Additional labels added to the sFlow collector Pod. |
| sflow.podSecurityContext.fsGroup | int | `65532` | Filesystem group ID used by the sFlow collector Pod. |
| sflow.podSecurityContext.runAsGroup | int | `65532` | Group ID used by the sFlow collector container. |
| sflow.podSecurityContext.runAsNonRoot | bool | `true` | Run the sFlow collector as a non-root user. |
| sflow.podSecurityContext.runAsUser | int | `65532` | User ID used by the sFlow collector container. |
| sflow.podSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` | Seccomp profile type applied to the sFlow collector Pod. |
| sflow.replicaCount | int | `1` | Number of sFlow collector replicas to run. |
| sflow.resources.limits.cpu | string | `"500m"` | CPU limit for the sFlow collector container. |
| sflow.resources.limits.memory | string | `"512Mi"` | Memory limit for the sFlow collector container. |
| sflow.resources.requests.cpu | string | `"100m"` | CPU requested by the sFlow collector container. |
| sflow.resources.requests.memory | string | `"128Mi"` | Memory requested by the sFlow collector container. |
| sflow.service.annotations | object | `{}` | Additional annotations added to the sFlow UDP Service. |
| sflow.service.enabled | bool | `true` | Enable the UDP Service for switch sFlow exporters. |
| sflow.service.nodePort | int | `0` | Optional UDP nodePort used by switch sFlow exporters. Set to 0 to let Kubernetes allocate one. |
| sflow.service.port | int | `6343` | UDP Service port used by switch sFlow exporters. |
| sflow.service.type | string | `"NodePort"` | Service type used for switch sFlow exporters. NodePort is the default because sFlow datagrams usually arrive from switches outside the cluster. |
| sflow.serviceAccount.annotations | object | `{}` | Additional annotations added to the sFlow collector ServiceAccount. |
| sflow.serviceAccount.create | bool | `true` | Create a dedicated ServiceAccount for the sFlow collector. |
| sflow.serviceAccount.name | string | `""` | ServiceAccount name for the sFlow collector. Defaults to the release-derived name when empty. |
| sflow.tolerations | list | `[]` | Tolerations applied to the sFlow collector Pod. |
| sflow.writer.batchSize | int | `2000` | Maximum sFlow records per ClickHouse write batch. |
| sflow.writer.flushInterval | string | `"2s"` | Maximum delay before flushing a non-empty write batch. |
| sflow.writer.queueSize | int | `65536` | Maximum records buffered before overload drops begin. |

## Controller and Agent

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| controller.affinity | object | `{}` | Affinity rules for scheduling controller pods. |
| controller.config.healthProbe.bindAddress | string | `":8081"` | Controller health and readiness bind address inside the container. |
| controller.config.leaderElection.enabled | bool | `true` | Enable Kubernetes leader election for the controller. |
| controller.config.leaderElection.id | string | `"unifabric-controller"` | Leader election lease identifier. |
| controller.config.leaderElection.namespace | string | `""` | Namespace used for leader election leases. Empty uses the release namespace. |
| controller.config.logLevel | string | `"info"` | Controller log level. Valid values are debug, info, warn, and error. |
| controller.config.metrics.bindAddress | string | `":8080"` | Controller metrics bind address inside the container. |
| controller.config.pprof.bindAddress | string | `""` | Controller pprof bind address. Leave empty to disable pprof. |
| controller.containerSecurityContext.allowPrivilegeEscalation | bool | `false` | Prevent privilege escalation in the controller container. |
| controller.containerSecurityContext.capabilities.drop | list | `["ALL"]` | Linux capabilities dropped from the controller container. |
| controller.containerSecurityContext.readOnlyRootFilesystem | bool | `true` | Mount the controller container root filesystem as read-only. |
| controller.enabled | bool | `true` | Deploy the Unifabric controller. |
| controller.image.pullPolicy | string | `"IfNotPresent"` | Image pull policy for the controller container. |
| controller.image.registry | string | `"ghcr.io"` | Container image registry for the controller. |
| controller.image.repository | string | `"unifabric-io/unifabric-controller"` | Container image repository for the controller. |
| controller.image.tag | string | `""` | Container image tag for the controller. Defaults to the chart appVersion when empty. |
| controller.nodeSelector | object | `{}` | Node selector for scheduling controller pods. |
| controller.podAnnotations | object | `{}` | Annotations added to controller pods. |
| controller.podLabels | object | `{}` | Extra labels added to controller pods. |
| controller.podSecurityContext.fsGroup | int | `65532` | Filesystem group ID used by mounted controller volumes. |
| controller.podSecurityContext.runAsGroup | int | `65532` | Group ID used by controller containers. |
| controller.podSecurityContext.runAsNonRoot | bool | `true` | Run the controller pod as a non-root user. |
| controller.podSecurityContext.runAsUser | int | `65532` | User ID used by controller containers. |
| controller.podSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` | Seccomp profile applied to controller pods. |
| controller.ports.health | int | `8081` | Controller health probe container port. |
| controller.ports.metrics | int | `8080` | Controller metrics container port. |
| controller.replicaCount | int | `1` | Number of controller replicas. |
| controller.resources.limits.cpu | string | `"500m"` | Controller CPU limit. |
| controller.resources.limits.memory | string | `"512Mi"` | Controller memory limit. |
| controller.resources.requests.cpu | string | `"100m"` | Controller CPU request. |
| controller.resources.requests.memory | string | `"128Mi"` | Controller memory request. |
| controller.service.enabled | bool | `true` | Create a Service for the controller metrics endpoint. |
| controller.service.port | int | `8080` | Service port for controller metrics. |
| controller.service.type | string | `"ClusterIP"` | Kubernetes Service type for the controller metrics endpoint. |
| controller.serviceAccount.annotations | object | `{}` | Annotations added to the controller ServiceAccount. |
| controller.serviceAccount.create | bool | `true` | Create a ServiceAccount for the controller. |
| controller.serviceAccount.name | string | `""` | Controller ServiceAccount name. Defaults to the generated controller name when empty. |
| controller.tolerations | list | `[]` | Tolerations for scheduling controller pods. |
| agent.affinity.nodeAffinity | object | See chart/values.yaml for the full default node affinity. | Default Linux node affinity for agent pods. |
| agent.config.healthProbe.bindAddress | string | `":8083"` | Agent health and readiness bind address inside the container. |
| agent.config.logLevel | string | `"info"` | Agent log level. Valid values are debug, info, warn, and error. |
| agent.config.metrics.bindAddress | string | `":8082"` | Agent metrics bind address inside the container. |
| agent.config.node.defaultRouteProbe | string | `"8.8.8.8:53"` | host:port used to infer the default route when classifying local interfaces. |
| agent.config.node.name | string | `""` | Kubernetes node name override. Leave empty to use the pod NODE_NAME environment value. |
| agent.config.node.role | string | `"gpu"` | Agent node role. Valid values are gpu and storage. |
| agent.containerSecurityContext.privileged | bool | `true` | Run the agent container as privileged so it can inspect host RDMA and network state. |
| agent.enabled | bool | `true` | Deploy the Unifabric node agent DaemonSet. |
| agent.hostNetwork | bool | `true` | Run agent pods in the host network namespace. |
| agent.hostPID | bool | `true` | Run agent pods in the host PID namespace. |
| agent.image.pullPolicy | string | `"IfNotPresent"` | Image pull policy shared by the agent and lldpd sidecar. |
| agent.image.registry | string | `"ghcr.io"` | Container image registry shared by the agent and lldpd sidecar. |
| agent.image.repository | string | `"unifabric-io/unifabric-agent"` | Container image repository shared by the agent and lldpd sidecar. |
| agent.image.tag | string | `""` | Container image tag shared by the agent and lldpd sidecar. Defaults to the chart appVersion when empty. |
| agent.lldp.containerSecurityContext.privileged | bool | `true` | Run the lldpd sidecar as privileged so it can access host network interfaces. |
| agent.lldp.enabled | bool | `true` | Run the lldpd sidecar used by the agent to collect LLDP neighbors. |
| agent.lldp.extraConfig | string | `""` | Additional raw lldpd configuration appended to lldpd.conf. |
| agent.lldp.interfaces | string | `""` | Interface pattern passed to lldpd for LLDP discovery. Leave empty to let lldpd use its defaults. |
| agent.lldp.managementIPPattern | string | `""` | lldpd management IP pattern written to lldpd.conf. Leave empty to let lldpd choose. |
| agent.lldp.resources.limits.cpu | string | `"100m"` | lldpd sidecar CPU limit. |
| agent.lldp.resources.limits.memory | string | `"128Mi"` | lldpd sidecar memory limit. |
| agent.lldp.resources.requests.cpu | string | `"50m"` | lldpd sidecar CPU request. |
| agent.lldp.resources.requests.memory | string | `"64Mi"` | lldpd sidecar memory request. |
| agent.lldp.txInterval | int | `30` | LLDP transmit interval, in seconds, configured for lldpd. |
| agent.nodeSelector | object | `{}` | Node selector for scheduling agent pods. |
| agent.podAnnotations | object | `{}` | Annotations added to agent pods. |
| agent.podLabels | object | `{}` | Extra labels added to agent pods. |
| agent.podSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` | Seccomp profile applied to agent pods. |
| agent.ports.health | int | `8083` | Agent health probe container port. |
| agent.ports.metrics | int | `8082` | Agent metrics container port. |
| agent.resources.limits.cpu | string | `"500m"` | Agent CPU limit. |
| agent.resources.limits.memory | string | `"512Mi"` | Agent memory limit. |
| agent.resources.requests.cpu | string | `"100m"` | Agent CPU request. |
| agent.resources.requests.memory | string | `"128Mi"` | Agent memory request. |
| agent.serviceAccount.annotations | object | `{}` | Annotations added to the agent ServiceAccount. |
| agent.serviceAccount.create | bool | `true` | Create a ServiceAccount for the agent. |
| agent.serviceAccount.name | string | `""` | Agent ServiceAccount name. Defaults to the generated agent name when empty. |
| agent.tolerations | list | `[]` | Tolerations for scheduling agent pods. |

## NVIDIA Topograph

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| nvidiaTopograph.credentialsSecretName | string | `""` | Existing Secret containing a credentials.yaml key. Required by the netq provider; credentials are mounted only into the topograph Pod. |
| nvidiaTopograph.gpuNodeSelector | object | `{"nvidia.com/gpu.present":"true"}` | Selects GPU Nodes observed by node-observer and running node-data-broker. |
| nvidiaTopograph.gpuNodeTolerations | list | `[]` | Tolerations applied to node-data-broker on selected GPU Nodes. |
| nvidiaTopograph.image.pullPolicy | string | `"IfNotPresent"` | NVIDIA Topograph image pull policy. |
| nvidiaTopograph.image.registry | string | `"ghcr.io"` | NVIDIA Topograph image registry. |
| nvidiaTopograph.image.repository | string | `"nvidia/topograph"` | NVIDIA Topograph image repository. |
| nvidiaTopograph.image.tag | string | `"v0.5.0"` | NVIDIA Topograph image tag. |
| nvidiaTopograph.provider.name | string | `"infiniband-k8s"` | Provider name. Use infiniband-k8s for InfiniBand or netq for Spectrum-X. |
| nvidiaTopograph.provider.params | object | `{}` | Provider-specific parameters, such as apiUrl for netq. |
| nvidiaTopograph.useGpuCliqueLabel | bool | `true` | Use the GPU Operator clique label as the accelerator topology source for infiniband-k8s. |
