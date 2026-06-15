import { expect, test } from '@grafana/plugin-e2e';
import { Components } from '../../src/selectors';

const fields = Components.ConfigEditor;

test('provisioned datasource passes its health check', async ({ request, readProvisionedDataSource }) => {
  const ds = await readProvisionedDataSource({ fileName: 'questdb_questdb_datasource.yaml' });
  const health = await request.get(`/api/datasources/uid/${ds.uid}/health`);
  expect(health.ok()).toBeTruthy();
  expect(await health.json()).toMatchObject({ status: 'OK' });
});

test('config editor: a datasource configured through the UI passes Save & Test', async ({
  createDataSourceConfigPage,
  page,
}) => {
  const configPage = await createDataSourceConfigPage({ type: 'questdb-questdb-datasource' });

  await page.getByLabel(fields.ServerAddress.label).fill('grafana-questdb-server');
  await page.getByLabel(fields.ServerPort.label).fill('8812');
  await page.getByLabel(fields.Username.label).fill('admin');
  await page.getByLabel(fields.Password.label, { exact: true }).fill('quest');
  await page.getByLabel(fields.TlsMode.label).click();
  await page.getByRole('option', { name: 'disable' }).click();

  await expect(configPage.saveAndTest()).toBeOK();
});
