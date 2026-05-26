## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| agent.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].key | string | `"kubernetes.io/os"` |  |
| agent.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].operator | string | `"In"` |  |
| agent.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].values[0] | string | `"linux"` |  |
| agent.config.healthProbe.bindAddress | string | `":8083"` |  |
| agent.config.logLevel | string | `"info"` |  |
| agent.config.metrics.bindAddress | string | `":8082"` |  |
| agent.config.node.defaultRouteProbe | string | `"8.8.8.8:53"` |  |
| agent.config.node.name | string | `""` |  |
| agent.config.node.role | string | `"gpu"` |  |
| agent.containerSecurityContext.privileged | bool | `true` |  |
| agent.enabled | bool | `true` |  |
| agent.hostNetwork | bool | `true` |  |
| agent.hostPID | bool | `true` |  |
| agent.image.pullPolicy | string | `"IfNotPresent"` |  |
| agent.image.registry | string | `"ghcr.io"` | Container image registry for the agent. |
| agent.image.repository | string | `"unifabric-io/unifabric-agent"` | Container image repository for the agent. |
| agent.image.tag | string | `""` | Container image tag for the agent. Defaults to the chart appVersion when empty. |
| agent.lldp.containerSecurityContext.privileged | bool | `true` |  |
| agent.lldp.enabled | bool | `true` |  |
| agent.lldp.extraConfig | string | `""` |  |
| agent.lldp.image.pullPolicy | string | `"IfNotPresent"` |  |
| agent.lldp.image.registry | string | `"ghcr.io"` | Container image registry for lldpd. |
| agent.lldp.image.repository | string | `"unifabric-io/unifabric-agent"` | Container image repository for lldpd. |
| agent.lldp.image.tag | string | `""` | Container image tag for lldpd. Defaults to the chart appVersion when empty. |
| agent.lldp.interfaces | string | `""` |  |
| agent.lldp.managementIPPattern | string | `""` |  |
| agent.lldp.resources.limits.cpu | string | `"100m"` |  |
| agent.lldp.resources.limits.memory | string | `"128Mi"` |  |
| agent.lldp.resources.requests.cpu | string | `"50m"` |  |
| agent.lldp.resources.requests.memory | string | `"64Mi"` |  |
| agent.lldp.txInterval | int | `30` |  |
| agent.nodeSelector | object | `{}` |  |
| agent.podAnnotations | object | `{}` |  |
| agent.podLabels | object | `{}` |  |
| agent.podSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| agent.ports.health | int | `8083` |  |
| agent.ports.metrics | int | `8082` |  |
| agent.resources.limits.cpu | string | `"500m"` |  |
| agent.resources.limits.memory | string | `"512Mi"` |  |
| agent.resources.requests.cpu | string | `"100m"` |  |
| agent.resources.requests.memory | string | `"128Mi"` |  |
| agent.service.enabled | bool | `true` |  |
| agent.service.port | int | `8082` |  |
| agent.service.type | string | `"ClusterIP"` |  |
| agent.serviceAccount.annotations | object | `{}` |  |
| agent.serviceAccount.create | bool | `true` |  |
| agent.serviceAccount.name | string | `""` |  |
| agent.tolerations | list | `[]` |  |
| controller.affinity | object | `{}` |  |
| controller.config.healthProbe.bindAddress | string | `":8081"` |  |
| controller.config.leaderElection.enabled | bool | `true` |  |
| controller.config.leaderElection.id | string | `"unifabric-controller"` |  |
| controller.config.leaderElection.namespace | string | `""` |  |
| controller.config.logLevel | string | `"info"` |  |
| controller.config.metrics.bindAddress | string | `":8080"` |  |
| controller.config.pprof.bindAddress | string | `""` |  |
| controller.containerSecurityContext.allowPrivilegeEscalation | bool | `false` |  |
| controller.containerSecurityContext.capabilities.drop[0] | string | `"ALL"` |  |
| controller.containerSecurityContext.readOnlyRootFilesystem | bool | `true` |  |
| controller.enabled | bool | `true` |  |
| controller.image.pullPolicy | string | `"IfNotPresent"` |  |
| controller.image.registry | string | `"ghcr.io"` | Container image registry for the controller. |
| controller.image.repository | string | `"unifabric-io/unifabric-controller"` | Container image repository for the controller. |
| controller.image.tag | string | `""` | Container image tag for the controller. Defaults to the chart appVersion when empty. |
| controller.nodeSelector | object | `{}` |  |
| controller.podAnnotations | object | `{}` |  |
| controller.podLabels | object | `{}` |  |
| controller.podSecurityContext.fsGroup | int | `65532` |  |
| controller.podSecurityContext.runAsGroup | int | `65532` |  |
| controller.podSecurityContext.runAsNonRoot | bool | `true` |  |
| controller.podSecurityContext.runAsUser | int | `65532` |  |
| controller.podSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| controller.ports.health | int | `8081` |  |
| controller.ports.metrics | int | `8080` |  |
| controller.replicaCount | int | `1` |  |
| controller.resources.limits.cpu | string | `"500m"` |  |
| controller.resources.limits.memory | string | `"512Mi"` |  |
| controller.resources.requests.cpu | string | `"100m"` |  |
| controller.resources.requests.memory | string | `"128Mi"` |  |
| controller.service.enabled | bool | `true` |  |
| controller.service.port | int | `8080` |  |
| controller.service.type | string | `"ClusterIP"` |  |
| controller.serviceAccount.annotations | object | `{}` |  |
| controller.serviceAccount.create | bool | `true` |  |
| controller.serviceAccount.name | string | `""` |  |
| controller.tolerations | list | `[]` |  |
| global.imagePullSecrets | list | `[]` |  |
| global.registry | string | `""` | Override the default image registry used by chart workloads when image-specific registry is empty. |
| grafanaDashboard | object | See individual dashboard settings below. | Controls rendering of prebuilt Grafana dashboard resources. |
| grafanaDashboard.allowCrossNamespaceImport | bool | `true` | Allow GrafanaDashboard resources to be imported across namespaces by Grafana Operator. |
| grafanaDashboard.enabled | bool | `true` | Enable rendering RDMA topology and metrics dashboards. |
| grafanaDashboard.instanceSelector | object | `{}` | Selector used by Grafana Operator to choose Grafana instances that import dashboards. |
| grafanaDashboard.kind | string | `"GrafanaDashboard"` | Dashboard resource kind: ConfigMap for Grafana sidecar, or GrafanaDashboard for Grafana Operator. |
| grafanaDashboard.labels | object | `{}` | Additional labels attached to rendered dashboard resources. |
| nodeMetrics | object | See individual node metrics settings below. | Controls node-level RDMA metrics exported by the agent. |
| nodeMetrics.enabled | bool | `true` | Enable collection of RDMA device metrics by the agent. |
| nodeMetrics.path | string | `"/metrics"` | HTTP path exposed by the node metrics endpoint. |
| nodeMetrics.port | int | `8082` | Metrics port exposed by the node metrics endpoint. |
| nodeMetrics.service | object | See individual Service settings below. | Kubernetes Service settings for exposing node metrics. |
| nodeMetrics.service.port | int | `8086` | Service port exposed for node metrics scraping. |
| nodeMetrics.service.type | string | `"ClusterIP"` | Service type used for the node metrics Service. |
| nodeMetrics.serviceMonitor | object | See individual ServiceMonitor settings below. | ServiceMonitor settings used by Prometheus Operator to scrape node metrics. |
| nodeMetrics.serviceMonitor.enabled | bool | `true` | Enable creation of a ServiceMonitor for node metrics. |
| nodeMetrics.serviceMonitor.interval | string | `"15s"` | Prometheus scrape interval for node metrics. |
| nodeMetrics.serviceMonitor.labels | object | `{}` | Additional labels attached to the ServiceMonitor resource. |
| nodeMetrics.serviceMonitor.path | string | `"/metrics"` | Metrics path used by the ServiceMonitor endpoint. |
| nodeMetrics.serviceMonitor.scrapeTimeout | string | `"10s"` | Prometheus scrape timeout for node metrics. |
| nodeTopologyDiscovery | object | See individual topology discovery settings below. | Controls local node topology discovery on each agent node. |
| nodeTopologyDiscovery.initialScanDelay | string | `"1m"` | Delay before the first local RDMA and LLDP topology scan, allowing lldpd to learn neighbors. |
| nodeTopologyDiscovery.refreshInterval | string | `"1m"` | How often each agent refreshes local RDMA interface and LLDP neighbor state. |
| nodeTopologyDiscovery.scaleOutInterfaceSelector | string | `""` | Select RDMA interfaces that belong to scale-out topology. Supports values like cidr=172.17.0.0/16 or interface=eth*,!eth9. |
| nodeTopologyDiscovery.scaleUpInterfaceSelector | string | `""` | Select RDMA interfaces that belong to scale-up topology. Configure only when scale-up RDMA interfaces exist; matched interfaces are excluded from FabricNode scale-out status. |
| nodeTopologyDiscovery.storageInterfaceSelector | string | `""` | Select RDMA interfaces that belong to storage topology. Configure only when storage RDMA interfaces exist. |
| nvidiaTopograph.enable | bool | `false` | Enable the NVIDIA topograph integration rendered by this chart. |
| nvidiaTopograph.engine | object | See individual engine settings below. | Engine settings for the NVIDIA topograph integration. |
| nvidiaTopograph.engine.name | string | `"k8s"` | Topograph engine name. |
| nvidiaTopograph.imagePullSecrets | list | `[]` | Image pull secrets applied to the NVIDIA topograph workloads. |
| nvidiaTopograph.nodeDataBroker.affinity | object | `{}` |  |
| nvidiaTopograph.nodeDataBroker.command[0] | string | `"tail"` |  |
| nvidiaTopograph.nodeDataBroker.command[1] | string | `"-f"` |  |
| nvidiaTopograph.nodeDataBroker.command[2] | string | `"/dev/null"` |  |
| nvidiaTopograph.nodeDataBroker.enable | bool | `true` |  |
| nvidiaTopograph.nodeDataBroker.image.pullPolicy | string | `"IfNotPresent"` |  |
| nvidiaTopograph.nodeDataBroker.image.registry | string | `"ghcr.io"` | Container image registry for the node data broker DaemonSet. |
| nvidiaTopograph.nodeDataBroker.image.repository | string | `"nvidia/topograph/ib"` | Container image repository for the node data broker DaemonSet. |
| nvidiaTopograph.nodeDataBroker.image.tag | string | `"main"` | Container image tag for the node data broker DaemonSet. |
| nvidiaTopograph.nodeDataBroker.initContainer.enable | bool | `true` |  |
| nvidiaTopograph.nodeDataBroker.initContainer.extraArgs[0] | string | `"gpu-operator-namespace=gpu-operator"` |  |
| nvidiaTopograph.nodeDataBroker.initContainer.extraArgs[1] | string | `"device-plugin-daemonset=nvidia-device-plugin-daemonset"` |  |
| nvidiaTopograph.nodeDataBroker.initContainer.image.pullPolicy | string | `"IfNotPresent"` |  |
| nvidiaTopograph.nodeDataBroker.initContainer.image.registry | string | `"ghcr.io"` | Container image registry for the node data broker init container. |
| nvidiaTopograph.nodeDataBroker.initContainer.image.repository | string | `"nvidia/topograph"` | Container image repository for the node data broker init container. |
| nvidiaTopograph.nodeDataBroker.initContainer.image.tag | string | `"v0.3.0"` | Container image tag for the node data broker init container. |
| nvidiaTopograph.nodeDataBroker.nodeSelector | object | `{}` |  |
| nvidiaTopograph.nodeDataBroker.podAnnotations | object | `{}` |  |
| nvidiaTopograph.nodeDataBroker.podLabels | object | `{}` |  |
| nvidiaTopograph.nodeDataBroker.podSecurityContext | object | `{}` |  |
| nvidiaTopograph.nodeDataBroker.resources.limits.cpu | string | `"100m"` |  |
| nvidiaTopograph.nodeDataBroker.resources.limits.memory | string | `"128Mi"` |  |
| nvidiaTopograph.nodeDataBroker.resources.requests.cpu | string | `"100m"` |  |
| nvidiaTopograph.nodeDataBroker.resources.requests.memory | string | `"128Mi"` |  |
| nvidiaTopograph.nodeDataBroker.securityContext.privileged | bool | `true` |  |
| nvidiaTopograph.nodeDataBroker.serviceAccount.annotations | object | `{}` |  |
| nvidiaTopograph.nodeDataBroker.serviceAccount.automount | bool | `true` |  |
| nvidiaTopograph.nodeDataBroker.serviceAccount.create | bool | `true` |  |
| nvidiaTopograph.nodeDataBroker.serviceAccount.name | string | `""` |  |
| nvidiaTopograph.nodeDataBroker.tolerations | list | `[]` |  |
| nvidiaTopograph.nodeDataBroker.verbosity | int | `3` |  |
| nvidiaTopograph.nodeDataBroker.volumeMounts[0].mountPath | string | `"/sys/class"` |  |
| nvidiaTopograph.nodeDataBroker.volumeMounts[0].name | string | `"sys-class"` |  |
| nvidiaTopograph.nodeDataBroker.volumes[0].hostPath.path | string | `"/sys/class"` |  |
| nvidiaTopograph.nodeDataBroker.volumes[0].hostPath.type | string | `"Directory"` |  |
| nvidiaTopograph.nodeDataBroker.volumes[0].name | string | `"sys-class"` |  |
| nvidiaTopograph.nodeObserver.affinity | object | `{}` |  |
| nvidiaTopograph.nodeObserver.image.pullPolicy | string | `"IfNotPresent"` |  |
| nvidiaTopograph.nodeObserver.image.registry | string | `"ghcr.io"` | Container image registry for the node observer deployment. |
| nvidiaTopograph.nodeObserver.image.repository | string | `"nvidia/topograph"` | Container image repository for the node observer deployment. |
| nvidiaTopograph.nodeObserver.image.tag | string | `"v0.3.0"` | Container image tag for the node observer deployment. |
| nvidiaTopograph.nodeObserver.nodeSelector | object | `{}` |  |
| nvidiaTopograph.nodeObserver.podAnnotations | object | `{}` |  |
| nvidiaTopograph.nodeObserver.podLabels | object | `{}` |  |
| nvidiaTopograph.nodeObserver.podSecurityContext | object | `{}` |  |
| nvidiaTopograph.nodeObserver.replicaCount | int | `1` |  |
| nvidiaTopograph.nodeObserver.resources.limits.cpu | string | `"400m"` |  |
| nvidiaTopograph.nodeObserver.resources.limits.memory | string | `"512Mi"` |  |
| nvidiaTopograph.nodeObserver.resources.requests.cpu | string | `"250m"` |  |
| nvidiaTopograph.nodeObserver.resources.requests.memory | string | `"256Mi"` |  |
| nvidiaTopograph.nodeObserver.securityContext | object | `{}` |  |
| nvidiaTopograph.nodeObserver.serviceAccount.annotations | object | `{}` |  |
| nvidiaTopograph.nodeObserver.serviceAccount.automount | bool | `true` |  |
| nvidiaTopograph.nodeObserver.serviceAccount.create | bool | `true` |  |
| nvidiaTopograph.nodeObserver.serviceAccount.name | string | `""` |  |
| nvidiaTopograph.nodeObserver.tolerations | list | `[]` |  |
| nvidiaTopograph.nodeObserver.trigger | object | `{}` |  |
| nvidiaTopograph.nodeObserver.verbosity | int | `3` |  |
| nvidiaTopograph.nodeObserver.wait.image.pullPolicy | string | `"IfNotPresent"` |  |
| nvidiaTopograph.nodeObserver.wait.image.registry | string | `"docker.io"` | Container image registry for the node observer wait init container. |
| nvidiaTopograph.nodeObserver.wait.image.repository | string | `"curlimages/curl"` | Container image repository for the node observer wait init container. |
| nvidiaTopograph.nodeObserver.wait.image.tag | string | `"8.13.0"` | Container image tag for the node observer wait init container. |
| nvidiaTopograph.provider | object | See individual provider settings below. | Provider settings for the NVIDIA topograph integration. |
| nvidiaTopograph.provider.name | string | `"infiniband-k8s"` | Topograph provider name. |
| nvidiaTopograph.service | object | See individual Service settings below. | Service settings shared by the NVIDIA topograph components. |
| nvidiaTopograph.service.port | int | `49021` | Service port used by the topograph HTTP endpoint. |
| nvidiaTopograph.service.type | string | `"ClusterIP"` | Service type used for the topograph Service. |
| nvidiaTopograph.topograph.affinity | object | `{}` |  |
| nvidiaTopograph.topograph.config.credentialsPath | string | `""` | Optional credentials path written to topograph-config.yaml. When credentialsSecret is set and this is empty, the chart uses /etc/topograph/credentials/credentials.yaml. |
| nvidiaTopograph.topograph.config.credentialsSecret | string | `""` | Optional Secret containing provider API credentials for topograph. By default the Secret must include a credentials.yaml key. |
| nvidiaTopograph.topograph.config.requestAggregationDelay | string | `"15s"` |  |
| nvidiaTopograph.topograph.env | object | `{}` |  |
| nvidiaTopograph.topograph.image.pullPolicy | string | `"IfNotPresent"` |  |
| nvidiaTopograph.topograph.image.registry | string | `"ghcr.io"` | Container image registry for the topograph deployment. |
| nvidiaTopograph.topograph.image.repository | string | `"nvidia/topograph"` | Container image repository for the topograph deployment. |
| nvidiaTopograph.topograph.image.tag | string | `"v0.3.0"` | Container image tag for the topograph deployment. |
| nvidiaTopograph.topograph.ingress.annotations | object | `{}` |  |
| nvidiaTopograph.topograph.ingress.className | string | `""` |  |
| nvidiaTopograph.topograph.ingress.enabled | bool | `false` |  |
| nvidiaTopograph.topograph.ingress.hosts[0].host | string | `"chart-example.local"` |  |
| nvidiaTopograph.topograph.ingress.hosts[0].paths[0].path | string | `"/"` |  |
| nvidiaTopograph.topograph.ingress.hosts[0].paths[0].pathType | string | `"ImplementationSpecific"` |  |
| nvidiaTopograph.topograph.ingress.tls | list | `[]` |  |
| nvidiaTopograph.topograph.livenessProbe.httpGet.path | string | `"/healthz"` |  |
| nvidiaTopograph.topograph.livenessProbe.httpGet.port | string | `"http"` |  |
| nvidiaTopograph.topograph.nodeSelector | object | `{}` |  |
| nvidiaTopograph.topograph.podAnnotations | object | `{}` |  |
| nvidiaTopograph.topograph.podLabels | object | `{}` |  |
| nvidiaTopograph.topograph.podSecurityContext | object | `{}` |  |
| nvidiaTopograph.topograph.readinessProbe.httpGet.path | string | `"/healthz"` |  |
| nvidiaTopograph.topograph.readinessProbe.httpGet.port | string | `"http"` |  |
| nvidiaTopograph.topograph.replicaCount | int | `1` |  |
| nvidiaTopograph.topograph.resources.limits.cpu | string | `"400m"` |  |
| nvidiaTopograph.topograph.resources.limits.memory | string | `"512Mi"` |  |
| nvidiaTopograph.topograph.resources.requests.cpu | string | `"250m"` |  |
| nvidiaTopograph.topograph.resources.requests.memory | string | `"256Mi"` |  |
| nvidiaTopograph.topograph.securityContext | object | `{}` |  |
| nvidiaTopograph.topograph.serviceAccount.annotations | object | `{}` |  |
| nvidiaTopograph.topograph.serviceAccount.automount | bool | `true` |  |
| nvidiaTopograph.topograph.serviceAccount.create | bool | `true` |  |
| nvidiaTopograph.topograph.serviceAccount.name | string | `""` |  |
| nvidiaTopograph.topograph.serviceMonitor.enabled | bool | `false` |  |
| nvidiaTopograph.topograph.serviceMonitor.interval | string | `"15s"` |  |
| nvidiaTopograph.topograph.serviceMonitor.namespace | string | `"monitoring"` |  |
| nvidiaTopograph.topograph.serviceMonitor.path | string | `"/metrics"` |  |
| nvidiaTopograph.topograph.serviceMonitor.port | string | `"http"` |  |
| nvidiaTopograph.topograph.serviceMonitor.scheme | string | `"http"` |  |
| nvidiaTopograph.topograph.serviceName | string | `""` |  |
| nvidiaTopograph.topograph.tolerations | list | `[]` |  |
| nvidiaTopograph.topograph.verbosity | int | `3` |  |
| nvidiaTopograph.topograph.volumeMounts | list | `[]` |  |
| nvidiaTopograph.topograph.volumes | list | `[]` |  |
| scaleOutDiscovery | object | See individual discovery settings below. | Controls controller-side scale-out discovery derived from FabricNode topology. |
| scaleOutDiscovery.leafGroups | object | See individual leaf group settings below. | Settings for ScaleOutLeafGroup discovery and Node leaf label reconciliation. |
| scaleOutDiscovery.leafGroups.enabled | bool | `true` | Enable ScaleOutLeafGroup discovery and Node leaf label updates. |
| scaleOutDiscovery.switches | object | See individual switch discovery settings below. | Settings for switch-driven scale-out discovery. |
| scaleOutDiscovery.switches.defaultGrpcPort | int | `8090` | Default gRPC port used when a Switch resource does not set spec.grpcPort. |
| scaleOutDiscovery.switches.dialTimeout | string | `"5s"` | Controller gRPC dial timeout for one switch-agent connection attempt. |
| scaleOutDiscovery.switches.enabled | bool | `false` | Enable switch-driven scale-out discovery and make it the active scale-out path in the controller. |
| scaleOutDiscovery.switches.groupNaming.hashLength | int | `7` | Hash length used when labelValueFormat=hash. |
| scaleOutDiscovery.switches.groupNaming.labelValueFormat | string | `"hash"` | Node label value format for discovered topology groups: name or hash. |
| scaleOutDiscovery.switches.ignoreSwitchPorts | list | `["mgmt*","Management*","oob*"]` | Controller-side local switch ports ignored during topology graph construction. |
| scaleOutDiscovery.switches.keepaliveTime | string | `"30s"` | gRPC keepalive interval for controller-to-switch subscriptions. |
| scaleOutDiscovery.switches.maxRecvMsgSize | int | `4194304` | Maximum gRPC message size accepted from a switch-agent snapshot stream. |
| scaleOutDiscovery.switches.mtls.autoGenerate | bool | `true` | Auto-generate pinned mTLS materials as Helm-managed Secrets. |
| scaleOutDiscovery.switches.mtls.controllerSecretName | string | `"switch-controller-mtls-controller"` | Secret name mounted into the controller with tls.crt, tls.key, and peer.crt. |
| scaleOutDiscovery.switches.mtls.enabled | bool | `true` | Require pinned mTLS for controller-to-switch subscriptions. |
| scaleOutDiscovery.switches.mtls.switchAgentSecretName | string | `"switch-controller-mtls-agent"` | Secret name mounted into switch agents with tls.crt, tls.key, and peer.crt. |
| scaleOutDiscovery.switches.mtls.validityDays | int | `36500` | Validity period in days for auto-generated pinned mTLS materials. |
| scaleOutDiscovery.switches.reconnectBackoff | string | `"30s"` | Backoff between switch-agent reconnect attempts. |
| topologyLabels | object | See individual label settings below. | Label keys written back to Kubernetes Nodes for discovered topology dimensions. |
| topologyLabels.scaleOutCore | string | `"unifabric.io/scale-out-core"` | Label key used to mark the core-level scale-out topology group of a node. |
| topologyLabels.scaleOutLeaf | string | `"unifabric.io/scale-out-leaf"` | Label key used to mark the leaf-level scale-out topology group of a node. |
| topologyLabels.scaleOutSpine | string | `"unifabric.io/scale-out-spine"` | Label key used to mark the spine-level scale-out topology group of a node. |
| topologyLabels.scaleUp | string | `"unifabric.io/scale-up"` | Label key used to mark nodes that participate in scale-up topology. |
