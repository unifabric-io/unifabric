export interface TopologyPanelOptions {
  showLabels: boolean;
}

export interface TopologyDomain {
  name: string;
  tier: number;
  parent?: string;
  switchMember?: string[];
}

export interface TopologyNodeGroup {
  nodes: string[];
  switchDomainPath: string[];
}

export interface TopologyResource {
  metadata: { name: string };
  status?: {
    domains?: TopologyDomain[];
    nodes?: TopologyNodeGroup[];
  };
}

export interface FabricNodeResource {
  metadata: { name: string };
}

export interface ResourceList<T> {
  items?: T[];
}
