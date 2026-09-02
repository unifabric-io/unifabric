import React, { useEffect, useState } from 'react';
import { QueryEditorProps, SelectableValue } from '@grafana/data';
import { InlineField, Select } from '@grafana/ui';
import { DataSource } from '../datasource';
import { MyDataSourceOptions, MyQuery, TopologyCluster } from '../types';

type Props = QueryEditorProps<DataSource, MyQuery, MyDataSourceOptions>;

export function QueryEditor({ query, onChange, onRunQuery, datasource }: Props) {
  const [clusters, setClusters] = useState<TopologyCluster[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    datasource
      .getClusters()
      .then((items) => {
        if (cancelled) {
          return;
        }
        setClusters(items);
        if (!query.cluster && items.length > 0) {
          onChange({ ...query, cluster: items[0].name });
          onRunQuery();
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
    // Fetch the cluster list once when the editor mounts.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const options: Array<SelectableValue<string>> = clusters.map((c) => ({ label: c.name, value: c.name }));
  const value = options.find((option) => option.value === query.cluster) ?? null;

  const handleChange = (option: SelectableValue<string>) => {
    onChange({ ...query, cluster: option.value });
    onRunQuery();
  };

  return (
    <>
      <InlineField label="Cluster" tooltip="Cluster to show the topology for.">
        <Select options={options} value={value} onChange={handleChange} isLoading={loading} width={30} />
      </InlineField>
      <div>Shows the scaleout, scaleup, and storage topologies combined in one graph.</div>
    </>
  );
}
