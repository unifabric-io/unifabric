import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

/** Fixed Topology CR names defined by the Unifabric Topology API (see docs/design/topology-crd.md). */
export type TopologyName = 'scaleout' | 'scaleup' | 'storage';

/** The panel always fetches and combines every Topology CR into a single graph, for one cluster. */
export interface MyQuery extends DataQuery {
  /** Cluster name returned by GET /clusters. Defaults to the first available cluster when unset. */
  cluster?: string;
}

/** Mirrors pkg/api/v1beta1.TopologyDomain (Topology CRD status.domains[]). */
export interface TopologyDomain {
  name: string;
  tier: number;
  parent?: string;
  switchMember?: string[];
}

/** Mirrors pkg/api/v1beta1.TopologyNodeGroup (Topology CRD status.nodes[]). */
export interface TopologyNodeGroup {
  name: string;
  nodes: string[];
  switchDomainPath: string[];
}

/** Mirrors pkg/api/v1beta1.TopologyStatus, e.g. `kubectl get topo <name> -o json` `.status`. */
export interface TopologyStatusResponse {
  domains?: TopologyDomain[];
  nodes?: TopologyNodeGroup[];
}

/** Mirrors pkg/api/v1beta1.Topology as it appears inside a TopologyList. */
export interface TopologyListItem {
  metadata: { name: string };
  status: TopologyStatusResponse;
}

/** Mirrors pkg/api/v1beta1.TopologyList, i.e. what `client.List` returns for Topology. */
export interface TopologyList {
  items: TopologyListItem[];
}

/** One entry from GET /clusters. Open source always returns exactly one, named "default". */
export interface TopologyCluster {
  name: string;
}

/** Mirrors k8s.io/apimachinery/pkg/apis/meta/v1.Condition, used by both Switch and FabricNode status.conditions[]. */
export interface ResourceCondition {
  type: string;
  status: 'True' | 'False' | 'Unknown';
  reason?: string;
  message?: string;
  lastTransitionTime?: string;
}

/** Mirrors pkg/api/v1beta1.ObjectMeta fields actually used by resource hover cards. */
export interface ResourceMetadata {
  name: string;
  creationTimestamp?: string;
  labels?: Record<string, string>;
}

/** Mirrors pkg/api/v1beta1.SwitchSpec. */
export interface SwitchSpec {
  mgmtIP?: string;
  role?: 'ScaleOut' | 'ScaleUp' | 'Storage';
  grpcPort?: number;
}

/** Mirrors pkg/api/v1beta1.SwitchNeighbor (status.lldpNeighbors[]). */
export interface SwitchNeighbor {
  remoteSystemType?: 'KubernetesNode' | 'Switch';
  remoteSystemName: string;
  linkCount?: number;
}

/** Mirrors pkg/api/v1beta1.SwitchStatus. */
export interface SwitchStatus {
  hostname?: string;
  healthy?: boolean;
  conditions?: ResourceCondition[];
  lldpNeighborCount?: number;
  lldpNeighbors?: SwitchNeighbor[];
}

/** Mirrors pkg/api/v1beta1.Switch, as returned by GET .../apis/unifabric.io/v1beta1/switch/{name}. */
export interface SwitchResource {
  metadata: ResourceMetadata;
  spec?: SwitchSpec;
  status?: SwitchStatus;
}

/** Mirrors pkg/api/v1beta1.NicInfo (status.scaleOutNics[] / status.storageNics[] on FabricNode). */
export interface NicInfo {
  name: string;
  rdmaDeviceName?: string;
  rdma?: boolean;
  ipv4?: string;
  ipv6?: string;
  state?: string;
}

/** Mirrors pkg/api/v1beta1.RdmaPod (status.rdmaPods[] on FabricNode). */
export interface RdmaPod {
  namespace: string;
  name: string;
}

/** Mirrors pkg/api/v1beta1.FabricNodeStatus. */
export interface FabricNodeStatus {
  conditions?: ResourceCondition[];
  totalNics?: number;
  scaleOutNics?: NicInfo[];
  storageNics?: NicInfo[];
  rdmaPods?: RdmaPod[];
  nodeRole?: 'GPU' | 'Storage';
  nodeIP?: string;
  topologies?: string[];
}

/** Mirrors pkg/api/v1beta1.FabricNode, as returned by GET .../apis/unifabric.io/v1beta1/fabricnode/{name}. */
export interface FabricNodeResource {
  metadata: ResourceMetadata;
  status?: FabricNodeStatus;
}

/** Mirrors pkg/api/v1beta1.FabricNodeList. */
export interface FabricNodeList {
  items: FabricNodeResource[];
}

/**
 * These are options configured for each DataSource instance.
 * HTTP settings are managed by Grafana's DataSourceHttpSettings component.
 */
export type MyDataSourceOptions = DataSourceJsonData;

/**
 * Values that are encrypted and only sent to the backend, never back to the frontend
 */
export type MySecureJsonData = Record<string, never>;
