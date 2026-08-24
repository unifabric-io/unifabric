import React, { useEffect, useMemo, useRef, useState } from 'react';
import { colorManipulator, DataFrame, GrafanaTheme2, PanelProps } from '@grafana/data';
import { FabricNodeResource, ResourceList, TopologyPanelOptions, TopologyResource } from 'types';
import { css, cx } from '@emotion/css';
import { Popover, useStyles2, useTheme2 } from '@grafana/ui';
import { getDataSourceSrv, isFetchError, PanelDataErrorView } from '@grafana/runtime';
import ReactFlow, {
  Background,
  Controls,
  Handle,
  Position,
  useNodesState,
  type Edge,
  type Node,
  type NodeProps,
  type NodeMouseHandler,
} from 'reactflow';
import { Network, Server } from 'lucide-react';
import 'reactflow/dist/style.css';
import { hoverCardBackground, ResourceHoverCard, resourceKey, isEntryFresh, type HoverTarget, type ResourceEntry } from './ResourceHoverCard';

interface Props extends PanelProps<TopologyPanelOptions> {}

interface TopologyNodeDatum {
  id: string;
  title: string;
  detail: string;
  tier: number;
  deviceType: string;
  nodeType: string;
  topology: string;
  members: string;
  /** Resolved cluster this node's data came from (see datasource's topologiesToDataFrames). */
  cluster: string;
  /** Parent domain name, only meaningful for nodeType === 'domain'. */
  parent?: string;
  /** True when the FabricNode exists but is absent from every Topology status.nodes group. */
  detached?: boolean;
}

interface TopologyEdgeDatum {
  id: string;
  source: string;
  target: string;
  kind: string;
}

const EMPTY_NODE_DATA: TopologyNodeDatum[] = [];
const EMPTY_EDGE_DATA: TopologyEdgeDatum[] = [];

const getStyles = (theme: GrafanaTheme2) => {
  return {
    wrapper: css`
      position: relative;
      overflow: hidden;

      .react-flow__attribution {
        display: none;
      }

      .react-flow__controls {
        box-shadow: ${theme.shadows.z1};
        border-radius: 6px;
        overflow: hidden;
      }

      .react-flow__controls-button {
        background: ${theme.colors.background.secondary};
        border-bottom: 1px solid ${theme.colors.border.weak};
        fill: ${theme.colors.text.primary};

        svg {
          fill: ${theme.colors.text.primary};
        }

        &:hover {
          background: ${theme.colors.action.hover};
        }

        &:disabled svg {
          fill-opacity: 0.4;
        }
      }

      /* switchGroup is deliberately selectable:false, so it misses react-flow's own
         .react-flow__node.selectable:focus{outline:none} reset and would otherwise show the
         browser's native focus outline on click/tab, unlike every other node type. */
      .react-flow__node-switchGroup:focus,
      .react-flow__node-switchGroup:focus-visible {
        outline: none;
      }

      .topology-color-control {
        position: absolute;
        right: 15px;
        bottom: 15px;
        z-index: 5;
      }

      .topology-color-menu-button {
        display: flex;
        align-items: center;
        justify-content: center;
        width: 30px;
        height: 30px;
        padding: 0;
        border: 2px solid ${theme.colors.border.weak};
        border-radius: 6px;
        background: ${theme.colors.background.secondary};
        color: #8a8a8a;
        cursor: pointer;

        &:hover {
          background: ${theme.colors.action.hover};
        }

        &.topology-color-menu-button-enabled {
          border-color: ${theme.colors.primary.main};
          background: ${colorManipulator.alpha(theme.colors.primary.main, 0.16)};
          color: ${theme.colors.primary.main};
        }

        &:focus-visible {
          outline: 2px solid ${theme.colors.primary.main};
          outline-offset: 1px;
        }
      }

      .topology-color-tooltip {
        position: absolute;
        right: 8px;
        bottom: 40px;
        min-width: 176px;
        padding: 10px;
        border: 1px solid ${theme.colors.border.weak};
        border-radius: 8px;
        background: ${colorManipulator.alpha(theme.colors.background.secondary, 0.72)};
        backdrop-filter: blur(10px);
        -webkit-backdrop-filter: blur(10px);
        box-shadow: ${theme.shadows.z2};
        opacity: 0;
        visibility: hidden;
        transform: translateY(4px);
        transition: opacity 120ms ease, transform 120ms ease, visibility 120ms ease;
        pointer-events: none;
      }

      .topology-color-control:hover .topology-color-tooltip,
      .topology-color-control:focus-within .topology-color-tooltip {
        opacity: 1;
        visibility: visible;
        transform: translateY(0);
      }

      .topology-tooltip-title {
        margin-bottom: 4px;
        color: ${theme.colors.text.primary};
        font-size: 11px;
        font-weight: 600;
      }

      .topology-tooltip-description {
        margin-bottom: 6px;
        color: ${theme.colors.text.secondary};
        font-size: 11px;
        line-height: 1.4;
      }

      .topology-color-legend {
        display: grid;
        gap: 6px;
        color: ${theme.colors.text.secondary};
        font-size: 11px;
      }

      .topology-color-legend-item {
        display: flex;
        align-items: center;
        gap: 8px;
      }

      .topology-color-swatch {
        width: 10px;
        height: 10px;
        flex: 0 0 auto;
        border-radius: 2px;
      }

      .topology-switch-control {
        position: absolute;
        right: 53px;
        bottom: 15px;
        z-index: 5;
      }

      .topology-switch-menu-button {
        display: flex;
        align-items: center;
        justify-content: center;
        width: 30px;
        height: 30px;
        padding: 0;
        border: 2px solid ${theme.colors.border.weak};
        border-radius: 6px;
        background: ${theme.colors.background.secondary};
        color: #8a8a8a;
        cursor: pointer;

        &:hover {
          background: ${theme.colors.action.hover};
        }

        &.topology-switch-menu-button-enabled {
          border-color: ${theme.colors.primary.main};
          background: ${colorManipulator.alpha(theme.colors.primary.main, 0.16)};
          color: ${theme.colors.primary.main};
        }

        &:focus-visible {
          outline: 2px solid ${theme.colors.primary.main};
          outline-offset: 1px;
        }
      }

      .topology-switch-tooltip {
        position: absolute;
        right: 0;
        bottom: 40px;
        min-width: 200px;
        padding: 10px;
        border: 1px solid ${theme.colors.border.weak};
        border-radius: 8px;
        background: ${colorManipulator.alpha(theme.colors.background.secondary, 0.72)};
        backdrop-filter: blur(10px);
        -webkit-backdrop-filter: blur(10px);
        box-shadow: ${theme.shadows.z2};
        opacity: 0;
        visibility: hidden;
        transform: translateY(4px);
        transition: opacity 120ms ease, transform 120ms ease, visibility 120ms ease;
        pointer-events: none;
      }

      .topology-switch-control:hover .topology-switch-tooltip,
      .topology-switch-control:focus-within .topology-switch-tooltip {
        opacity: 1;
        visibility: visible;
        transform: translateY(0);
      }

      .topology-detached-control {
        position: absolute;
        right: 91px;
        bottom: 15px;
        z-index: 5;
      }

      .topology-detached-menu-button {
        display: flex;
        align-items: center;
        justify-content: center;
        width: 30px;
        height: 30px;
        padding: 0;
        border: 2px solid ${theme.colors.border.weak};
        border-radius: 6px;
        background: ${theme.colors.background.secondary};
        color: #8a8a8a;
        cursor: pointer;

        &:hover {
          background: ${theme.colors.action.hover};
        }

        &.topology-detached-menu-button-enabled {
          border-color: ${theme.colors.primary.main};
          background: ${colorManipulator.alpha(theme.colors.primary.main, 0.16)};
          color: ${theme.colors.primary.main};
        }

        &:focus-visible {
          outline: 2px solid ${theme.colors.primary.main};
          outline-offset: 1px;
        }
      }

      .topology-detached-tooltip {
        position: absolute;
        right: 0;
        bottom: 40px;
        min-width: 190px;
        padding: 10px;
        border: 1px solid ${theme.colors.border.weak};
        border-radius: 8px;
        background: ${colorManipulator.alpha(theme.colors.background.secondary, 0.72)};
        backdrop-filter: blur(10px);
        -webkit-backdrop-filter: blur(10px);
        box-shadow: ${theme.shadows.z2};
        opacity: 0;
        visibility: hidden;
        transform: translateY(4px);
        transition: opacity 120ms ease, transform 120ms ease, visibility 120ms ease;
        pointer-events: none;
      }

      .topology-detached-control:hover .topology-detached-tooltip,
      .topology-detached-control:focus-within .topology-detached-tooltip {
        opacity: 1;
        visibility: visible;
        transform: translateY(0);
      }
    `,
  };
};

function fieldValues(frame: DataFrame, name: string): unknown[] {
  return frame.fields.find((field) => field.name === name)?.values ?? [];
}

// The "nodes"/"edges" frame contract is produced by unifabric-unifabrictopology-datasource.
function toNodes(frame?: DataFrame): TopologyNodeDatum[] {
  if (!frame) {
    return [];
  }
  const ids = fieldValues(frame, 'id') as string[];
  const titles = fieldValues(frame, 'title') as string[];
  const details = fieldValues(frame, 'detail') as string[];
  const tiers = fieldValues(frame, 'tier') as number[];
  const deviceTypes = fieldValues(frame, 'deviceType') as string[];
  const nodeTypes = fieldValues(frame, 'nodeType') as string[];
  const topologies = fieldValues(frame, 'topology') as string[];
  const members = fieldValues(frame, 'members') as string[];
  const parents = fieldValues(frame, 'parent') as string[];
  const clusters = fieldValues(frame, 'cluster') as string[];
  const detached = fieldValues(frame, 'detached') as boolean[];

  return ids.map((nodeId, i) => ({
    id: nodeId,
    title: titles[i] ?? nodeId,
    detail: details[i] ?? titles[i] ?? nodeId,
    tier: tiers[i] ?? 0,
    deviceType: deviceTypes[i] ?? 'switch',
    nodeType: nodeTypes[i] ?? 'domain',
    topology: topologies[i] ?? '',
    members: members[i] ?? '',
    parent: parents[i] || undefined,
    cluster: clusters[i] ?? '',
    detached: detached[i] ?? false,
  }));
}

function toEdges(frame?: DataFrame): TopologyEdgeDatum[] {
  if (!frame) {
    return [];
  }
  const ids = fieldValues(frame, 'id') as string[];
  const sources = fieldValues(frame, 'source') as string[];
  const targets = fieldValues(frame, 'target') as string[];
  const kinds = fieldValues(frame, 'kind') as string[];

  return sources.map((source, i) => ({
    id: ids[i] ?? `${source}-${targets[i]}`,
    source,
    target: targets[i],
    kind: kinds[i] ?? '',
  }));
}

function resourceListFromFrame<T>(frame?: DataFrame): ResourceList<T> | undefined {
  if (!frame) {
    return undefined;
  }
  return fieldValues(frame, 'resource')[0] as ResourceList<T> | undefined;
}

function topologyResourcesToGraph(
  topologies: TopologyResource[],
  fabricNodes: FabricNodeResource[],
  cluster: string
): { nodes: TopologyNodeDatum[]; edges: TopologyEdgeDatum[] } {
  const nodes: TopologyNodeDatum[] = [];
  const edges: TopologyEdgeDatum[] = [];
  const seenHosts = new Set<string>();
  const hostId = (name: string) => `host/${name}`;

  for (const item of topologies) {
    const topology = item.metadata.name;
    const domainId = (name: string) => `${topology}/${name}`;

    for (const domain of item.status?.domains ?? []) {
      const id = domainId(domain.name);
      nodes.push({
        id,
        title: domain.name,
        detail: domain.switchMember?.length
          ? `${topology}: ${domain.name} — switches: ${domain.switchMember.join(', ')}`
          : `${topology}: ${domain.name}`,
        tier: domain.tier,
        deviceType: 'switch',
        nodeType: 'domain',
        topology,
        members: domain.switchMember?.join(', ') ?? '',
        parent: domain.parent,
        cluster,
      });

      if (domain.parent) {
        edges.push({ id: `${id}->${domainId(domain.parent)}`, source: id, target: domainId(domain.parent), kind: 'parent' });
      }
    }

    for (const group of item.status?.nodes ?? []) {
      const nearestDomain = group.switchDomainPath[group.switchDomainPath.length - 1];

      for (const hostName of group.nodes) {
        const id = hostId(hostName);
        if (!seenHosts.has(hostName)) {
          seenHosts.add(hostName);
          nodes.push({
            id,
            title: hostName,
            detail: hostName,
            tier: 0,
            deviceType: 'host',
            nodeType: 'host',
            topology: '',
            members: '',
            cluster,
            detached: false,
          });
        }

        if (nearestDomain) {
          edges.push({ id: `${id}->${domainId(nearestDomain)}`, source: id, target: domainId(nearestDomain), kind: 'member' });
        }
      }
    }
  }

  for (const fabricNode of fabricNodes) {
    const hostName = fabricNode.metadata.name;
    if (!seenHosts.has(hostName)) {
      nodes.push({
        id: hostId(hostName),
        title: hostName,
        detail: hostName,
        tier: 0,
        deviceType: 'host',
        nodeType: 'host',
        topology: '',
        members: '',
        cluster,
        detached: true,
      });
      seenHosts.add(hostName);
    }
  }

  return { nodes, edges };
}

function toGraphData(series: DataFrame[]): { nodes: TopologyNodeDatum[]; edges: TopologyEdgeDatum[] } | undefined {
  const legacyNodesFrame = series.find((frame) => frame.name === 'nodes');
  if (legacyNodesFrame) {
    return {
      nodes: toNodes(legacyNodesFrame),
      edges: toEdges(series.find((frame) => frame.name === 'edges')),
    };
  }

  const topologiesFrame = series.find((frame) => frame.name === 'topologies');
  const fabricNodesFrame = series.find((frame) => frame.name === 'fabricNodes');
  if (!topologiesFrame && !fabricNodesFrame) {
    return undefined;
  }

  const cluster = (fieldValues(topologiesFrame ?? fabricNodesFrame!, 'cluster')[0] as string | undefined) ?? '';
  const topologies = resourceListFromFrame<TopologyResource>(topologiesFrame)?.items ?? [];
  const fabricNodes = resourceListFromFrame<FabricNodeResource>(fabricNodesFrame)?.items ?? [];
  return topologyResourcesToGraph(topologies, fabricNodes, cluster);
}

interface BoxNodeData {
  label: string;
  meta?: string;
  members?: string;
  topology?: string;
  detail: string;
  showLabel: boolean;
  selected: boolean;
  colorized?: boolean;
  /** Cluster + raw tier/parent, threaded through for hover-card resolution (see resolveHoverTarget). */
  cluster?: string;
  tier?: number;
  parent?: string;
}

const hiddenHandle = css`
  opacity: 0;
`;

// Every card gets both top and bottom source/target handles so edges can connect upward (e.g. host
// -> scaleout domain) or downward (e.g. host -> storage domain) depending on where they land.
function CardHandles() {
  return (
    <>
      <Handle id="top-target" type="target" position={Position.Top} className={hiddenHandle} />
      <Handle id="top-source" type="source" position={Position.Top} className={hiddenHandle} />
      <Handle id="bottom-source" type="source" position={Position.Bottom} className={hiddenHandle} />
      <Handle id="bottom-target" type="target" position={Position.Bottom} className={hiddenHandle} />
    </>
  );
}

// Card look shared by domain/host nodes, styled after .tmp/ui1 and .tmp/ui2's reference UI
// (icon chip + title + meta, rounded card, highlighted border when selected). `paddingY` lets
// domain cards get a bit more vertical breathing room than the compact host cards.
function cardStyles(
  theme: GrafanaTheme2,
  selected: boolean,
  paddingY = 6,
  borderWidth = 1,
  accentColor?: string,
  ringWidth = 2,
  borderColorOverride?: string
) {
  const borderColor = accentColor ?? (selected ? theme.colors.primary.main : borderColorOverride ?? theme.colors.border.weak);
  const selectionRing = selected
    ? accentColor
      ? `0 0 0 ${ringWidth}px ${colorManipulator.alpha(accentColor, 0.24)}`
      : `0 0 0 2px ${theme.colors.primary.transparent}`
    : 'none';

  return css`
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: ${paddingY}px 10px;
    box-sizing: border-box;
    border-radius: 10px;
    border: ${borderWidth}px solid ${borderColor};
    background: ${
      accentColor
        ? `linear-gradient(${colorManipulator.alpha(accentColor, 0.08)}, ${colorManipulator.alpha(accentColor, 0.08)}), ${theme.colors.background.primary}`
        : theme.colors.background.primary
    };
    box-shadow: ${selectionRing};
    transition: border-color 120ms ease, box-shadow 120ms ease;
  `;
}

function iconChipStyles(theme: GrafanaTheme2, accentColor?: string) {
  return css`
    display: flex;
    align-items: center;
    justify-content: center;
    flex: 0 0 auto;
    width: 26px;
    height: 26px;
    border-radius: 8px;
    background: ${accentColor ? colorManipulator.alpha(accentColor, 0.16) : theme.colors.background.secondary};
    color: ${accentColor ?? theme.colors.text.secondary};
  `;
}

const TOPOLOGY_LABELS: Record<string, string> = {
  scaleout: 'Scale-Out',
  scaleup: 'Scale-Up',
  storage: 'Storage',
};

const TOPOLOGY_COLORS: Record<string, string> = {
  storage: '#5343B3',
  scaleout: '#328CE6',
  scaleup: '#2FA89A',
};

const TOPOLOGY_COLOR_ENTRIES = [
  { key: 'storage', label: 'Storage', color: TOPOLOGY_COLORS.storage },
  { key: 'scaleout', label: 'Scale Out', color: TOPOLOGY_COLORS.scaleout },
  { key: 'scaleup', label: 'Scale Up', color: TOPOLOGY_COLORS.scaleup },
];

function topologyLabel(topology: string): string {
  return TOPOLOGY_LABELS[topology] ?? topology;
}

function DomainNode({ data }: NodeProps<BoxNodeData>) {
  const theme = useTheme2();
  const accentColor = data.colorized && data.topology ? TOPOLOGY_COLORS[data.topology] : undefined;
  return (
    <div
      data-testid="topology-panel-node"
      title={data.detail}
      className={cardStyles(theme, data.selected, 10, 2, accentColor)}
    >
      <CardHandles />
      <div className={iconChipStyles(theme, accentColor)}>
        <Network size={14} />
      </div>
      {data.showLabel && (
        <div
          className={css`
            min-width: 0;
          `}
        >
          {(data.topology || data.meta) && (
            <div
              className={css`
                font-size: 10px;
                font-weight: 600;
                color: ${theme.colors.text.secondary};
              `}
            >
              {[data.topology && topologyLabel(data.topology), data.meta].filter(Boolean).join(' · ')}
            </div>
          )}
          <div
            className={css`
              overflow: hidden;
              text-overflow: ellipsis;
              white-space: nowrap;
              font-size: 11px;
              font-weight: 600;
              color: ${theme.colors.text.primary};
            `}
          >
            {data.label}
          </div>
          {data.members && (
            <div
              className={css`
                margin-top: 2px;
              `}
            >
              <div
                className={css`
                  font-size: 9px;
                  font-weight: 600;
                  color: ${theme.colors.text.secondary};
                `}
              >
                Members
              </div>
              {data.members.split(', ').map((member) => (
                <div
                  key={member}
                  className={css`
                    overflow: hidden;
                    text-overflow: ellipsis;
                    white-space: nowrap;
                    font-size: 10px;
                    color: ${theme.colors.text.secondary};
                  `}
                >
                  • {member}
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function HostNode({ data }: NodeProps<BoxNodeData>) {
  const theme = useTheme2();
  // border.weak reads as too faint against a light background; border.medium alone is fine in dark.
  const borderColorOverride = theme.colors.mode === 'light' ? theme.colors.border.medium : undefined;
  return (
    <div
      data-testid="topology-panel-node"
      title={data.detail}
      className={cardStyles(theme, data.selected, 6, 2, undefined, undefined, borderColorOverride)}
    >
      <CardHandles />
      <div className={iconChipStyles(theme)}>
        <Server size={13} />
      </div>
      {data.showLabel && (
        <div
          className={css`
            min-width: 0;
          `}
        >
          <div
            className={css`
              font-size: 9px;
              font-weight: 600;
              color: ${theme.colors.text.secondary};
            `}
          >
            Node
          </div>
          <div
            className={css`
              overflow: hidden;
              text-overflow: ellipsis;
              white-space: nowrap;
              font-size: 11px;
              font-weight: 500;
              color: ${theme.colors.text.primary};
            `}
          >
            {data.label}
          </div>
        </div>
      )}
    </div>
  );
}

// One rectangle per switch, wrapped by its parent SwitchGroupNode (React Flow `parentNode` +
// `extent: 'parent'`). Reuses BoxNodeData/CardHandles/cardStyles like DomainNode/HostNode so the
// selected/colorized visuals stay consistent with the rest of the graph.
function SwitchMemberNode({ data }: NodeProps<BoxNodeData>) {
  const theme = useTheme2();
  const accentColor = data.colorized && data.topology ? TOPOLOGY_COLORS[data.topology] : undefined;
  return (
    <div
      data-testid="topology-panel-node"
      title={data.detail}
      className={cardStyles(theme, data.selected, 6, 2, accentColor)}
    >
      <CardHandles />
      <div className={iconChipStyles(theme, accentColor)}>
        <Network size={12} />
      </div>
      {data.showLabel && (
        <div
          className={css`
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
            font-size: 11px;
            font-weight: 500;
            color: ${theme.colors.text.primary};
          `}
        >
          {data.label}
        </div>
      )}
    </div>
  );
}

// Background "group" box a domain's switch members render inside of when switch view is enabled.
// Not selectable/clickable itself (see handleNodeClick) — only the individual switch rectangles
// respond to selection, since edges now attach to switches rather than to the domain as a whole.
// Always opaque, same as every other domain/device box in the panel — plain `background.primary`
// (the panel's own background, e.g. white in light theme), not accent-tinted like cardStyles'
// device rectangles, since this is a functional occlusion fix (hiding edges that cross behind a
// group, e.g. host<->scaleout edges crossing the scaleup band) rather than a colorization surface.
function SwitchGroupNode({ data }: NodeProps<BoxNodeData>) {
  const theme = useTheme2();
  const accentColor = data.colorized && data.topology ? TOPOLOGY_COLORS[data.topology] : undefined;
  return (
    <div
      data-testid="topology-switch-group"
      title={data.detail}
      className={css`
        width: 100%;
        height: 100%;
        box-sizing: border-box;
        border: 2px dashed ${accentColor ?? theme.colors.border.medium};
        border-radius: 10px;
        background: ${theme.colors.background.primary};
      `}
    >
      {data.showLabel && (
        <div
          className={css`
            padding: 4px 8px;
          `}
        >
          {(data.topology || data.meta) && (
            <div
              className={css`
                overflow: hidden;
                text-overflow: ellipsis;
                white-space: nowrap;
                font-size: 10px;
                font-weight: 600;
                color: ${theme.colors.text.secondary};
              `}
            >
              {[data.topology && topologyLabel(data.topology), data.meta].filter(Boolean).join(' · ')}
            </div>
          )}
          <div
            className={css`
              overflow: hidden;
              text-overflow: ellipsis;
              white-space: nowrap;
              font-size: 11px;
              font-weight: 600;
              color: ${theme.colors.text.primary};
            `}
          >
            {data.label}
          </div>
        </div>
      )}
    </div>
  );
}

function DetachedGroupNode({ data }: NodeProps<BoxNodeData>) {
  const theme = useTheme2();
  return (
    <div
      data-testid="topology-detached-group"
      title={data.detail}
      className={css`
        width: 100%;
        height: 100%;
        box-sizing: border-box;
        border: 2px dashed ${theme.colors.border.medium};
        border-radius: 10px;
        background: ${theme.colors.background.primary};
      `}
    >
      {data.showLabel && (
        <div
          className={css`
            padding: 4px 8px;
          `}
        >
          <div
            className={css`
              font-size: 10px;
              font-weight: 600;
              color: ${theme.colors.text.secondary};
            `}
          >
            {data.meta}
          </div>
          <div
            className={css`
              overflow: hidden;
              text-overflow: ellipsis;
              white-space: nowrap;
              font-size: 11px;
              font-weight: 600;
              color: ${theme.colors.text.primary};
            `}
          >
            {data.label}
          </div>
        </div>
      )}
    </div>
  );
}

function EmptyTopologyState() {
  const theme = useTheme2();

  return (
    <div
      data-testid="topology-panel-empty-state"
      className={css`
        position: absolute;
        inset: 0;
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        gap: 8px;
        color: ${theme.colors.text.secondary};
        text-align: center;
      `}
    >
      <Network size={28} strokeWidth={1.5} />
      <div
        className={css`
          color: ${theme.colors.text.primary};
          font-size: 14px;
          font-weight: 600;
        `}
      >
        No topology information
      </div>
      <div
        className={css`
          font-size: 12px;
        `}
      >
        The current cluster has no topology data to display.
      </div>
    </div>
  );
}

interface ColorizationMenuProps {
  enabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
}

// User-supplied icon (iconfont-style, 16x15), used verbatim at its native size/color.
function ResourceColorsIcon() {
  return (
    <svg width="16" height="15" viewBox="0 0 16 15" fill="none" aria-hidden="true">
      <path d="M0 7.34866C0 2.2344 4.428 -0.507732 9.29867 0.0779795C13.2147 0.55012 16 3.42654 16 6.96938C16 8.84723 15.1267 9.67223 13.402 10.1787C13.2847 10.2137 13.174 10.2437 12.9587 10.3037C12.2093 10.5122 11.912 10.6515 11.7853 10.8465C11.6293 11.0894 11.6167 11.2615 11.6933 11.7965C11.72 11.9872 11.7307 12.0701 11.7413 12.1851C11.828 13.1765 11.452 13.9236 10.3567 14.6008C9.09533 15.3815 6.52667 14.9779 4.28333 13.7322C1.68267 12.2879 0 10.0301 0 7.34866ZM9.16267 1.40583C4.932 0.895119 1.23067 3.18725 1.23067 7.34866C1.23067 9.44866 2.61467 11.3058 4.844 12.5444C6.75533 13.6058 8.92933 13.9479 9.746 13.4422C10.4307 13.0186 10.556 12.7686 10.516 12.3115C10.5061 12.2073 10.4932 12.1034 10.4773 12.0001C10.356 11.1529 10.3887 10.6829 10.778 10.0801C11.1393 9.51937 11.602 9.30437 12.6513 9.01152C12.8687 8.9508 12.9713 8.92223 13.0807 8.89009C14.33 8.52366 14.7693 8.10938 14.7693 6.96867C14.7693 4.15725 12.492 1.80583 9.16267 1.4044V1.40512V1.40583ZM3.94933 6.79224C4.51533 6.79224 4.97467 7.29009 4.97467 7.90438C4.97467 8.51866 4.51467 9.01652 3.94867 9.01652C3.382 9.01652 2.92333 8.51866 2.92333 7.90438C2.92333 7.29009 3.382 6.79224 3.94867 6.79224H3.94933ZM9.846 3.40939C10.4127 3.40939 10.872 3.90725 10.872 4.52153C10.872 5.13582 10.412 5.63367 9.846 5.63367C9.27933 5.63367 8.82067 5.1351 8.82067 4.52153C8.82067 3.90725 9.28 3.40939 9.846 3.40939ZM5.74333 3.34511C6.11 3.34511 6.44867 3.55725 6.632 3.90082C6.72207 4.07026 6.76944 4.2619 6.76944 4.45689C6.76944 4.65188 6.72207 4.84352 6.632 5.01296C6.54295 5.18155 6.41381 5.32185 6.25771 5.41959C6.10161 5.51733 5.92413 5.56902 5.74333 5.56939C5.17667 5.56939 4.718 5.07153 4.718 4.45725C4.718 3.84296 5.17733 3.34511 5.74333 3.34511Z" fill="currentColor" />
    </svg>
  );
}

function ColorizationMenu({ enabled, onEnabledChange }: ColorizationMenuProps) {
  return (
    <div className="topology-color-control">
      <div className="topology-color-tooltip" data-testid="topology-color-tooltip" role="tooltip">
        <div className="topology-tooltip-title">Resource colors</div>
        <div className="topology-tooltip-description">Color domains and switches by resource type.</div>
        <div className="topology-color-legend" aria-label="Resource type colors">
          {TOPOLOGY_COLOR_ENTRIES.map((entry) => (
            <div className="topology-color-legend-item" key={entry.key}>
              <span className="topology-color-swatch" style={{ backgroundColor: entry.color }} />
              <span>{entry.label}</span>
            </div>
          ))}
        </div>
      </div>
      <button
        type="button"
        className={cx('topology-color-menu-button', enabled && 'topology-color-menu-button-enabled')}
        aria-label="Toggle topology colors"
        aria-pressed={enabled}
        title="Toggle topology colors"
        onClick={() => onEnabledChange(!enabled)}
      >
        <ResourceColorsIcon />
      </button>
    </div>
  );
}

interface SwitchViewMenuProps {
  enabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
}

// User-supplied icon (iconfont-style, 16x14), used verbatim at its native size/color.
function SwitchViewIcon() {
  return (
    <svg width="16" height="14" viewBox="0 0 16 14" fill="none" aria-hidden="true">
      <path d="M14.6764 10.3498V8.93238C14.6764 7.64566 13.7324 6.59925 12.5724 6.59925H8.40217V3.64784H9.32901C9.37885 3.64879 9.42836 3.63949 9.4746 3.62049C9.52084 3.60149 9.56286 3.57319 9.59816 3.53727C9.63345 3.50135 9.66131 3.45854 9.68006 3.4114C9.69881 3.36426 9.70807 3.31375 9.7073 3.26288V0.486457C9.7073 0.226313 9.51301 1.59491e-08 9.26959 0H6.67077C6.46163 0 6.29249 0.198316 6.29249 0.422296V3.19872C6.29249 3.45886 6.48677 3.64901 6.73019 3.64901H7.59761V6.59925H3.24338C2.2034 6.59925 1.32341 7.55117 1.32341 8.67923V10.3498H0.370281C0.161141 10.3498 0 10.5481 0 10.7732V13.5496C0 13.8098 0.187426 13.9988 0.429708 13.9988H3.02967C3.07987 13.9998 3.12977 13.9907 3.17646 13.9718C3.22314 13.953 3.26566 13.9248 3.30154 13.8889C3.33741 13.8531 3.36591 13.8103 3.38536 13.763C3.40481 13.7158 3.41482 13.665 3.41481 13.6138V10.8374C3.41481 10.5772 3.21253 10.3498 2.9691 10.3498H2.0914V8.67923C2.0914 7.99563 2.61939 7.41934 3.24338 7.41934H7.59761V10.3498H6.67077C6.46163 10.3498 6.29249 10.5481 6.29249 10.7732V13.5496C6.29249 13.8098 6.48677 13.9988 6.73019 13.9988H9.32901C9.37889 13.9999 9.42848 13.9907 9.47479 13.9717C9.5211 13.9528 9.56318 13.9245 9.59851 13.8886C9.63384 13.8526 9.66169 13.8097 9.68038 13.7625C9.69908 13.7153 9.70823 13.6647 9.7073 13.6138V10.8374C9.7073 10.5772 9.51301 10.3498 9.26959 10.3498H8.40217V7.41934H12.5724C13.3095 7.41934 13.9095 8.09945 13.9095 8.93354V10.3509H12.9701C12.761 10.3509 12.585 10.5492 12.585 10.7744V13.5508C12.585 13.8109 12.7873 13.9999 13.0307 13.9999H15.6284C15.6779 14.0009 15.7271 13.9916 15.773 13.9725C15.8188 13.9534 15.8604 13.925 15.8952 13.889C15.9299 13.853 15.9571 13.8101 15.9751 13.763C15.9931 13.7159 16.0015 13.6655 15.9998 13.615V10.8374C15.9998 10.5772 15.8124 10.3498 15.5701 10.3498H14.6764Z" fill="currentColor" />
    </svg>
  );
}

// Toggle for rendering each domain's switch members as individual boxes grouped inside the domain's
// outline, with connections drawn switch-to-switch instead of domain-to-domain/domain-to-host.
function SwitchViewMenu({ enabled, onEnabledChange }: SwitchViewMenuProps) {
  return (
    <div className="topology-switch-control">
      <div className="topology-switch-tooltip" data-testid="topology-switch-tooltip" role="tooltip">
        <div className="topology-tooltip-title">Switch view</div>
        <div className="topology-tooltip-description">
          Show every switch as its own box, grouped by domain, and connect switches directly instead of
          domain groups.
        </div>
      </div>
      <button
        type="button"
        className={cx('topology-switch-menu-button', enabled && 'topology-switch-menu-button-enabled')}
        aria-label="Toggle switch view"
        aria-pressed={enabled}
        title="Toggle switch view"
        onClick={() => onEnabledChange(!enabled)}
      >
        <SwitchViewIcon />
      </button>
    </div>
  );
}

interface DetachedNodesMenuProps {
  enabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
}

function DetachedNodesIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 100 100" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
      <path
        d="M73.9994 23.0007H47.0003C46.1727 23.0007 45.4647 23.2938 44.8782 23.8798C44.2904 24.4681 43.9973 25.1734 43.9973 26.002V53.0017C43.9973 53.8293 44.2904 54.5342 44.8782 55.1225C45.4647 55.7068 46.1727 56.002 47.0003 56.002H73.9994C74.8279 56.002 75.535 55.7068 76.1202 55.1229C76.7089 54.5342 77.0015 53.8293 77.0015 53.0017V26.002C77.0015 25.173 76.7089 24.4681 76.1206 23.8798C75.5346 23.2938 74.8279 23.0007 73.9994 23.0007ZM37.9989 52.9982V43.9984H26.0015C25.1721 43.9984 24.4681 44.2928 23.8776 44.8775C23.2937 45.4654 22.9985 46.1711 22.9985 46.9996V73.9985C22.9985 74.8274 23.2933 75.5358 23.8776 76.1153C24.4681 76.7111 25.1721 76.9953 26.0015 76.9953H53.0006C53.8282 76.9953 54.5344 76.7111 55.1235 76.1153C55.7079 75.5358 56.0027 74.8274 56.0027 73.9989V61.9992H47.0021C44.5196 61.9992 42.3953 61.121 40.6375 59.3628C38.8802 57.605 38.0011 55.4837 38.0011 53.0012L37.9993 52.9977L37.9989 52.9982ZM47.0008 17H73.9999C76.485 17 78.6053 17.8782 80.3635 19.6365C82.1218 21.3947 83 23.516 83 25.9989V52.9982C83 55.4842 82.1218 57.6014 80.3635 59.3614C78.6053 61.1188 76.485 61.9979 73.9999 61.9979H62.0011V73.9967C62.0011 76.4827 61.1216 78.6 59.3642 80.36C57.606 82.12 55.4835 83 53.0006 83H26.001C23.5177 83 21.3947 82.12 19.6365 80.36C17.8782 78.6 17 76.4823 17 73.9963V46.9983C17 44.5141 17.8782 42.392 19.6365 40.6346C21.3947 38.8764 23.5177 37.9986 26.001 37.9986H37.9993V25.9989C37.9993 23.516 38.8784 21.3947 40.6358 19.6365C42.394 17.8782 44.5152 17 47.0003 17H47.0008Z"
        fill="#8A8A8A"
        style={{ fill: '#8A8A8A', fillOpacity: 1 }}
      />
    </svg>
  );
}

function DetachedNodesMenu({ enabled, onEnabledChange }: DetachedNodesMenuProps) {
  return (
    <div className="topology-detached-control">
      <div className="topology-detached-tooltip" data-testid="topology-detached-tooltip" role="tooltip">
        <div className="topology-tooltip-title">Outside topology</div>
        <div className="topology-tooltip-description">Show nodes that do not appear in any topology.</div>
      </div>
      <button
        type="button"
        className={cx('topology-detached-menu-button', enabled && 'topology-detached-menu-button-enabled')}
        aria-label="Toggle nodes outside topology"
        aria-pressed={enabled}
        title="Toggle nodes outside topology"
        onClick={() => onEnabledChange(!enabled)}
      >
        <DetachedNodesIcon />
      </button>
    </div>
  );
}

const NODE_TYPES = {
  domain: DomainNode,
  host: HostNode,
  switchGroup: SwitchGroupNode,
  switchMember: SwitchMemberNode,
  detachedGroup: DetachedGroupNode,
};

const DOMAIN_WIDTH = 170;
const HOST_SIZE = { width: DOMAIN_WIDTH, height: 44 };
const DETACHED_GROUP_ID = 'detached-nodes';
const DETACHED_GROUP_WIDTH = HOST_SIZE.width + 16;
const DETACHED_GROUP_LABEL_HEIGHT = 42;
const DETACHED_GROUP_PADDING_BOTTOM = 8;
const DETACHED_NODE_GAP = 8;
const DETACHED_SIDE_GAP = 40;

// Domain card height adapts to its content: a fixed base for the icon/category/title lines, plus
// one row for the "Members" heading and one row per member, so the card never clips or overflows
// regardless of how many members a domain has. Base height also accounts for the domain card's
// extra 4px top/bottom padding (see cardStyles' paddingY) on top of the icon/title content itself.
const DOMAIN_BASE_HEIGHT = 48;
const MEMBERS_TOP_MARGIN = 2;
const MEMBERS_HEADING_HEIGHT = 13;
const MEMBER_ROW_HEIGHT = 13;

function domainNodeHeight(node: TopologyNodeDatum): number {
  const memberCount = node.members ? node.members.split(', ').filter(Boolean).length : 0;
  if (memberCount === 0) {
    return DOMAIN_BASE_HEIGHT;
  }
  return DOMAIN_BASE_HEIGHT + MEMBERS_TOP_MARGIN + MEMBERS_HEADING_HEIGHT + memberCount * MEMBER_ROW_HEIGHT;
}

function switchMembersOf(node: TopologyNodeDatum): string[] {
  return node.members ? node.members.split(', ').filter(Boolean) : [];
}

function switchNodeId(domainId: string, member: string): string {
  return `${domainId}::switch::${member}`;
}

// Switch view lays a domain's switch members out in a single row inside a wrapping group box, so
// the group's width grows with member count instead of staying at the fixed DOMAIN_WIDTH. Each
// switch box is the same fixed size as a host card (HOST_SIZE) rather than sized to its own text,
// matching the rest of the graph's "every box of a given kind is the same size" convention.
const SWITCH_WIDTH = HOST_SIZE.width;
const SWITCH_HEIGHT = HOST_SIZE.height;
const SWITCH_GAP = 8;
const GROUP_PADDING_X = 8;
// Tall enough for a two-line label (meta line + title line, same as the collapsed DomainNode's
// own header, plus its own 4px bottom padding) before the switch row starts underneath it.
const GROUP_LABEL_HEIGHT = 42;
const GROUP_PADDING_BOTTOM = 8;
const GROUP_LABEL_CHAR_WIDTH = 5.6;
// Matches the group label div's own `padding: 4px 8px` (8px each side).
const GROUP_LABEL_TEXT_PADDING_X = 16;

// The group's label is two lines (meta line, title line, see SwitchGroupNode), so its minimum
// width has to fit the wider of the two — usually moot once there's at least one switch
// (SWITCH_WIDTH alone comfortably fits typical labels), but keeps very-long domain titles or a
// zero-member group from ending up narrower than their own label.
function groupLabelMinWidth(node: TopologyNodeDatum): number {
  const metaLine = [node.topology && topologyLabel(node.topology), `Tier-${node.tier}`].filter(Boolean).join(' · ');
  const widest = Math.max(metaLine.length, node.title.length);
  return Math.ceil(widest * GROUP_LABEL_CHAR_WIDTH) + GROUP_LABEL_TEXT_PADDING_X;
}

function domainGroupSize(node: TopologyNodeDatum): { width: number; height: number } {
  const columns = Math.max(switchMembersOf(node).length, 1);
  const switchesWidth = GROUP_PADDING_X * 2 + columns * SWITCH_WIDTH + (columns - 1) * SWITCH_GAP;

  return {
    width: Math.max(switchesWidth, groupLabelMinWidth(node)),
    height: GROUP_LABEL_HEIGHT + SWITCH_HEIGHT + GROUP_PADDING_BOTTOM,
  };
}

interface Region {
  x: number;
  y: number;
  width: number;
  height: number;
}

type PositionMap = Map<string, { x: number; y: number }>;

// Builds one switch-view group: a non-selectable background box (see handleNodeClick) plus one
// child rectangle per switch member, positioned in a single row and clamped inside the group via
// `extent: 'parent'`. Records each switch's center in `positions` (not the domain's), since edges
// attach to individual switches in this mode. A domain with zero members falls back to recording
// the group box's own center, so edges never end up with nowhere to attach.
function buildSwitchGroupNodes(
  node: TopologyNodeDatum,
  x: number,
  y: number,
  width: number,
  height: number,
  showLabels: boolean,
  positions: PositionMap
): Node[] {
  const members = switchMembersOf(node);
  const groupId = node.id;

  const nodes: Node[] = [
    {
      id: groupId,
      type: 'switchGroup',
      position: { x, y },
      style: { width, height },
      data: {
        label: node.title,
        meta: `Tier-${node.tier}`,
        topology: node.topology,
        detail: node.detail,
        showLabel: showLabels,
        selected: false,
        cluster: node.cluster,
        tier: node.tier,
        parent: node.parent,
      },
      draggable: true,
      selectable: false,
      // Lower than every device box (2) and even lower than a regular edge's own zIndex (1), so
      // edges generally draw on top of this group card; only edges explicitly given a zIndex below
      // this (see the `crossesScaleup` check in buildGraph) end up hidden behind it instead.
      zIndex: 0,
    },
  ];

  if (members.length === 0) {
    positions.set(groupId, { x: x + width / 2, y: y + height / 2 });
    return nodes;
  }

  const innerY = GROUP_LABEL_HEIGHT + (height - GROUP_LABEL_HEIGHT - SWITCH_HEIGHT - GROUP_PADDING_BOTTOM) / 2;
  const rowWidth = members.length * SWITCH_WIDTH + (members.length - 1) * SWITCH_GAP;
  // Centered rather than left-aligned at GROUP_PADDING_X: the group can be wider than its switches
  // strictly need (when the group's own label is the wider constraint), so this avoids the switch
  // row bunching up on one side with a lopsided gap on the other.
  let relX = (width - rowWidth) / 2;

  members.forEach((member) => {
    const switchId = switchNodeId(groupId, member);
    positions.set(switchId, { x: x + relX + SWITCH_WIDTH / 2, y: y + innerY + SWITCH_HEIGHT / 2 });

    nodes.push({
      id: switchId,
      type: 'switchMember',
      parentNode: groupId,
      extent: 'parent',
      position: { x: relX, y: innerY },
      style: { width: SWITCH_WIDTH, height: SWITCH_HEIGHT },
      data: {
        label: member,
        topology: node.topology,
        detail: `${node.detail} — ${member}`,
        showLabel: showLabels,
        selected: false,
        cluster: node.cluster,
      },
      draggable: false,
      zIndex: 2,
    });

    relX += SWITCH_WIDTH + SWITCH_GAP;
  });

  return nodes;
}

function buildDetachedGroupNodes(items: TopologyNodeDatum[], x: number, y: number, showLabels: boolean): Node[] {
  const contentHeight = items.length * HOST_SIZE.height + Math.max(0, items.length - 1) * DETACHED_NODE_GAP;
  const height = DETACHED_GROUP_LABEL_HEIGHT + contentHeight + DETACHED_GROUP_PADDING_BOTTOM;
  const nodes: Node[] = [
    {
      id: DETACHED_GROUP_ID,
      type: 'detachedGroup',
      position: { x, y },
      style: { width: DETACHED_GROUP_WIDTH, height },
      data: {
        label: 'Outside topology',
        meta: `${items.length} ${items.length === 1 ? 'node' : 'nodes'}`,
        detail: 'Nodes outside topology',
        showLabel: showLabels,
        selected: false,
      },
      draggable: true,
      selectable: false,
      zIndex: 0,
    },
  ];

  items.forEach((node, index) => {
    nodes.push({
      id: node.id,
      type: 'host',
      parentNode: DETACHED_GROUP_ID,
      extent: 'parent',
      position: {
        x: (DETACHED_GROUP_WIDTH - HOST_SIZE.width) / 2,
        y: DETACHED_GROUP_LABEL_HEIGHT + index * (HOST_SIZE.height + DETACHED_NODE_GAP),
      },
      style: HOST_SIZE,
      data: {
        label: node.title,
        detail: node.detail,
        showLabel: showLabels,
        selected: false,
        cluster: node.cluster,
      },
      draggable: false,
      zIndex: 2,
    });
  });

  return nodes;
}

// Lays out domains on a tier grid: rows stack starting from the edge nearest the shared hosts row
// (tier 1 is always nearest a host, by CRD convention) and use each row's own required height, with
// any leftover region height left as blank space on the far side instead of stretching rows apart.
// That avoids e.g. storage's single tier centering inside a region sized for a multi-tier scaleout,
// which used to leave a big gap next to hosts. `flip=false` (scaleout, scaleup) stacks from the
// bottom of the region upward; `flip=true` (storage) stacks from the top downward.
function layoutVerticalDomains(
  domains: TopologyNodeDatum[],
  region: Region,
  flip: boolean,
  showLabels: boolean,
  switchView: boolean,
  positions: PositionMap
): Node[] {
  const tiers = Array.from(new Set(domains.map((node) => node.tier))).sort((a, b) => a - b);
  const nodes: Node[] = [];

  let cursor = 0;
  tiers.forEach((tier) => {
    const row = domains.filter((node) => node.tier === tier);
    const rowHeight = switchView
      ? Math.max(...row.map((node) => domainGroupSize(node).height))
      : Math.max(...row.map((node) => domainNodeHeight(node)));
    const rowTop = flip ? region.y + cursor : region.y + region.height - cursor - rowHeight;

    if (switchView) {
      // Same even-cellWidth-division-then-center approach as the non-switchView branch below
      // (and layoutHorizontalRow's host row), so group-to-group spacing feels as roomy as
      // domain/host spacing instead of a fixed gap that looks cramped next to it.
      const cellWidth = region.width / row.length;

      row.forEach((node, i) => {
        const { width: nodeWidth, height: nodeHeight } = domainGroupSize(node);
        const x = region.x + cellWidth * i + (cellWidth - nodeWidth) / 2;
        const y = rowTop + (rowHeight - nodeHeight) / 2;
        nodes.push(...buildSwitchGroupNodes(node, x, y, nodeWidth, nodeHeight, showLabels, positions));
      });

      cursor += rowHeight + TIER_GAP;
      return;
    }

    const cellWidth = region.width / row.length;

    row.forEach((node, i) => {
      const nodeHeight = domainNodeHeight(node);
      const x = region.x + cellWidth * i + (cellWidth - DOMAIN_WIDTH) / 2;
      const y = rowTop + (rowHeight - nodeHeight) / 2;
      positions.set(node.id, { x: x + DOMAIN_WIDTH / 2, y: y + nodeHeight / 2 });

      nodes.push({
        id: node.id,
        type: 'domain',
        position: { x, y },
        style: { width: DOMAIN_WIDTH, height: nodeHeight },
        data: {
          label: node.title,
          meta: `Tier-${node.tier}`,
          members: node.members,
          topology: node.topology,
          detail: node.detail,
          showLabel: showLabels,
          selected: false,
          cluster: node.cluster,
          tier: node.tier,
          parent: node.parent,
        },
        draggable: true,
        zIndex: 2,
      });
    });

    cursor += rowHeight + TIER_GAP;
  });

  return nodes;
}

// Spreads a flat list of cards evenly across one row, centered in the region. Used for the shared,
// deduplicated host row (hosts have no tier concept, so they never stack into multiple rows) and
// for the scaleup band. In switch view, domain items (not hosts) render as variable-width groups
// packed by their own size instead of the uniform cellWidth grid.
function layoutHorizontalRow(
  items: TopologyNodeDatum[],
  region: Region,
  showLabels: boolean,
  switchView: boolean,
  positions: PositionMap
): Node[] {
  const isGroupedRow = switchView && items.length > 0 && items[0].nodeType !== 'host';

  if (isGroupedRow) {
    // Same even-cellWidth-division-then-center approach as the plain host/domain row below, so
    // group-to-group spacing feels as roomy as node-to-node spacing instead of a fixed gap.
    const cellWidth = region.width / items.length;
    const nodes: Node[] = [];

    items.forEach((node, i) => {
      const { width, height } = node.nodeType === 'host' ? HOST_SIZE : domainGroupSize(node);
      const x = region.x + cellWidth * i + (cellWidth - width) / 2;
      const y = region.y + (region.height - height) / 2;

      if (node.nodeType === 'host') {
        positions.set(node.id, { x: x + width / 2, y: y + height / 2 });
        nodes.push({
          id: node.id,
          type: 'host',
          position: { x, y },
          style: { width, height },
          data: { label: node.title, detail: node.detail, showLabel: showLabels, selected: false, cluster: node.cluster },
          draggable: true,
          zIndex: 2,
        });
      } else {
        nodes.push(...buildSwitchGroupNodes(node, x, y, width, height, showLabels, positions));
      }
    });

    return nodes;
  }

  const cellWidth = region.width / (items.length || 1);

  return items.map((node, i) => {
    const isHost = node.nodeType === 'host';
    const width = isHost ? HOST_SIZE.width : DOMAIN_WIDTH;
    const height = isHost ? HOST_SIZE.height : domainNodeHeight(node);
    const x = region.x + cellWidth * i + (cellWidth - width) / 2;
    const y = region.y + (region.height - height) / 2;
    positions.set(node.id, { x: x + width / 2, y: y + height / 2 });

    return {
      id: node.id,
      type: isHost ? 'host' : 'domain',
      position: { x, y },
      style: { width, height },
      data: {
        label: node.title,
        meta: isHost ? undefined : `Tier-${node.tier}`,
        members: isHost ? undefined : node.members,
        topology: isHost ? undefined : node.topology,
        detail: node.detail,
        showLabel: showLabels,
        selected: false,
        cluster: node.cluster,
        tier: isHost ? undefined : node.tier,
        parent: isHost ? undefined : node.parent,
      },
      draggable: true,
      zIndex: 2,
    };
  });
}

// scaleout on top (tiers descending), storage on the bottom (tiers ascending, mirrored), the
// shared/deduplicated host row sandwiched in between, and scaleup directly above hosts (tiers
// descending, same direction as scaleout since both sit above the host row).
const BANDS = [{ key: 'scaleout' }, { key: 'scaleup' }, { key: 'hosts' }, { key: 'storage' }] as const;

// Extra fixed vertical space left between adjacent bands, on top of each band's own height. Each
// band also has an implicit 8px pad top+bottom (see `region` below), so the actual visible gap
// between two bands' content is this plus 16 (e.g. 64 here -> 80px effective).
const BAND_GAP = 64;

// Vertical space left between adjacent tiers within the same band (e.g. leaf and spine within
// scaleout). Without this, tiers stacked flush against each other, so a leaf's top edge and the
// spine's bottom edge landed on the same Y and the connecting line looked almost flat. Unlike
// BAND_GAP, there's no extra per-tier padding, so this raw value is already the effective gap —
// kept equal to BAND_GAP's own 80px effective gap so within-band and between-band spacing match.
const TIER_GAP = 80;

// Number of stacked tiers a band needs: scaleout/scaleup/storage each stack one row per distinct
// tier (storage is always exactly one tier, by design); hosts have no tier concept and are always a
// single flat row.
function bandTierCount(key: (typeof BANDS)[number]['key'], items: TopologyNodeDatum[]): number {
  if (key === 'hosts') {
    return 1;
  }
  return new Set(items.map((node) => node.tier)).size || 1;
}

function buildGraph(
  nodeData: TopologyNodeDatum[],
  edgeData: TopologyEdgeDatum[],
  width: number,
  showLabels: boolean,
  switchView: boolean,
  showDetached: boolean
): { nodes: Node[]; edges: Edge[] } {
  const detachedHosts = showDetached ? nodeData.filter((node) => node.nodeType === 'host' && node.detached) : [];
  const itemsByBand: Record<(typeof BANDS)[number]['key'], TopologyNodeDatum[]> = {
    scaleout: nodeData.filter((node) => node.topology === 'scaleout' && node.nodeType === 'domain'),
    scaleup: nodeData.filter((node) => node.topology === 'scaleup' && node.nodeType === 'domain'),
    hosts: nodeData.filter((node) => node.nodeType === 'host' && !node.detached),
    storage: nodeData.filter((node) => node.topology === 'storage' && node.nodeType === 'domain'),
  };

  // Every band's height is a multiple of one tier row's height, so a band with more tiers (e.g. a
  // 2-tier scaleout) gets proportionally more space instead of a fixed fraction of the panel that
  // may not match how many tiers actually exist. The unit is storage's own row height, since
  // storage is always exactly one tier. Switch view's groups are a fixed height regardless of
  // member count (members lay out horizontally, not as a vertical list), unlike domainNodeHeight.
  const unitHeight = switchView
    ? GROUP_LABEL_HEIGHT + SWITCH_HEIGHT + GROUP_PADDING_BOTTOM
    : itemsByBand.storage.length > 0
      ? Math.max(...itemsByBand.storage.map(domainNodeHeight))
      : DOMAIN_BASE_HEIGHT;

  const nodes: Node[] = [];
  const positions: PositionMap = new Map();
  const minimumWidth = 16 + DOMAIN_WIDTH + DETACHED_SIDE_GAP + DETACHED_GROUP_WIDTH + 16;
  const graphWidth = detachedHosts.length > 0 ? Math.max(width, minimumWidth) : width;
  const topologyLeft = detachedHosts.length > 0 ? 16 + DETACHED_GROUP_WIDTH + DETACHED_SIDE_GAP : 16;
  const topologyRight = graphWidth - 16;
  let hostRowTop = 0;
  let offsetY = 0;

  for (const band of BANDS) {
    const items = itemsByBand[band.key];
    const tierCount = bandTierCount(band.key, items);
    // Hosts always render as fixed HOST_SIZE cards, never as switch-view groups or
    // domainNodeHeight-sized cards, so they must use their own real height here — reusing the
    // shared `unitHeight` (sized for groups/storage) left hosts' band ~25-45px taller than its
    // actual content needed, making gaps touching the host row look bigger than gaps between two
    // non-host bands (e.g. scaleout-to-scaleup) even though both use the same BAND_GAP.
    const rowUnitHeight = band.key === 'hosts' ? HOST_SIZE.height : unitHeight;
    const bandHeight = rowUnitHeight * tierCount + Math.max(0, tierCount - 1) * TIER_GAP + 16;

    if (items.length > 0) {
      const region = { x: topologyLeft, y: offsetY + 8, width: topologyRight - topologyLeft, height: bandHeight - 16 };

      if (band.key === 'scaleout' || band.key === 'scaleup') {
        nodes.push(...layoutVerticalDomains(items, region, false, showLabels, switchView, positions));
      } else if (band.key === 'storage') {
        nodes.push(...layoutVerticalDomains(items, region, true, showLabels, switchView, positions));
      } else {
        hostRowTop = region.y + (region.height - HOST_SIZE.height) / 2;
        nodes.push(...layoutHorizontalRow(items, region, showLabels, switchView, positions));
      }
    }

    offsetY += bandHeight + BAND_GAP;
  }

  if (detachedHosts.length > 0) {
    nodes.push(...buildDetachedGroupNodes(detachedHosts, 16, hostRowTop - DETACHED_GROUP_LABEL_HEIGHT, showLabels));
  }

  const nodeById = new Map(nodeData.map((node) => [node.id, node]));

  // In switch view, a domain id resolves to its switch-member node ids (falling back to the domain
  // id itself if it has no members, so an edge never ends up with nowhere to attach).
  function switchTargetsOf(domainId: string): string[] {
    const domain = nodeById.get(domainId);
    const members = domain ? switchMembersOf(domain) : [];
    return members.length > 0 ? members.map((member) => switchNodeId(domainId, member)) : [domainId];
  }

  function toEdge(id: string, source: string, target: string, topology?: string, zIndex = 1): Edge | undefined {
    const from = positions.get(source);
    const to = positions.get(target);
    if (!from || !to) {
      return undefined;
    }

    const goingUp = to.y <= from.y;
    return {
      id,
      source,
      target,
      sourceHandle: goingUp ? 'top-source' : 'bottom-source',
      targetHandle: goingUp ? 'bottom-target' : 'top-target',
      type: 'straight',
      zIndex,
      data: { topology },
    };
  }

  const edges: Edge[] = edgeData.flatMap((edge) => {
    // The target is always the domain side for both edge kinds (host -> domain "member" edges,
    // domain -> parent domain "parent" edges), so its topology is the edge's topology; the source
    // fallback only matters if that ever stops being true.
    const topology = nodeById.get(edge.target)?.topology || nodeById.get(edge.source)?.topology;

    // host<->scaleout edges have to cross through the scaleup band spatially (band order is
    // scaleout, scaleup, hosts, storage - scaleup sits directly between scaleout and hosts), so
    // they're drawn at a distinctly lower zIndex than every other edge to sit behind the scaleup
    // group/card rather than visually crossing over it.
    const crossesScaleup =
      topology === 'scaleout' &&
      (nodeById.get(edge.source)?.nodeType === 'host' || nodeById.get(edge.target)?.nodeType === 'host');
    const zIndex = crossesScaleup ? -1 : 1;

    if (!switchView) {
      const built = toEdge(edge.id, edge.source, edge.target, topology, zIndex);
      return built ? [built] : [];
    }

    // Real per-switch link data isn't part of the Topology CRD contract (only "this domain's switch
    // members" and "this domain's parent domain" are known), so switch view fans every edge out to
    // every (source switch, target switch) pair instead of picking just one - e.g. a host with one
    // NIC per leaf (confirmed via Switch.status.lldpNeighbors: every leaf in a tier reports every
    // host in that tier as a neighbor) really does connect to every switch in its domain, not just
    // one, and a real leaf-spine fabric normally wires every leaf to every spine the same way.
    const sourceSwitches = switchTargetsOf(edge.source);
    const targetSwitches = switchTargetsOf(edge.target);

    return sourceSwitches.flatMap((sourceId) =>
      targetSwitches.flatMap((targetId) => {
        const built = toEdge(`${edge.id}::${sourceId}->${targetId}`, sourceId, targetId, topology, zIndex);
        return built ? [built] : [];
      })
    );
  });

  return { nodes, edges };
}

// Duck-typed view of unifabric-unifabrictopology-datasource's DataSource class (a separately
// bundled plugin, so it can't be imported directly) — just enough surface to fetch one CRD by
// kind+name, reusing that datasource's own Grafana backend API client instead of hand-building
// fetch calls here.
interface ResourceDataSourceApi {
  getResource<T>(cluster: string, kind: string, name: string): Promise<T>;
}

// Resolves which resource (if any) a hovered node represents, and the topology accent color that
// matches its own card when resource-type colorization is on. switchMember -> Switch CRD, host ->
// FabricNode CRD (both fetched on demand, see the panel's hover-fetch effect); domain/switchGroup
// both represent one TopologyDomain, whose fields already arrived with the main topology query, so
// they resolve straight from `data` with no fetch at all.
function resolveHoverTarget(
  node: Node<BoxNodeData>,
  colorized: boolean
): { target: HoverTarget; accentColor?: string } | undefined {
  const accentColor = colorized && node.data.topology ? TOPOLOGY_COLORS[node.data.topology] : undefined;

  if (node.type === 'host' && node.data.cluster) {
    return { target: { kind: 'host', cluster: node.data.cluster, name: node.data.label }, accentColor };
  }
  if (node.type === 'switchMember' && node.data.cluster) {
    return { target: { kind: 'switch', cluster: node.data.cluster, name: node.data.label }, accentColor };
  }
  if (node.type === 'domain' || node.type === 'switchGroup') {
    return {
      target: {
        kind: 'switchGroup',
        name: node.data.label,
        topologyLabel: node.data.topology ? topologyLabel(node.data.topology) : undefined,
        tier: node.data.tier,
        parent: node.data.parent,
        members: node.data.members ? node.data.members.split(', ').filter(Boolean) : [],
      },
      accentColor,
    };
  }
  return undefined;
}

export const TopologyPanel: React.FC<Props> = ({ options, data, width, height, fieldConfig, id }) => {
  const theme = useTheme2();
  const styles = useStyles2(getStyles);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [colorized, setColorized] = useState(true);
  const [switchView, setSwitchView] = useState(false);
  const [showDetached, setShowDetached] = useState(true);

  // Hover-card state: `hover` identifies the currently-hovered node (undefined = nothing hovered)
  // plus its current fetch `entry`, if any. `resourceCache` memoizes fetch results across separate
  // hover sessions (keyed by resourceKey(target)) so re-hovering an already-fetched node reuses it
  // instead of firing a new request. It's a plain ref, read only from event handlers/effects, never
  // during render (react-hooks/refs) — `hover.entry` is what render actually reads. Keying the cache
  // by resource identity (rather than one "latest result" slot) is what makes this race-safe: a slow
  // response for a node the user already moved away from can only ever write its OWN cache entry,
  // and only ever updates `hover.entry` if `hover` still points at that same target.
  const [hover, setHover] = useState<
    { target: HoverTarget; accentColor?: string; anchor: HTMLElement; entry?: ResourceEntry } | undefined
  >();
  const resourceCache = useRef<Map<string, ResourceEntry>>(new Map());
  const dataSourceRef = useRef<Promise<ResourceDataSourceApi | undefined> | null>(null);
  const hoverHideTimeoutRef = useRef<number | null>(null);

  const clearHoverHideTimeout = () => {
    if (hoverHideTimeoutRef.current !== null) {
      window.clearTimeout(hoverHideTimeoutRef.current);
      hoverHideTimeoutRef.current = null;
    }
  };

  const scheduleHoverHide = () => {
    clearHoverHideTimeout();
    hoverHideTimeoutRef.current = window.setTimeout(() => setHover(undefined), 150);
  };

  // Clears any pending hide on unmount, so it never fires setHover after the panel is gone.
  useEffect(() => clearHoverHideTimeout, []);

  const graphData = useMemo(() => toGraphData(data.series), [data.series]);
  const nodeData = graphData?.nodes ?? EMPTY_NODE_DATA;
  const edgeData = graphData?.edges ?? EMPTY_EDGE_DATA;

  const layout = useMemo(
    () => buildGraph(nodeData, edgeData, width, options.showLabels, switchView, showDetached),
    [nodeData, edgeData, width, options.showLabels, switchView, showDetached]
  );

  // Node positions/dragging are owned by React Flow itself (useNodesState + its onNodesChange
  // applies changes internally); we only resync from the computed layout when the underlying data,
  // size or label option actually changes (detected by comparing against the last-synced layout
  // during render, React's recommended alternative to an effect), so dragging a card never fights
  // with re-renders and selecting another node doesn't snap dragged cards back.
  const [nodes, setNodes, onNodesChange] = useNodesState<BoxNodeData>(layout.nodes);
  const [edges, setEdges] = useState<Edge[]>(layout.edges);
  const [syncedLayout, setSyncedLayout] = useState(layout);

  if (layout !== syncedLayout) {
    setSyncedLayout(layout);
    setNodes(layout.nodes);
    setEdges(layout.edges);
    // The layout's node set is being rebuilt (e.g. switch view toggled), so any DOM element a hover
    // card is currently anchored to may no longer be attached once this render commits.
    setHover(undefined);
  }

  const displayNodes = useMemo(
    () =>
      nodes.map((node) => ({
        ...node,
        data: { ...node.data, selected: node.id === selectedId, colorized },
      })),
    [nodes, selectedId, colorized]
  );

  const displayEdges = useMemo(
    () =>
      edges.map((edge) => {
        const connected = selectedId !== null && (edge.source === selectedId || edge.target === selectedId);
        const topology = edge.data?.topology as string | undefined;
        const accentColor = colorized && topology ? TOPOLOGY_COLORS[topology] : undefined;
        const stroke = accentColor
          ? colorManipulator.alpha(accentColor, connected ? 1 : 0.35)
          : connected
            ? theme.colors.primary.main
            : colorManipulator.alpha(theme.colors.text.primary, 0.3);
        return {
          ...edge,
          style: { stroke, strokeWidth: 1.75 },
        };
      }),
    [edges, selectedId, theme, colorized]
  );

  const handleNodeClick: NodeMouseHandler = (_, node) => {
    // The switch-view group background isn't an independently selectable resource (edges attach to
    // its individual switch members, not to the group), so clicking it is a no-op.
    if (node.type === 'switchGroup' || node.type === 'detachedGroup') {
      return;
    }
    setSelectedId((current) => (current === node.id ? null : node.id));
  };

  const handleNodeMouseEnter: NodeMouseHandler = (event, node) => {
    clearHoverHideTimeout();
    const resolved = resolveHoverTarget(node as Node<BoxNodeData>, colorized);
    if (!resolved) {
      return;
    }
    // Hydrate immediately from the cache if it's still fresh (reading a ref here is fine — this
    // runs from an event handler, not during render). A stale hit is dropped so the effect below
    // re-fetches instead of showing minutes-old data.
    const cached = resolved.target.kind !== 'switchGroup' ? resourceCache.current.get(resourceKey(resolved.target)) : undefined;
    const entry = isEntryFresh(cached) ? cached : undefined;
    setHover({ target: resolved.target, accentColor: resolved.accentColor, anchor: event.currentTarget as HTMLElement, entry });
  };

  const handleNodeMouseLeave: NodeMouseHandler = () => {
    // Delayed rather than immediate: the popover renders through a Portal elsewhere in the DOM, so
    // moving the mouse from the node onto the popover itself (e.g. to select text or click a link) always
    // passes through a brief moment of hovering neither element. clearHoverHideTimeout (wired to the
    // popover's own onMouseEnter below) cancels this if the mouse lands back on either.
    scheduleHoverHide();
  };

  // Only used as this effect's dependency: a stable primitive (rather than the whole `hover`
  // object) that changes exactly when the hovered *resource* changes, not when this same effect's
  // own setHover(...entry) calls update `hover.entry` for the resource already being hovered.
  const hoverKey = hover && hover.target.kind !== 'switchGroup' ? resourceKey(hover.target) : undefined;

  // Fetches the Switch/FabricNode CRD behind the currently-hovered node, skipping the request when
  // a still-fresh (see RESOURCE_CACHE_TTL_MS) result is already cached (switchGroup targets never
  // reach here, see resolveHoverTarget and handleNodeMouseEnter, which hydrates from cache
  // synchronously before this effect even runs).
  useEffect(() => {
    if (!hover || hover.target.kind === 'switchGroup') {
      return;
    }
    const target = hover.target;
    const key = resourceKey(target);
    if (isEntryFresh(resourceCache.current.get(key))) {
      return;
    }

    let cancelled = false;

    // Writes both the long-lived ref cache (survives this hover ending) and, if this exact target
    // is still what's hovered, the state React actually renders from.
    const commit = (entry: ResourceEntry) => {
      resourceCache.current.set(key, entry);
      if (!cancelled) {
        setHover((prev) => (prev && prev.target === target ? { ...prev, entry } : prev));
      }
    };

    (async () => {
      commit({ status: 'loading' });

      if (!dataSourceRef.current) {
        const ref = data.request?.targets?.[0]?.datasource ?? undefined;
        dataSourceRef.current = ref
          ? getDataSourceSrv()
              .get(ref)
              .then((instance) => instance as unknown as ResourceDataSourceApi)
              .catch(() => undefined)
          : Promise.resolve(undefined);
      }

      const ds = await dataSourceRef.current;
      if (cancelled) {
        return;
      }
      if (!ds) {
        commit({ status: 'error', message: 'Datasource unavailable', fetchedAt: Date.now() });
        return;
      }

      try {
        const kind = target.kind === 'switch' ? 'switch' : 'fabricnode';
        const resourceData = await ds.getResource(target.cluster, kind, target.name);
        if (!cancelled) {
          commit({ status: 'success', data: resourceData, fetchedAt: Date.now() });
        }
      } catch (err) {
        if (!cancelled) {
          const notFound = isFetchError(err) && err.status === 404;
          commit({
            status: 'error',
            notFound,
            message: notFound ? undefined : err instanceof Error ? err.message : 'Request failed',
            fetchedAt: Date.now(),
          });
        }
      }
    })();

    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hoverKey, data.request]);

  const panelClassName = cx(
    styles.wrapper,
    css`
      width: ${width}px;
      height: ${height}px;
    `
  );

  if (!graphData) {
    return <PanelDataErrorView fieldConfig={fieldConfig} panelId={id} data={data} needsStringField />;
  }

  if (nodeData.length === 0) {
    return (
      <div className={panelClassName}>
        <EmptyTopologyState />
        <DetachedNodesMenu enabled={showDetached} onEnabledChange={setShowDetached} />
        <SwitchViewMenu enabled={switchView} onEnabledChange={setSwitchView} />
        <ColorizationMenu enabled={colorized} onEnabledChange={setColorized} />
      </div>
    );
  }

  return (
    <div className={panelClassName}>
      <ReactFlow
        nodes={displayNodes}
        edges={displayEdges}
        nodeTypes={NODE_TYPES}
        fitView
        fitViewOptions={{ padding: 0.15 }}
        minZoom={0.1}
        nodesDraggable
        nodesConnectable={false}
        onNodesChange={onNodesChange}
        onNodeClick={handleNodeClick}
        onNodeMouseEnter={handleNodeMouseEnter}
        onNodeMouseLeave={handleNodeMouseLeave}
        onPaneClick={() => setSelectedId(null)}
        proOptions={{ hideAttribution: true }}
      >
        <Background />
        <Controls showInteractive={false} />
      </ReactFlow>
      <DetachedNodesMenu enabled={showDetached} onEnabledChange={setShowDetached} />
      <SwitchViewMenu enabled={switchView} onEnabledChange={setSwitchView} />
      <ColorizationMenu enabled={colorized} onEnabledChange={setColorized} />
      {hover && (
        <Popover
          show
          renderArrow
          referenceElement={hover.anchor}
          placement="right-start"
          onMouseEnter={clearHoverHideTimeout}
          onMouseLeave={scheduleHoverHide}
          // Popover's own FloatingArrow always fills with theme.colors.border.weak, not the card's
          // own background — recolor just the arrow (direct-child svg, not the card's own nested
          // lucide icons) so it reads as one continuous shape with the card instead of a mismatched
          // gray wedge.
          className={css`
            > svg path {
              fill: ${hoverCardBackground(theme)};
            }
          `}
          content={<ResourceHoverCard target={hover.target} entry={hover.entry} accentColor={hover.accentColor} />}
        />
      )}
    </div>
  );
};
