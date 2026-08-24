import { test, expect } from '@grafana/plugin-e2e';
import { MyDataSourceOptions, MySecureJsonData } from '../src/types';

test('smoke: should render config editor', async ({ createDataSourceConfigPage, readProvisionedDataSource, page }) => {
  const ds = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  await createDataSourceConfigPage({ type: ds.type });
  await expect(page.getByLabel('API Base URL')).toBeVisible();
});

test('"Save & test" should be successful when configuration is valid', async ({
  createDataSourceConfigPage,
  readProvisionedDataSource,
  page,
}) => {
  const ds = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  // testDatasource() calls GET /clusters as a real connectivity check; mock it so
  // "Save & test" succeeds without a real backend running.
  await page.route('**/clusters', (route) =>
    route.fulfill({ status: 200, body: JSON.stringify([{ name: 'default' }]) })
  );
  const configPage = await createDataSourceConfigPage({ type: ds.type });
  // This datasource has no backend component, so "Save & test" never calls the default
  // /health endpoint saveAndTest() waits for; point it at the /clusters call it actually makes.
  await expect(configPage.saveAndTest({ path: 'clusters' })).toBeOK();
});
