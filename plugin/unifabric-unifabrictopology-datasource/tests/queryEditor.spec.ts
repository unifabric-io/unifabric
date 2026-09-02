import { test, expect } from '@grafana/plugin-e2e';

test('smoke: should render query editor', async ({ panelEditPage, readProvisionedDataSource }) => {
  const ds = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  await panelEditPage.datasource.set(ds.name);
  await expect(panelEditPage.getQueryEditorRow('A').getByText(/scaleout.*scaleup.*storage/i)).toBeVisible();
});

test('data query should return native resource list frames', async ({
  panelEditPage,
  readProvisionedDataSource,
  page,
}) => {
  const ds = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  page.on('console', (msg) => {
    // eslint-disable-next-line no-console
    console.log(`[diag] console.${msg.type()}: ${msg.text()}`);
  });
  page.on('pageerror', (err) => {
    // eslint-disable-next-line no-console
    console.log(`[diag] pageerror: ${err.message}`);
  });
  page.on('request', (request) => {
    if (/clusters|topology/.test(request.url())) {
      // eslint-disable-next-line no-console
      console.log(`[diag] request: ${request.method()} ${request.url()}`);
    }
  });
  page.on('response', (response) => {
    if (/clusters|topology/.test(response.url())) {
      // eslint-disable-next-line no-console
      console.log(`[diag] response: ${response.status()} ${response.url()}`);
    }
  });
  await page.route('**/clusters', (route) =>
    route.fulfill({ status: 200, body: JSON.stringify([{ name: 'default' }]) })
  );
  await page.route('**/clusters/*/apis/unifabric.io/v1beta1/topology', async (route) =>
    route.fulfill({
      status: 200,
      body: JSON.stringify({
        apiVersion: 'unifabric.io/v1beta1',
        kind: 'TopologyList',
        items: [{ metadata: { name: 'scaleout' }, status: { domains: [{ name: 'tier1-group1', tier: 1 }] } }],
      }),
    })
  );
  await page.route('**/clusters/*/apis/unifabric.io/v1beta1/fabricnode', async (route) =>
    route.fulfill({
      status: 200,
      body: JSON.stringify({
        apiVersion: 'unifabric.io/v1beta1',
        kind: 'FabricNodeList',
        items: [{ metadata: { name: 'node-outside-topology' } }],
      }),
    })
  );
  await panelEditPage.datasource.set(ds.name);
  await panelEditPage.setVisualization('Table');
  await panelEditPage.panel.scrollIntoView();
  // eslint-disable-next-line no-console
  console.log(`[diag] panel text: ${JSON.stringify(await panelEditPage.panel.locator.innerText())}`);
  await expect(panelEditPage.panel.fieldNames).toContainText(['resource', 'cluster']);
  await expect(panelEditPage.panel.locator).toContainText('TopologyList');
});
