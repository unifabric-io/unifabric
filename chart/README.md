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
| grafanaDashboard | object | `{"allowCrossNamespaceImport":true,"enabled":true,"instanceSelector":{},"kind":"GrafanaDashboard","labels":{}}` | Controls rendering of prebuilt Grafana dashboard resources. |
| grafanaDashboard.allowCrossNamespaceImport | bool | `true` | Allow GrafanaDashboard resources to be imported across namespaces by Grafana Operator. |
| grafanaDashboard.enabled | bool | `true` | Enable rendering RDMA topology and metrics dashboards. |
| grafanaDashboard.instanceSelector | object | `{}` | Selector used by Grafana Operator to choose Grafana instances that import dashboards. |
| grafanaDashboard.kind | string | `"GrafanaDashboard"` | Dashboard resource kind: ConfigMap for Grafana sidecar, or GrafanaDashboard for Grafana Operator. |
| grafanaDashboard.labels | object | `{}` | Additional labels attached to rendered dashboard resources. |
| nodeMetrics | object | `{"enabled":true,"path":"/metrics","port":8082,"service":{"port":8086,"type":"ClusterIP"},"serviceMonitor":{"enabled":true,"interval":"15s","labels":{},"path":"/metrics","scrapeTimeout":"10s"}}` | Controls node-level RDMA metrics exported by the agent. |
| nodeMetrics.enabled | bool | `true` | Enable collection of RDMA device metrics by the agent. |
| nodeMetrics.path | string | `"/metrics"` | HTTP path exposed by the node metrics endpoint. |
| nodeMetrics.port | int | `8082` | Metrics port exposed by the node metrics endpoint. |
| nodeMetrics.service | object | `{"port":8086,"type":"ClusterIP"}` | Kubernetes Service settings for exposing node metrics. |
| nodeMetrics.service.port | int | `8086` | Service port exposed for node metrics scraping. |
| nodeMetrics.service.type | string | `"ClusterIP"` | Service type used for the node metrics Service. |
| nodeMetrics.serviceMonitor | object | `{"enabled":true,"interval":"15s","labels":{},"path":"/metrics","scrapeTimeout":"10s"}` | ServiceMonitor settings used by Prometheus Operator to scrape node metrics. |
| nodeMetrics.serviceMonitor.enabled | bool | `true` | Enable creation of a ServiceMonitor for node metrics. |
| nodeMetrics.serviceMonitor.interval | string | `"15s"` | Prometheus scrape interval for node metrics. |
| nodeMetrics.serviceMonitor.labels | object | `{}` | Additional labels attached to the ServiceMonitor resource. |
| nodeMetrics.serviceMonitor.path | string | `"/metrics"` | Metrics path used by the ServiceMonitor endpoint. |
| nodeMetrics.serviceMonitor.scrapeTimeout | string | `"10s"` | Prometheus scrape timeout for node metrics. |
| nodeTopologyDiscovery | object | `{"initialScanDelay":"1m","refreshInterval":"1m","scaleOutInterfaceSelector":"","scaleUpInterfaceSelector":"","storageInterfaceSelector":""}` | Controls local node topology discovery on each agent node. |
| nodeTopologyDiscovery.initialScanDelay | string | `"1m"` | Delay before the first local RDMA and LLDP topology scan, allowing lldpd to learn neighbors. |
| nodeTopologyDiscovery.refreshInterval | string | `"1m"` | How often each agent refreshes local RDMA interface and LLDP neighbor state. |
| nodeTopologyDiscovery.scaleOutInterfaceSelector | string | `""` | Select RDMA interfaces that belong to scale-out topology. Supports values like cidr=172.17.0.0/16 or interface=eth*,!eth9. |
| nodeTopologyDiscovery.scaleUpInterfaceSelector | string | `""` | Select RDMA interfaces that belong to scale-up topology. Configure only when scale-up RDMA interfaces exist; matched interfaces are excluded from FabricNode scale-out status. |
| nodeTopologyDiscovery.storageInterfaceSelector | string | `""` | Select RDMA interfaces that belong to storage topology. Configure only when storage RDMA interfaces exist. |
| nvidiaTopograph.enable | bool | `false` | Enable the NVIDIA topograph integration rendered by this chart. |
| nvidiaTopograph.engine | object | `{"name":"k8s"}` | Engine settings for the NVIDIA topograph integration. |
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
| nvidiaTopograph.provider | object | `{"name":"infiniband-k8s"}` | Provider settings for the NVIDIA topograph integration. |
| nvidiaTopograph.provider.name | string | `"infiniband-k8s"` | Topograph provider name. |
| nvidiaTopograph.service | object | `{"port":49021,"type":"ClusterIP"}` | Service settings shared by the NVIDIA topograph components. |
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
| scaleOutDiscovery | object | `{"leafGroups":{"enabled":true}}` | Controls controller-side scale-out discovery derived from FabricNode topology. |
| scaleOutDiscovery.leafGroups | object | `{"enabled":true}` | Settings for ScaleOutLeafGroup discovery and Node leaf label reconciliation. |
| scaleOutDiscovery.leafGroups.enabled | bool | `true` | Enable ScaleOutLeafGroup discovery and Node leaf label updates. |
| sflow | object | See child values. | Controls switch sFlow ingestion and ClickHouse export. |
| sflow.affinity | object | `{}` | Affinity rules applied to the sFlow collector Pod. |
| sflow.clickhouse | object | See child values. | ClickHouse settings used by the sFlow collector. |
| sflow.clickhouse.address | string | `""` | ClickHouse native protocol address used by the sFlow collector. |
| sflow.clickhouse.database | string | `"default"` | ClickHouse database containing the flows_raw table. |
| sflow.clickhouse.managed | object | See child values. | Managed ClickHouse deployment settings for sFlow flow records. |
| sflow.clickhouse.managed.affinity | object | `{}` | Affinity rules applied to the managed ClickHouse Pod. |
| sflow.clickhouse.managed.containerSecurityContext | object | `{}` | Container security context applied to the managed ClickHouse container. |
| sflow.clickhouse.managed.enabled | bool | `false` | Deploy a single-node ClickHouse instance for sFlow flow records. |
| sflow.clickhouse.managed.image | object | See child values. | Container image settings for the managed ClickHouse server. |
| sflow.clickhouse.managed.image.pullPolicy | string | `"IfNotPresent"` | Container image pull policy for the managed ClickHouse server. |
| sflow.clickhouse.managed.image.registry | string | `"docker.io"` | Container image registry for the managed ClickHouse server. |
| sflow.clickhouse.managed.image.repository | string | `"clickhouse/clickhouse-server"` | Container image repository for the managed ClickHouse server. |
| sflow.clickhouse.managed.image.tag | string | `"26.3"` | Container image tag for the managed ClickHouse server. |
| sflow.clickhouse.managed.nodeSelector | object | `{}` | Node selector applied to the managed ClickHouse Pod. |
| sflow.clickhouse.managed.persistence | object | See child values. | Persistence settings for the managed ClickHouse server. |
| sflow.clickhouse.managed.persistence.accessModes | list | `["ReadWriteOnce"]` | Access modes for the managed ClickHouse PVC. |
| sflow.clickhouse.managed.persistence.enabled | bool | `true` | Persist managed ClickHouse data. When false, data uses emptyDir. |
| sflow.clickhouse.managed.persistence.hostPath | object | See child values. | HostPath settings used when persistence.type=hostPath. |
| sflow.clickhouse.managed.persistence.hostPath.path | string | `""` | Host path used when persistence.type=hostPath. The ClickHouse Pod must stay on a node that has this path. |
| sflow.clickhouse.managed.persistence.hostPath.type | string | `"DirectoryOrCreate"` | Kubernetes hostPath type used when persistence.type=hostPath. |
| sflow.clickhouse.managed.persistence.size | string | `"20Gi"` | Requested size for managed ClickHouse data. |
| sflow.clickhouse.managed.persistence.storageClassName | string | `""` | StorageClass for the managed ClickHouse PVC. Empty uses the cluster default. |
| sflow.clickhouse.managed.persistence.type | string | `"pvc"` | Persistence backend for managed ClickHouse data when enabled. Supported values: pvc, hostPath. |
| sflow.clickhouse.managed.podAnnotations | object | `{}` | Additional annotations added to the managed ClickHouse Pod. |
| sflow.clickhouse.managed.podLabels | object | `{}` | Additional labels added to the managed ClickHouse Pod. |
| sflow.clickhouse.managed.podSecurityContext | object | `{}` | Pod security context applied to the managed ClickHouse Pod. |
| sflow.clickhouse.managed.resources | object | See child values. | Resource requests and limits for the managed ClickHouse container. |
| sflow.clickhouse.managed.resources.limits | object | See child values. | Resource limits for the managed ClickHouse container. |
| sflow.clickhouse.managed.resources.limits.cpu | string | `"2"` | CPU limit for the managed ClickHouse container. |
| sflow.clickhouse.managed.resources.limits.memory | string | `"4Gi"` | Memory limit for the managed ClickHouse container. |
| sflow.clickhouse.managed.resources.requests | object | See child values. | Resource requests for the managed ClickHouse container. |
| sflow.clickhouse.managed.resources.requests.cpu | string | `"500m"` | CPU requested by the managed ClickHouse container. |
| sflow.clickhouse.managed.resources.requests.memory | string | `"1Gi"` | Memory requested by the managed ClickHouse container. |
| sflow.clickhouse.managed.service | object | See child values. | Service settings for the managed ClickHouse server. |
| sflow.clickhouse.managed.service.httpPort | int | `8123` | HTTP port exposed by the managed ClickHouse Service. |
| sflow.clickhouse.managed.service.nativePort | int | `9000` | Native protocol port exposed by the managed ClickHouse Service. |
| sflow.clickhouse.managed.tolerations | list | `[]` | Tolerations applied to the managed ClickHouse Pod. |
| sflow.clickhouse.password | string | `""` | Inline ClickHouse password. Prefer passwordSecret for production installs. |
| sflow.clickhouse.passwordSecret | object | See child values. | Existing Secret reference for the ClickHouse password. |
| sflow.clickhouse.passwordSecret.key | string | `""` | Secret key containing the ClickHouse password. Defaults to "password" when a Secret name is provided. |
| sflow.clickhouse.passwordSecret.name | string | `""` | Secret name containing the ClickHouse password. |
| sflow.clickhouse.schema | object | See child values. | Schema reconciliation settings for the ClickHouse flows_raw table. |
| sflow.clickhouse.schema.lock | object | See child values. | Kubernetes Lease settings used to serialize schema migrations. |
| sflow.clickhouse.schema.lock.leaseDuration | string | `"10s"` | Duration before another sFlow replica can take over a stale schema migration Lease. |
| sflow.clickhouse.schema.lock.name | string | `"unifabric-sflow-clickhouse-schema"` | Kubernetes Lease name used to serialize ClickHouse schema migrations across sFlow replicas. |
| sflow.clickhouse.schema.lock.namespace | string | `""` | Kubernetes namespace for the schema migration Lease. Defaults to the chart release namespace in rendered config. |
| sflow.clickhouse.schema.lock.retryInterval | string | `"2s"` | Retry interval used while waiting for the schema migration Lease. |
| sflow.clickhouse.schema.retentionDays | int | `3` | Retention window in days for the flows_raw table. Must be at least 1. The collector reconciles this table TTL during startup. |
| sflow.clickhouse.table | string | `"flows_raw"` | ClickHouse table that stores enriched sFlow rows. |
| sflow.clickhouse.username | string | `"default"` | ClickHouse username used by the sFlow collector. |
| sflow.config | object | See child values. | Runtime configuration for the sFlow collector. |
| sflow.config.logLevel | string | `"info"` | Log level used by the sFlow collector. |
| sflow.containerSecurityContext | object | See child values. | Container security context applied to the sFlow collector container. |
| sflow.containerSecurityContext.allowPrivilegeEscalation | bool | `false` | Allow privilege escalation in the sFlow collector container. |
| sflow.containerSecurityContext.capabilities | object | See child values. | Linux capabilities settings for the sFlow collector container. |
| sflow.containerSecurityContext.capabilities.drop | list | `["ALL"]` | Linux capabilities dropped from the sFlow collector container. |
| sflow.containerSecurityContext.readOnlyRootFilesystem | bool | `true` | Mount the sFlow collector root filesystem as read-only. |
| sflow.enabled | bool | `false` | Enable the sFlow collector deployment and UDP service. |
| sflow.healthProbe | object | See child values. | Health probe settings for the sFlow collector. |
| sflow.healthProbe.bindAddress | string | `":8085"` | Health probe bind address used by the sFlow collector. |
| sflow.healthProbe.port | int | `8085` | Health probe container port exposed by the sFlow collector. |
| sflow.image | object | See child values. | Container image settings for the sFlow collector. |
| sflow.image.pullPolicy | string | `"IfNotPresent"` | Container image pull policy for the sFlow collector. |
| sflow.image.registry | string | `"ghcr.io"` | Container image registry for the sFlow collector. |
| sflow.image.repository | string | `"unifabric-io/unifabric-sflow"` | Container image repository for the sFlow collector. |
| sflow.image.tag | string | `""` | Container image tag for the sFlow collector. Defaults to the chart appVersion when empty. |
| sflow.listen | object | See child values. | UDP listener settings for the sFlow collector. |
| sflow.listen.bindAddress | string | `":6343"` | UDP bind address used by the sFlow collector container. |
| sflow.listen.port | int | `6343` | UDP container port used by the sFlow collector. |
| sflow.metrics | object | See child values. | Prometheus metrics settings for the sFlow collector. |
| sflow.metrics.bindAddress | string | `":8084"` | Metrics bind address used by the sFlow collector. |
| sflow.metrics.path | string | `"/metrics"` | Metrics path exposed by the sFlow collector. |
| sflow.metrics.port | int | `8084` | Metrics container port exposed by the sFlow collector. |
| sflow.nodeSelector | object | `{}` | Node selector applied to the sFlow collector Pod. |
| sflow.podAnnotations | object | `{}` | Additional annotations added to the sFlow collector Pod. |
| sflow.podLabels | object | `{}` | Additional labels added to the sFlow collector Pod. |
| sflow.podSecurityContext | object | See child values. | Pod security context applied to the sFlow collector Pod. |
| sflow.podSecurityContext.fsGroup | int | `65532` | Filesystem group ID used by the sFlow collector Pod. |
| sflow.podSecurityContext.runAsGroup | int | `65532` | Group ID used by the sFlow collector container. |
| sflow.podSecurityContext.runAsNonRoot | bool | `true` | Run the sFlow collector as a non-root user. |
| sflow.podSecurityContext.runAsUser | int | `65532` | User ID used by the sFlow collector container. |
| sflow.podSecurityContext.seccompProfile | object | See child values. | Seccomp profile applied to the sFlow collector Pod. |
| sflow.podSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` | Seccomp profile type applied to the sFlow collector Pod. |
| sflow.replicaCount | int | `1` | Number of sFlow collector replicas to run. |
| sflow.resources | object | See child values. | Resource requests and limits for the sFlow collector container. |
| sflow.resources.limits | object | See child values. | Resource limits for the sFlow collector container. |
| sflow.resources.limits.cpu | string | `"500m"` | CPU limit for the sFlow collector container. |
| sflow.resources.limits.memory | string | `"512Mi"` | Memory limit for the sFlow collector container. |
| sflow.resources.requests | object | See child values. | Resource requests for the sFlow collector container. |
| sflow.resources.requests.cpu | string | `"100m"` | CPU requested by the sFlow collector container. |
| sflow.resources.requests.memory | string | `"128Mi"` | Memory requested by the sFlow collector container. |
| sflow.service | object | See child values. | UDP Service settings for switch sFlow exporters. |
| sflow.service.annotations | object | `{}` | Additional annotations added to the sFlow UDP Service. |
| sflow.service.enabled | bool | `true` | Enable the UDP Service for switch sFlow exporters. |
| sflow.service.nodePort | int | `0` | Optional UDP nodePort used by switch sFlow exporters. Set to 0 to let Kubernetes allocate one. |
| sflow.service.port | int | `6343` | UDP Service port used by switch sFlow exporters. |
| sflow.service.type | string | `"NodePort"` | Service type used for switch sFlow exporters. NodePort is the default because sFlow datagrams usually arrive from switches outside the cluster. |
| sflow.serviceAccount | object | See child values. | ServiceAccount settings for the sFlow collector. |
| sflow.serviceAccount.annotations | object | `{}` | Additional annotations added to the sFlow collector ServiceAccount. |
| sflow.serviceAccount.create | bool | `true` | Create a dedicated ServiceAccount for the sFlow collector. |
| sflow.serviceAccount.name | string | `""` | ServiceAccount name for the sFlow collector. Defaults to the release-derived name when empty. |
| sflow.tolerations | list | `[]` | Tolerations applied to the sFlow collector Pod. |
| sflow.writer | object | See child values. | ClickHouse writer batching and queue settings for sFlow records. |
| sflow.writer.batchSize | int | `2000` | Maximum sFlow records per ClickHouse write batch. |
| sflow.writer.flushInterval | string | `"2s"` | Maximum delay before flushing a non-empty write batch. |
| sflow.writer.queueSize | int | `65536` | Maximum records buffered before overload drops begin. |
| topologyLabels | object | `{"scaleOutCore":"unifabric.io/scale-out-core","scaleOutLeaf":"unifabric.io/scale-out-leaf","scaleOutSpine":"unifabric.io/scale-out-spine","scaleUp":"unifabric.io/scale-up"}` | Label keys written back to Kubernetes Nodes for discovered topology dimensions. |
| topologyLabels.scaleOutCore | string | `"unifabric.io/scale-out-core"` | Label key used to mark the core-level scale-out topology group of a node. |
| topologyLabels.scaleOutLeaf | string | `"unifabric.io/scale-out-leaf"` | Label key used to mark the leaf-level scale-out topology group of a node. |
| topologyLabels.scaleOutSpine | string | `"unifabric.io/scale-out-spine"` | Label key used to mark the spine-level scale-out topology group of a node. |
| topologyLabels.scaleUp | string | `"unifabric.io/scale-up"` | Label key used to mark nodes that participate in scale-up topology. |
