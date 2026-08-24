import React from 'react';
import { DataSourceHttpSettings } from '@grafana/ui';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { MyDataSourceOptions, MySecureJsonData } from '../types';

interface Props extends DataSourcePluginOptionsEditorProps<MyDataSourceOptions, MySecureJsonData> {}

export function ConfigEditor(props: Props) {
  const { onOptionsChange, options } = props;

  return (
    <DataSourceHttpSettings
      defaultUrl="http://localhost:8080"
      dataSourceConfig={options}
      onChange={onOptionsChange}
      showAccessOptions
      urlLabel="API Base URL"
    />
  );
}
