import { BackendSrvRequest, getBackendSrv, getTemplateSrv, isFetchError } from '@grafana/runtime';
import {
  DataQueryRequest,
  DataQueryResponse,
  DataSourceApi,
  DataSourceInstanceSettings,
  DataFrame,
  createDataFrame,
  FieldType,
  MetricFindValue,
} from '@grafana/data';

import {
  FabricNodeList,
  MyQuery,
  MyDataSourceOptions,
  TopologyCluster,
  TopologyList,
} from './types';
import { lastValueFrom } from 'rxjs';

export class DataSource extends DataSourceApi<MyQuery, MyDataSourceOptions> {
  baseUrl: string;

  constructor(instanceSettings: DataSourceInstanceSettings<MyDataSourceOptions>) {
    super(instanceSettings);
    this.baseUrl = instanceSettings.url!;
  }

  async query(options: DataQueryRequest<MyQuery>): Promise<DataQueryResponse> {
    const resolvedTargets = options.targets.map((target) => ({
      ...target,
      cluster: target.cluster ? getTemplateSrv().replace(target.cluster, options.scopedVars) : undefined,
    }));
    const needsDefaultCluster = resolvedTargets.some((target) => !target.cluster);
    const defaultCluster = needsDefaultCluster ? await this.getDefaultCluster() : undefined;

    const data = (
      await Promise.all(
        resolvedTargets.map(async (target) => {
          const cluster = target.cluster || defaultCluster || 'default';
          const [topologyList, fabricNodeList] = await Promise.all([
            requestList(
              () =>
              this.request<TopologyList>(
                `/clusters/${encodeURIComponent(cluster)}/apis/unifabric.io/v1beta1/topology`,
                undefined,
                { showErrorAlert: false }
              ),
              { items: [] }
            ),
            requestList(
              () =>
              this.request<FabricNodeList>(
                `/clusters/${encodeURIComponent(cluster)}/apis/unifabric.io/v1beta1/fabricnode`,
                undefined,
                { showErrorAlert: false }
              ),
              { items: [] }
            ),
          ]);
          return resourceListsToDataFrames(target.refId, topologyList, fabricNodeList, cluster);
        })
      )
    ).flat();

    return { data };
  }

  async request<T>(url: string, params?: string, options?: Partial<BackendSrvRequest>): Promise<T> {
    const response = getBackendSrv().fetch<T>({
      url: `${this.baseUrl}${url}${params?.length ? `?${params}` : ''}`,
      ...options,
    });
    const result = await lastValueFrom(response);
    return result.data;
  }

  /** Lists clusters available for the "Cluster" query editor dropdown. */
  async getClusters(): Promise<TopologyCluster[]> {
    return this.request<TopologyCluster[]>('/clusters');
  }

  /**
   * Fetches a single cluster-scoped resource by its (singular, lower-case) kind and name, e.g.
   * `getResource(cluster, 'switch', 'leaf1')`. Backs panel-side "hover for details" UI so panels
   * don't have to hand-build `/clusters/{cluster}/apis/...` paths themselves. 404s/other errors are
   * left to the caller (not swallowed here) and reported without Grafana's global error toast,
   * since a missing/failed lookup for an optional hover card isn't an application-level error.
   */
  async getResource<T>(cluster: string, kind: string, name: string): Promise<T> {
    return this.request<T>(
      `/clusters/${encodeURIComponent(cluster)}/apis/unifabric.io/v1beta1/${kind}/${encodeURIComponent(name)}`,
      undefined,
      { showErrorAlert: false }
    );
  }

  /** Powers a dashboard "Query" template variable (e.g. `$cluster`) that lists clusters. */
  async metricFindQuery(): Promise<MetricFindValue[]> {
    const clusters = await this.getClusters();
    return clusters.map((cluster) => ({ text: cluster.name, value: cluster.name }));
  }

  private async getDefaultCluster(): Promise<string> {
    const clusters = await this.getClusters();
    return clusters[0]?.name ?? 'default';
  }

  /** Checks whether we can connect to the API, using GET /clusters as a basic connectivity check. */
  async testDatasource() {
    try {
      await this.getClusters();
      return {
        status: 'success',
        message: 'Success',
      };
    } catch (err) {
      return {
        status: 'error',
        message: err instanceof Error ? err.message : 'Failed to reach the Unifabric topology API',
      };
    }
  }
}

async function requestList<T>(request: () => Promise<T>, emptyList: T): Promise<T> {
  try {
    return await request();
  } catch (err) {
    if (isFetchError(err) && err.status === 404) {
      return emptyList;
    }
    throw err;
  }
}

/**
 * Wraps each API response unchanged in a one-row DataFrame. Grafana requires query results to be
 * DataFrames, while the panel owns interpretation of the native Kubernetes resource lists.
 */
function resourceListsToDataFrames(
  refId: string,
  topologyList: TopologyList,
  fabricNodeList: FabricNodeList,
  cluster: string
): DataFrame[] {
  const topologiesFrame = createDataFrame({
    refId,
    name: 'topologies',
    fields: [
      { name: 'resource', type: FieldType.other, values: [topologyList] },
      { name: 'cluster', type: FieldType.string, values: [cluster] },
    ],
  });

  const fabricNodesFrame = createDataFrame({
    refId,
    name: 'fabricNodes',
    fields: [
      { name: 'resource', type: FieldType.other, values: [fabricNodeList] },
      { name: 'cluster', type: FieldType.string, values: [cluster] },
    ],
  });

  return [topologiesFrame, fabricNodesFrame];
}
