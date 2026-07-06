# Helm Values

## Global Settings

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| global.imagePullSecrets | list | `[]` | Image pull secrets applied to Unifabric controller and agent pods. |
| global.registry | string | `""` | Registry prepended to chart-managed images when an image-specific registry is not set. |

## Topology Labels

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| topologyLabels.scaleOutCore | string | `"unifabric.io/scale-out-core"` | Label key for the core-level scale-out topology domain on a node. |
| topologyLabels.scaleOutLeaf | string | `"unifabric.io/scale-out-leaf"` | Label key for the leaf-level scale-out topology domain on a node. |
| topologyLabels.scaleOutSpine | string | `"unifabric.io/scale-out-spine"` | Label key for the spine-level scale-out topology domain on a node. |
| topologyLabels.scaleUp | string | `"unifabric.io/scale-up"` | Label key for the scale-up topology domain on a node. |
| internalTopologyLabelWriter.enabled | bool | `true` | Let Unifabric write and clean topology Node labels internally. Disable when another component, such as NVIDIA Topograph, owns topology labels. |
| topologyGroupNaming.hashLength | int | `7` | Number of hash characters used when labelValueFormat is hash. |
| topologyGroupNaming.labelValueFormat | string | `"hash"` | Format for topology label values. Use name for readable switch-based values, or hash for compact stable values. |

## Switch Topology Discovery

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| switchTopologyDiscovery.defaultGrpcPort | int | `8090` | Default gRPC port used when a Switch resource does not set spec.grpcPort. |
| switchTopologyDiscovery.dialTimeout | string | `"5s"` | Timeout for one controller-to-switch-agent gRPC dial attempt. |
| switchTopologyDiscovery.enabled | bool | `false` | Enable controller-to-switch-agent subscriptions for switch-side topology discovery. |
| switchTopologyDiscovery.ignoreSwitchPorts | list | `["mgmt*","Management*","oob*"]` | Switch local port name patterns ignored before topology graph construction. |
| switchTopologyDiscovery.keepaliveTime | string | `"30s"` | gRPC keepalive interval for controller-to-switch-agent streams. |
| switchTopologyDiscovery.maxRecvMsgSize | int | `4194304` | Maximum gRPC message size accepted from switch-agent topology snapshots, in bytes. |
| switchTopologyDiscovery.mtls.autoGenerate | bool | `true` | Generate Helm-managed pinned mTLS Secrets when they do not already exist. |
| switchTopologyDiscovery.mtls.controllerSecretName | string | `"switch-controller-mtls-controller"` | Secret mounted into the controller containing tls.crt, tls.key, and peer.crt. |
| switchTopologyDiscovery.mtls.enabled | bool | `true` | Require pinned mTLS for controller-to-switch-agent streams. |
| switchTopologyDiscovery.mtls.switchAgentSecretName | string | `"switch-controller-mtls-agent"` | Secret exported for switch agents containing tls.crt, tls.key, and peer.crt. |
| switchTopologyDiscovery.mtls.validityDays | int | `36500` | Validity period, in days, for auto-generated mTLS certificates. |
| switchTopologyDiscovery.reconnectBackoff | string | `"30s"` | Delay before retrying a disconnected switch-agent stream. |

## Node Topology Discovery and Observability

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| nodeTopologyDiscovery.initialScanDelay | string | `"1m"` | Delay before the first agent scan, allowing lldpd to learn neighbors. |
| nodeTopologyDiscovery.refreshInterval | string | `"1m"` | Interval between agent refreshes of local RDMA interfaces and LLDP neighbors. |
| nodeTopologyDiscovery.scaleOutInterfaceSelector | string | `""` | RDMA interfaces included in scale-out topology and metrics. Supports selectors such as cidr=172.17.0.0/16 or interface=eth*,!eth9. |
| nodeTopologyDiscovery.scaleUpInterfaceSelector | string | `""` | RDMA interfaces classified as scale-up and excluded from scale-out topology. Leave empty when no dedicated scale-up RDMA network exists. |
| nodeTopologyDiscovery.storageInterfaceSelector | string | `""` | RDMA interfaces classified as storage and excluded from scale-out topology. Leave empty when no dedicated storage RDMA network exists. |
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
| agent.image.pullPolicy | string | `"IfNotPresent"` | Image pull policy for the agent container. |
| agent.image.registry | string | `"ghcr.io"` | Container image registry for the agent. |
| agent.image.repository | string | `"unifabric-io/unifabric-agent"` | Container image repository for the agent. |
| agent.image.tag | string | `""` | Container image tag for the agent. Defaults to the chart appVersion when empty. |
| agent.lldp.containerSecurityContext.privileged | bool | `true` | Run the lldpd sidecar as privileged so it can access host network interfaces. |
| agent.lldp.enabled | bool | `true` | Run the lldpd sidecar used by the agent to collect LLDP neighbors. |
| agent.lldp.extraConfig | string | `""` | Additional raw lldpd configuration appended to lldpd.conf. |
| agent.lldp.image.pullPolicy | string | `"IfNotPresent"` | Image pull policy for the lldpd sidecar. |
| agent.lldp.image.registry | string | `"ghcr.io"` | Container image registry for lldpd. |
| agent.lldp.image.repository | string | `"unifabric-io/unifabric-agent"` | Container image repository for lldpd. |
| agent.lldp.image.tag | string | `""` | Container image tag for lldpd. Defaults to the chart appVersion when empty. |
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
| nvidiaTopograph.enable | bool | `false` | Deploy the NVIDIA Topograph integration rendered by this chart. |
| nvidiaTopograph.engine.name | string | `"k8s"` | Topograph engine name used by node-observer. |
| nvidiaTopograph.imagePullSecrets | list | `[]` | Image pull secrets applied to NVIDIA Topograph workloads. Defaults to global.imagePullSecrets when empty. |
| nvidiaTopograph.nodeDataBroker.affinity | object | `{}` | Affinity rules for scheduling node-data-broker pods. |
| nvidiaTopograph.nodeDataBroker.enable | bool | `true` | Deploy the node-data-broker DaemonSet used by NVIDIA Topograph. |
| nvidiaTopograph.nodeDataBroker.extraArgs | list | `[]` |  |
| nvidiaTopograph.nodeDataBroker.image.pullPolicy | string | `"IfNotPresent"` | Image pull policy for the node-data-broker container. |
| nvidiaTopograph.nodeDataBroker.image.registry | string | `"ghcr.io"` | Container image registry for node-data-broker. |
| nvidiaTopograph.nodeDataBroker.image.repository | string | `"nvidia/topograph"` | Container image repository for node-data-broker. |
| nvidiaTopograph.nodeDataBroker.image.tag | string | `"v0.5.0"` | Container image tag for node-data-broker. |
| nvidiaTopograph.nodeDataBroker.nodeSelector | object | `{"nvidia.com/gpu.present":"true"}` | Node selector for scheduling node-data-broker pods. |
| nvidiaTopograph.nodeDataBroker.podAnnotations | object | `{}` | Annotations added to node-data-broker pods. |
| nvidiaTopograph.nodeDataBroker.podLabels | object | `{}` | Extra labels added to node-data-broker pods. |
| nvidiaTopograph.nodeDataBroker.port | int | `8080` | Port the node-data-broker serves its /healthz endpoint on after applying node annotations. |
| nvidiaTopograph.nodeDataBroker.refreshInterval | string | `"5m"` | How often node-data-broker reapplies node annotations. Set to 0 to disable periodic refresh. |
| nvidiaTopograph.nodeDataBroker.resources.limits.cpu | string | `"100m"` | node-data-broker CPU limit. |
| nvidiaTopograph.nodeDataBroker.resources.limits.memory | string | `"128Mi"` | node-data-broker memory limit. |
| nvidiaTopograph.nodeDataBroker.resources.requests.cpu | string | `"100m"` | node-data-broker CPU request. |
| nvidiaTopograph.nodeDataBroker.resources.requests.memory | string | `"128Mi"` | node-data-broker memory request. |
| nvidiaTopograph.nodeDataBroker.securityContext.privileged | bool | `true` | Run node-data-broker as privileged so it can read host topology data. |
| nvidiaTopograph.nodeDataBroker.startupProbe.failureThreshold | int | `30` | Startup probe failure threshold for slow IB topology discovery. |
| nvidiaTopograph.nodeDataBroker.startupProbe.periodSeconds | int | `10` | Startup probe period in seconds for node-data-broker. |
| nvidiaTopograph.nodeDataBroker.tolerations | list | `[]` | Tolerations for scheduling node-data-broker pods. |
| nvidiaTopograph.nodeDataBroker.verbosity | int | `3` | Verbosity level passed to node-data-broker. |
| nvidiaTopograph.nodeObserver.affinity | object | `{}` | Affinity rules for scheduling node-observer pods. |
| nvidiaTopograph.nodeObserver.image.pullPolicy | string | `"IfNotPresent"` | Image pull policy for the node-observer container. |
| nvidiaTopograph.nodeObserver.image.registry | string | `"ghcr.io"` | Container image registry for node-observer. |
| nvidiaTopograph.nodeObserver.image.repository | string | `"nvidia/topograph"` | Container image repository for node-observer. |
| nvidiaTopograph.nodeObserver.image.tag | string | `"v0.5.0"` | Container image tag for node-observer. |
| nvidiaTopograph.nodeObserver.nodeSelector | object | `{}` | Node selector for scheduling node-observer pods. |
| nvidiaTopograph.nodeObserver.podAnnotations | object | `{}` | Annotations added to node-observer pods. |
| nvidiaTopograph.nodeObserver.podLabels | object | `{}` | Extra labels added to node-observer pods. |
| nvidiaTopograph.nodeObserver.podSecurityContext | object | `{}` | Pod security context for node-observer pods. |
| nvidiaTopograph.nodeObserver.replicaCount | int | `1` | Number of node-observer replicas. |
| nvidiaTopograph.nodeObserver.resources.limits.cpu | string | `"400m"` | node-observer CPU limit. |
| nvidiaTopograph.nodeObserver.resources.limits.memory | string | `"512Mi"` | node-observer memory limit. |
| nvidiaTopograph.nodeObserver.resources.requests.cpu | string | `"250m"` | node-observer CPU request. |
| nvidiaTopograph.nodeObserver.resources.requests.memory | string | `"256Mi"` | node-observer memory request. |
| nvidiaTopograph.nodeObserver.securityContext | object | `{}` | Container security context for the node-observer container. |
| nvidiaTopograph.nodeObserver.tolerations | list | `[]` | Tolerations for scheduling node-observer pods. |
| nvidiaTopograph.nodeObserver.topograph.apiServer.containerName | string | `"topograph"` | Topograph API container name watched by node-observer. |
| nvidiaTopograph.nodeObserver.topograph.apiServer.enabled | bool | `true` | Watch the Topograph API pod and trigger topology generation when it becomes ready. |
| nvidiaTopograph.nodeObserver.topograph.apiServer.podSelector | object | `{}` | Pod selector used to find the Topograph API pod. Empty uses the chart-managed API pod labels. |
| nvidiaTopograph.nodeObserver.topograph.nodeDataBroker.containerName | string | `"node-data-broker"` | node-data-broker container name watched by node-observer. |
| nvidiaTopograph.nodeObserver.topograph.nodeDataBroker.enabled | bool | `true` | Wait for node-data-broker pods before the first topology request. |
| nvidiaTopograph.nodeObserver.topograph.nodeDataBroker.podSelector | object | `{}` | Pod selector used to find node-data-broker pods. Empty uses the chart-managed broker pod labels. |
| nvidiaTopograph.nodeObserver.topograph.trigger.nodeSelector | object | `{"nvidia.com/gpu.present":"true"}` | Node selector used by node-observer to choose Nodes that trigger topology generation. |
| nvidiaTopograph.nodeObserver.verbosity | int | `3` | Verbosity level passed to node-observer. |
| nvidiaTopograph.provider.name | string | `"infiniband-k8s"` | Topograph provider name, such as infiniband-k8s or netq. |
| nvidiaTopograph.provider.params.useGpuCliqueLabel | bool | `true` | Use the GPU Operator clique label as the accelerator topology source for infiniband-k8s. |
| nvidiaTopograph.service.type | string | `"ClusterIP"` | Kubernetes Service type for the topograph HTTP API. |
| nvidiaTopograph.topograph.affinity | object | `{}` | Affinity rules for scheduling topograph API pods. |
| nvidiaTopograph.topograph.config.credentialsPath | string | `""` | Credentials file path written to topograph-config.yaml. Defaults to /etc/topograph/credentials/credentials.yaml when credentialsSecret is set. |
| nvidiaTopograph.topograph.config.credentialsSecret | string | `""` | Secret containing provider credentials for topograph. The Secret must include credentials.yaml by default. |
| nvidiaTopograph.topograph.config.requestAggregationDelay | string | `"15s"` | Delay used by topograph to aggregate topology generation requests. |
| nvidiaTopograph.topograph.env | object | `{}` | Extra environment variables added to the topograph API container. |
| nvidiaTopograph.topograph.image.pullPolicy | string | `"IfNotPresent"` | Image pull policy for the topograph API container. |
| nvidiaTopograph.topograph.image.registry | string | `"ghcr.io"` | Container image registry for the topograph API. |
| nvidiaTopograph.topograph.image.repository | string | `"nvidia/topograph"` | Container image repository for the topograph API. |
| nvidiaTopograph.topograph.image.tag | string | `"v0.5.0"` | Container image tag for the topograph API. |
| nvidiaTopograph.topograph.livenessProbe.httpGet.path | string | `"/healthz"` | HTTP path used by the topograph API liveness probe. |
| nvidiaTopograph.topograph.livenessProbe.httpGet.port | string | `"http"` | Named port used by the topograph API liveness probe. |
| nvidiaTopograph.topograph.nodeSelector | object | `{}` | Node selector for scheduling topograph API pods. |
| nvidiaTopograph.topograph.podAnnotations | object | `{}` | Annotations added to topograph API pods. |
| nvidiaTopograph.topograph.podLabels | object | `{}` | Extra labels added to topograph API pods. |
| nvidiaTopograph.topograph.podSecurityContext | object | `{}` | Pod security context for topograph API pods. |
| nvidiaTopograph.topograph.readinessProbe.httpGet.path | string | `"/healthz"` | HTTP path used by the topograph API readiness probe. |
| nvidiaTopograph.topograph.readinessProbe.httpGet.port | string | `"http"` | Named port used by the topograph API readiness probe. |
| nvidiaTopograph.topograph.replicaCount | int | `1` | Number of topograph API replicas. |
| nvidiaTopograph.topograph.resources.limits.cpu | string | `"400m"` | Topograph API CPU limit. |
| nvidiaTopograph.topograph.resources.limits.memory | string | `"512Mi"` | Topograph API memory limit. |
| nvidiaTopograph.topograph.resources.requests.cpu | string | `"250m"` | Topograph API CPU request. |
| nvidiaTopograph.topograph.resources.requests.memory | string | `"256Mi"` | Topograph API memory request. |
| nvidiaTopograph.topograph.securityContext | object | `{}` | Container security context for the topograph API container. |
| nvidiaTopograph.topograph.serviceMonitor.enabled | bool | `false` | Create a Prometheus Operator ServiceMonitor for the topograph API. |
| nvidiaTopograph.topograph.serviceMonitor.interval | string | `"15s"` | Prometheus scrape interval for the topograph API. |
| nvidiaTopograph.topograph.serviceMonitor.namespace | string | `"monitoring"` | Namespace where the topograph ServiceMonitor is created. |
| nvidiaTopograph.topograph.serviceMonitor.path | string | `"/metrics"` | HTTP path scraped by the topograph ServiceMonitor. |
| nvidiaTopograph.topograph.serviceMonitor.port | string | `"http"` | Service port name scraped by the topograph ServiceMonitor. |
| nvidiaTopograph.topograph.serviceMonitor.scheme | string | `"http"` | HTTP scheme used by the topograph ServiceMonitor. |
| nvidiaTopograph.topograph.serviceName | string | `""` | Existing topograph Service name used by node-observer. Leave empty to use the chart-managed Service. |
| nvidiaTopograph.topograph.tolerations | list | `[]` | Tolerations for scheduling topograph API pods. |
| nvidiaTopograph.topograph.verbosity | int | `3` | Verbosity level passed to the topograph API process. |
| nvidiaTopograph.topograph.volumeMounts | list | `[]` | Extra volume mounts added to the topograph API container. |
| nvidiaTopograph.topograph.volumes | list | `[]` | Extra volumes added to topograph API pods. |
