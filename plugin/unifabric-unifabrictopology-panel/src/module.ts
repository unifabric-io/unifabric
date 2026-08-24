import { PanelPlugin } from '@grafana/data';
import { TopologyPanelOptions } from './types';
import { TopologyPanel } from './components/TopologyPanel';

export const plugin = new PanelPlugin<TopologyPanelOptions>(TopologyPanel).setPanelOptions((builder) => {
  return builder.addBooleanSwitch({
    path: 'showLabels',
    name: 'Show labels',
    description: 'Show node names next to each device',
    defaultValue: true,
  });
});
