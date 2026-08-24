import { test, expect } from '@grafana/plugin-e2e';

test('should display "No data" in case panel data is empty', async ({
  gotoPanelEditPage,
  readProvisionedDashboard,
}) => {
  const dashboard = await readProvisionedDashboard({ fileName: 'dashboard.json' });
  const panelEditPage = await gotoPanelEditPage({ dashboard, id: '2' });
  await expect(panelEditPage.panel.locator).toContainText('No data');
});

test('should display an empty topology state when topology data is an empty array', async ({
  gotoPanelEditPage,
  readProvisionedDashboard,
}) => {
  const dashboard = await readProvisionedDashboard({ fileName: 'dashboard.json' });
  const panelEditPage = await gotoPanelEditPage({ dashboard, id: '3' });
  const panel = panelEditPage.panel.locator;

  await expect(panel.getByTestId('topology-panel-empty-state')).toBeVisible();
  await expect(panel).toContainText('No topology information');
  await expect(panel).not.toContainText('No data');
});

test('should display topology nodes when data is passed to the panel', async ({
  gotoPanelEditPage,
  readProvisionedDashboard,
  page,
}) => {
  const dashboard = await readProvisionedDashboard({ fileName: 'dashboard.json' });
  const panelEditPage = await gotoPanelEditPage({ dashboard, id: '1' });
  await expect(page.getByTestId('topology-panel-node').first()).toBeVisible();
});

test('should hide labels when "Show labels" option is disabled', async ({ gotoPanelEditPage, readProvisionedDashboard }) => {
  const dashboard = await readProvisionedDashboard({ fileName: 'dashboard.json' });
  const panelEditPage = await gotoPanelEditPage({ dashboard, id: '1' });
  const options = panelEditPage.getCustomOptions('Unifabric-Topology');
  const showLabels = options.getSwitch('Show labels');

  await showLabels.uncheck();
  // Scoped to the panel's own container: a page-wide getByText also matches an
  // unrelated JSON/query-inspector view that renders the same domain name as data.
  await expect(panelEditPage.panel.locator.getByText('tier2-group1')).not.toBeVisible();
});

test('should toggle topology colorization and show its color legend', async ({
  gotoPanelEditPage,
  readProvisionedDashboard,
  page,
}) => {
  const dashboard = await readProvisionedDashboard({ fileName: 'dashboard.json' });
  const panelEditPage = await gotoPanelEditPage({ dashboard, id: '1' });
  const panel = panelEditPage.panel.locator;
  const colorButton = panel.getByRole('button', { name: 'Toggle topology colors' });
  const colorTooltip = panel.getByTestId('topology-color-tooltip');

  await expect(colorButton).toHaveAttribute('aria-pressed', 'true');
  await colorButton.hover();

  await expect(colorTooltip).toBeVisible();
  await expect(colorTooltip).toContainText('Storage');
  await expect(colorTooltip).toContainText('Scale Out');
  await expect(colorTooltip).toContainText('Scale Up');

  await colorButton.click();

  await expect(colorButton).toHaveAttribute('aria-pressed', 'false');
  await colorButton.click();
  await expect(colorButton).toHaveAttribute('aria-pressed', 'true');

  const coloredDomain = panel.getByTestId('topology-panel-node').filter({ hasText: 'tier2-group1' }).first();
  await coloredDomain.click();
  // cardStyles applies a 120ms border-color/box-shadow transition on selection - wait for it to
  // settle before reading computed style, or this can catch a mid-transition color value.
  await page.waitForTimeout(200);

  const domainSelectionStyle = await coloredDomain.evaluate((element) => {
    const style = getComputedStyle(element);
    return { borderColor: style.borderColor, boxShadow: style.boxShadow };
  });
  // Browser-dependent serialization: some engines report an opaque color as `rgb(r, g, b)`,
  // others as `rgba(r, g, b, 1)` - match on the channel values only, not the exact prefix.
  expect(domainSelectionStyle.borderColor).toContain('50, 140, 230');
  expect(domainSelectionStyle.boxShadow).toContain('2px');

  const host = panel.getByTestId('topology-panel-node').filter({ hasText: 'node1' }).first();
  await host.click();

  const hostSelectionStyle = await host.evaluate((element) => getComputedStyle(element).boxShadow);
  expect(hostSelectionStyle).not.toContain('4px');
});

test('should toggle switch view, rendering domains as groups of individual switch boxes', async ({
  gotoPanelEditPage,
  readProvisionedDashboard,
}) => {
  const dashboard = await readProvisionedDashboard({ fileName: 'dashboard.json' });
  const panelEditPage = await gotoPanelEditPage({ dashboard, id: '1' });
  const panel = panelEditPage.panel.locator;
  const switchViewButton = panel.getByRole('button', { name: 'Toggle switch view' });

  await expect(switchViewButton).toHaveAttribute('aria-pressed', 'false');
  await expect(panel.getByTestId('topology-switch-group')).toHaveCount(0);

  await switchViewButton.click();

  await expect(switchViewButton).toHaveAttribute('aria-pressed', 'true');
  await expect(panel.getByTestId('topology-switch-group').first()).toBeVisible();
  await expect(panel.getByText('leaf1', { exact: true })).toBeVisible();
  await expect(panel.getByText('leaf2', { exact: true })).toBeVisible();

  const leaf1 = panel.getByTestId('topology-panel-node').filter({ hasText: 'leaf1' }).first();
  await leaf1.click();
  const leaf1Style = await leaf1.evaluate((element) => getComputedStyle(element).boxShadow);
  expect(leaf1Style).not.toBe('none');

  await switchViewButton.click();

  await expect(switchViewButton).toHaveAttribute('aria-pressed', 'false');
  await expect(panel.getByTestId('topology-switch-group')).toHaveCount(0);
});
