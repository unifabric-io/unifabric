import React from 'react';
import { colorManipulator, GrafanaTheme2 } from '@grafana/data';
import { Badge, BadgeColor, Spinner, useStyles2 } from '@grafana/ui';
import { css } from '@emotion/css';
import { AlertTriangle, Network, SearchX, Server } from 'lucide-react';

/**
 * The three hoverable resource kinds this panel supports. `switchGroup` covers both a collapsed
 * domain card (switch view off) and a switch-view group box — both represent one
 * `TopologyDomain` entry, not a standalone CRD, so it never needs a fetch (see below).
 */
export type HoverKind = 'switch' | 'host' | 'switchGroup';

interface FetchableHoverTarget {
  kind: 'switch' | 'host';
  cluster: string;
  name: string;
}

/** A domain (TopologyDomain), whose fields already arrived with the panel's main topology query. */
export interface SwitchGroupHoverTarget {
  kind: 'switchGroup';
  name: string;
  topologyLabel?: string;
  tier?: number;
  parent?: string;
  members: string[];
}

export type HoverTarget = FetchableHoverTarget | SwitchGroupHoverTarget;

/** Stable cache/lookup key for the fetchable kinds (switch, host). */
export function resourceKey(target: FetchableHoverTarget): string {
  return `${target.kind}:${target.cluster}:${target.name}`;
}

/** Fetch state for a switch/host CRD lookup, keyed by `resourceKey`. */
export type ResourceEntry =
  | { status: 'loading' }
  | { status: 'success'; data: unknown; fetchedAt: number }
  | { status: 'error'; notFound?: boolean; message?: string; fetchedAt: number };

/** How long a settled (success/error) fetch result is reused before a re-hover re-fetches it. */
export const RESOURCE_CACHE_TTL_MS = 5000;

/** An in-flight fetch is always reused (never double-fetch the same key); a settled one only
 * within RESOURCE_CACHE_TTL_MS of when it was fetched. */
export function isEntryFresh(entry: ResourceEntry | undefined): boolean {
  if (!entry) {
    return false;
  }
  return entry.status === 'loading' || Date.now() - entry.fetchedAt < RESOURCE_CACHE_TTL_MS;
}

// Subsets of pkg/api/v1beta1.Switch / .FabricNode actually rendered by this card. Kept local
// (rather than imported from the sibling datasource plugin, which is a separately bundled npm
// project) so this component has no build-time dependency on it — see datasource/src/types.ts for
// the fuller mirror of the same CRDs.
interface ResourceConditionLike {
  type: string;
  status?: string;
  reason?: string;
}

interface SwitchResourceLike {
  metadata?: { creationTimestamp?: string };
  spec?: { mgmtIP?: string; role?: string };
  status?: {
    hostname?: string;
    healthy?: boolean;
    conditions?: ResourceConditionLike[];
    lldpNeighborCount?: number;
  };
}

interface FabricNodeResourceLike {
  metadata?: { creationTimestamp?: string };
  status?: {
    conditions?: ResourceConditionLike[];
    nodeRole?: string;
    nodeIP?: string;
    topologies?: string[];
    scaleOutNics?: unknown[];
    storageNics?: unknown[];
    rdmaPods?: unknown[];
  };
}

interface FieldRow {
  label: string;
  value?: string;
  badge?: { text: string; color: BadgeColor };
}

const KIND_META: Record<HoverKind, { label: string; icon: React.ComponentType<{ size?: number | string }> }> = {
  switch: { label: 'Switch', icon: Network },
  host: { label: 'Host', icon: Server },
  switchGroup: { label: 'Switch Group', icon: Network },
};

function findCondition(conditions: ResourceConditionLike[] | undefined, type: string): ResourceConditionLike | undefined {
  return conditions?.find((condition) => condition.type === type);
}

function conditionBadge(condition: ResourceConditionLike | undefined): { text: string; color: BadgeColor } {
  if (!condition) {
    return { text: 'Unknown', color: 'darkgrey' };
  }
  if (condition.status === 'True') {
    return { text: condition.reason || 'True', color: 'green' };
  }
  if (condition.status === 'False') {
    return { text: condition.reason || 'False', color: 'red' };
  }
  return { text: condition.reason || 'Unknown', color: 'darkgrey' };
}

function healthyBadge(healthy: boolean | undefined): { text: string; color: BadgeColor } {
  if (healthy === true) {
    return { text: 'Healthy', color: 'green' };
  }
  if (healthy === false) {
    return { text: 'Unhealthy', color: 'red' };
  }
  return { text: 'Unknown', color: 'darkgrey' };
}

function roleBadgeColor(role: string | undefined): BadgeColor {
  if (role === 'ScaleOut') {
    return 'blue';
  }
  if (role === 'ScaleUp') {
    return 'green';
  }
  if (role === 'Storage') {
    return 'purple';
  }
  return 'darkgrey';
}

// Buckets to the coarsest unit that's still >= 1, e.g. "3d" rather than "72h" or an exact timestamp.
function formatAge(iso: string | undefined): string | undefined {
  if (!iso) {
    return undefined;
  }
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) {
    return undefined;
  }
  const minutes = Math.floor((Date.now() - then) / 60000);
  if (minutes < 1) {
    return 'just now';
  }
  if (minutes < 60) {
    return `${minutes}m`;
  }
  const hours = Math.floor(minutes / 60);
  if (hours < 24) {
    return `${hours}h`;
  }
  return `${Math.floor(hours / 24)}d`;
}

function buildSwitchRows(resource: SwitchResourceLike): FieldRow[] {
  const status = resource.status ?? {};
  const spec = resource.spec ?? {};

  return [
    { label: 'Role', value: spec.role, badge: spec.role ? { text: spec.role, color: roleBadgeColor(spec.role) } : undefined },
    { label: 'Healthy', badge: healthyBadge(status.healthy) },
    { label: 'Ready', badge: conditionBadge(findCondition(status.conditions, 'Ready')) },
    { label: 'Connected', badge: conditionBadge(findCondition(status.conditions, 'Connected')) },
    { label: 'Hostname', value: status.hostname },
    { label: 'Mgmt IP', value: spec.mgmtIP },
    { label: 'LLDP Neighbors', value: status.lldpNeighborCount != null ? String(status.lldpNeighborCount) : undefined },
    { label: 'Age', value: formatAge(resource.metadata?.creationTimestamp) },
  ];
}

function buildHostRows(resource: FabricNodeResourceLike): FieldRow[] {
  const status = resource.status ?? {};

  return [
    { label: 'Role', value: status.nodeRole, badge: status.nodeRole ? { text: status.nodeRole, color: 'blue' } : undefined },
    { label: 'Ready', badge: conditionBadge(findCondition(status.conditions, 'Ready')) },
    { label: 'LLDP Ready', badge: conditionBadge(findCondition(status.conditions, 'LLDPNeighborsReady')) },
    { label: 'Node IP', value: status.nodeIP },
    { label: 'Topologies', value: status.topologies?.length ? status.topologies.join(', ') : undefined },
    { label: 'Scale-Out NICs', value: status.scaleOutNics ? String(status.scaleOutNics.length) : undefined },
    { label: 'Storage NICs', value: status.storageNics ? String(status.storageNics.length) : undefined },
    { label: 'RDMA Pods', value: status.rdmaPods ? String(status.rdmaPods.length) : undefined },
    { label: 'Age', value: formatAge(resource.metadata?.creationTimestamp) },
  ];
}

function buildSwitchGroupRows(target: SwitchGroupHoverTarget): FieldRow[] {
  return [
    { label: 'Topology', value: target.topologyLabel },
    { label: 'Tier', value: target.tier != null ? String(target.tier) : undefined },
    { label: 'Parent', value: target.parent },
    {
      label: 'Switches',
      value: target.members.length ? `${target.members.length} · ${target.members.join(', ')}` : undefined,
    },
  ];
}

/** Shared with TopologyPanel.tsx so the Popover's arrow can be recolored to match the card. */
export function hoverCardBackground(theme: GrafanaTheme2): string {
  return colorManipulator.alpha(theme.colors.background.secondary, 0.92);
}

function getStyles(theme: GrafanaTheme2) {
  return {
    card: css`
      min-width: 220px;
      max-width: 300px;
      padding: 10px 12px;
      border: 1px solid ${theme.colors.border.weak};
      border-radius: 8px;
      background: ${hoverCardBackground(theme)};
      backdrop-filter: blur(10px);
      -webkit-backdrop-filter: blur(10px);
      box-shadow: ${theme.shadows.z2};
      font-size: 11px;
    `,
    header: css`
      display: flex;
      align-items: center;
      gap: 8px;
      margin-bottom: 8px;
    `,
    iconChip: css`
      display: flex;
      align-items: center;
      justify-content: center;
      flex: 0 0 auto;
      width: 24px;
      height: 24px;
      border-radius: 7px;
      background: ${theme.colors.background.primary};
      color: ${theme.colors.text.secondary};
    `,
    headerText: css`
      min-width: 0;
    `,
    kindLabel: css`
      font-size: 10px;
      font-weight: 600;
      color: ${theme.colors.text.secondary};
      text-transform: uppercase;
      letter-spacing: 0.02em;
    `,
    title: css`
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: 12px;
      font-weight: 600;
      color: ${theme.colors.text.primary};
    `,
    grid: css`
      display: grid;
      grid-template-columns: minmax(64px, auto) 1fr;
      column-gap: 10px;
      row-gap: 4px;
      align-items: start;
    `,
    label: css`
      color: ${theme.colors.text.secondary};
      white-space: nowrap;
      padding-top: 1px;
    `,
    value: css`
      color: ${theme.colors.text.primary};
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      min-width: 0;
    `,
    statusRow: css`
      display: flex;
      align-items: center;
      gap: 6px;
      color: ${theme.colors.text.secondary};
    `,
  };
}

function FieldGrid({ rows }: { rows: FieldRow[] }) {
  const styles = useStyles2(getStyles);
  return (
    <div className={styles.grid}>
      {rows.map((row) => (
        <React.Fragment key={row.label}>
          <div className={styles.label}>{row.label}</div>
          <div className={styles.value} title={row.badge ? undefined : row.value}>
            {row.badge ? <Badge text={row.badge.text} color={row.badge.color} /> : row.value ?? '—'}
          </div>
        </React.Fragment>
      ))}
    </div>
  );
}

export interface ResourceHoverCardProps {
  target: HoverTarget;
  /** Fetch state for `switch`/`host` targets. Ignored (and may be omitted) for `switchGroup`. */
  entry?: ResourceEntry;
  /** Matches the hovered node's own topology accent color when resource-type colorization is on. */
  accentColor?: string;
}

// Generic hover card for the three resource kinds this panel renders (Switch, Host/FabricNode,
// Switch Group/TopologyDomain): one shell (header + Loading/Empty/Error/field-grid body) fed by a
// per-kind field configuration, instead of three near-duplicate cards.
export function ResourceHoverCard({ target, entry, accentColor }: ResourceHoverCardProps) {
  const styles = useStyles2(getStyles);

  const meta = KIND_META[target.kind];
  const Icon = meta.icon;

  let body: React.ReactNode;

  if (target.kind === 'switchGroup') {
    body = <FieldGrid rows={buildSwitchGroupRows(target)} />;
  } else if (!entry || entry.status === 'loading') {
    body = (
      <div className={styles.statusRow}>
        <Spinner size={12} />
        <span>Loading {meta.label.toLowerCase()} details…</span>
      </div>
    );
  } else if (entry.status === 'error' && entry.notFound) {
    body = (
      <div className={styles.statusRow}>
        <SearchX size={14} />
        <span>
          {meta.label} &quot;{target.name}&quot; not found.
        </span>
      </div>
    );
  } else if (entry.status === 'error') {
    body = (
      <div className={styles.statusRow}>
        <AlertTriangle size={14} />
        <span>Failed to load {meta.label.toLowerCase()} details{entry.message ? `: ${entry.message}` : '.'}</span>
      </div>
    );
  } else {
    body = (
      <FieldGrid
        rows={target.kind === 'switch' ? buildSwitchRows(entry.data as SwitchResourceLike) : buildHostRows(entry.data as FabricNodeResourceLike)}
      />
    );
  }

  return (
    <div className={styles.card} data-testid="topology-resource-hover-card">
      <div className={styles.header}>
        <div className={styles.iconChip} style={accentColor ? { background: colorManipulator.alpha(accentColor, 0.16), color: accentColor } : undefined}>
          <Icon size={14} />
        </div>
        <div className={styles.headerText}>
          <div className={styles.kindLabel}>{meta.label}</div>
          <div className={styles.title} title={target.name}>
            {target.name}
          </div>
        </div>
      </div>
      {body}
    </div>
  );
}
