import { APIRequestContext, Response } from '@playwright/test';
import { expect, test } from '@grafana/plugin-e2e';

const QUESTDB_HTTP = process.env.QUESTDB_HTTP_URL ?? 'http://localhost:9000';

// The same env that provisions the datasource (threaded through docker compose into
// provisioning/datasources) tells this suite which path to assert: prepared
// statements on (the default) must bind, the opt-out must inline literals. Each CI
// matrix leg asserts its path unconditionally — a silently dead bind path cannot pass.
const expectBindParams = (process.env.QDB_DISABLE_PREPARED_STATEMENTS || 'false') !== 'true';

// seedTradesTable (re)creates the table the provisioned e2e dashboard queries, with
// rows at 11:00, 12:00, 13:00 and 14:00 UTC on 2024-01-20. The dashboard's window
// (11:30-13:30) selects exactly two of them.
async function seedTradesTable(request: APIRequestContext) {
  const statements = [
    'DROP TABLE IF EXISTS e2e_trades',
    'CREATE TABLE e2e_trades (ts timestamp, val long) TIMESTAMP(ts) PARTITION BY DAY BYPASS WAL',
    "INSERT INTO e2e_trades VALUES ('2024-01-20T11:00:00.000000Z', 1)," +
      "('2024-01-20T12:00:00.000000Z', 2)," +
      "('2024-01-20T13:00:00.000000Z', 3)," +
      "('2024-01-20T14:00:00.000000Z', 4)",
  ];
  for (const query of statements) {
    const res = await request.get(`${QUESTDB_HTTP}/exec`, { params: { query } });
    expect(res.ok(), `seeding QuestDB failed for: ${query}`).toBeTruthy();
  }
}

function frameOf(body: any) {
  const frames = body?.results?.A?.frames;
  expect(frames, 'expected a data frame for query A').toBeTruthy();
  expect(frames.length).toBeGreaterThan(0);
  return frames[0];
}

async function matchesPanelQuery(
  response: Response,
  expected: { executedIncludes: string; rowCount?: number; firstValue?: number }
) {
  if (!response.url().includes('/api/ds/query') || !response.ok()) {
    return false;
  }

  const body = await response.json().catch(() => undefined);
  const frame = body?.results?.A?.frames?.[0];
  const executed = frame?.schema?.meta?.executedQueryString;
  if (typeof executed !== 'string' || !executed.includes(expected.executedIncludes)) {
    return false;
  }

  const values = frame?.data?.values?.[0];
  if (expected.rowCount !== undefined && values?.length !== expected.rowCount) {
    return false;
  }
  if (expected.firstValue !== undefined && values?.[0] !== expected.firstValue) {
    return false;
  }
  return true;
}

test('time-bound macros are executed as bind parameters and return the right rows', async ({
  request,
  readProvisionedDashboard,
  gotoPanelEditPage,
}) => {
  await seedTradesTable(request);

  const dashboard = await readProvisionedDashboard({ fileName: 'e2e-timefilter.json' });
  const panelEditPage = await gotoPanelEditPage({ dashboard: { uid: dashboard.uid }, id: '1' });

  // Set the window explicitly rather than relying on the provisioned dashboard's
  // default time range staying in sync with the seed data: 11:30-13:30 -> the 12:00
  // and 13:00 rows.
  await panelEditPage.timeRange.set({ from: '2024-01-20 11:30:00', to: '2024-01-20 13:30:00' });
  const narrowResponse = await panelEditPage.refreshPanel({
    waitForResponsePredicateCallback: (response) =>
      matchesPanelQuery(response, { executedIncludes: 'SELECT ts, val FROM e2e_trades', rowCount: 2 }),
  });
  expect(narrowResponse.ok()).toBeTruthy();
  const narrowFrame = frameOf(await narrowResponse.json());

  // With prepared statements on (the default) the plugin must send placeholders; with
  // the datasource opt-out (provisioned for old servers like 8.0.3, which reject bind
  // parameters) it must inline literals. Both must return the right rows; the
  // byte-stability property is asserted on the parameterized path.
  const executedNarrow: string = narrowFrame.schema.meta.executedQueryString;
  if (expectBindParams) {
    expect(executedNarrow).toContain('cast($1 as timestamp)');
    expect(executedNarrow).toContain('cast($2 as timestamp)');
  } else {
    expect(executedNarrow).toMatch(/cast\(\d+ as timestamp\)/);
    expect(executedNarrow).not.toContain('$1');
  }
  expect(executedNarrow).not.toContain('__qdbTimeParam');
  expect(narrowFrame.data.values[0]).toHaveLength(2);

  await expect(panelEditPage.panel.getErrorIcon()).not.toBeVisible();

  // Widen the window to 10:30-14:30 -> all 4 rows; on the parameterized path the SQL
  // text must be byte-identical to the narrow window's (the plan-cache property).
  await panelEditPage.timeRange.set({ from: '2024-01-20 10:30:00', to: '2024-01-20 14:30:00' });
  const wideResponse = await panelEditPage.refreshPanel({
    waitForResponsePredicateCallback: (response) =>
      matchesPanelQuery(response, { executedIncludes: 'SELECT ts, val FROM e2e_trades', rowCount: 4 }),
  });
  expect(wideResponse.ok()).toBeTruthy();
  const wideFrame = frameOf(await wideResponse.json());

  if (expectBindParams) {
    expect(wideFrame.schema.meta.executedQueryString).toEqual(executedNarrow);
  }
  expect(wideFrame.data.values[0]).toHaveLength(4);
});

test('multi-statement query typed in the SQL editor falls back to literal bounds and still runs', async ({
  page,
  request,
  readProvisionedDashboard,
  gotoPanelEditPage,
}) => {
  await seedTradesTable(request);

  const dashboard = await readProvisionedDashboard({ fileName: 'e2e-timefilter.json' });
  const panelEditPage = await gotoPanelEditPage({ dashboard: { uid: dashboard.uid }, id: '1' });

  // Replace the SQL in the Monaco editor; the editor commits the text on blur.
  const editor = panelEditPage.getQueryEditorRow('A').getByRole('textbox');
  await editor.click();
  await page.keyboard.press('ControlOrMeta+KeyA');
  await page.keyboard.type('SELECT count(*) FROM e2e_trades WHERE $__timeFilter(ts); SELECT 1');
  await editor.blur();

  const response = await panelEditPage.refreshPanel({
    waitForResponsePredicateCallback: (resp) =>
      matchesPanelQuery(resp, {
        executedIncludes: 'SELECT count(*) FROM e2e_trades WHERE',
        firstValue: 2,
      }),
  });
  expect(response.ok()).toBeTruthy();
  const frame = frameOf(await response.json());

  // The multi-statement guard inlines the bounds as literals (no placeholders, no
  // sentinel) and the query runs over the simple protocol. The first result row is the
  // count for the 11:30-13:30 window. (lib/pq surfaces the second statement's result
  // as an additional row-set, which the SDK appends — pre-existing behavior.)
  const executed: string = frame.schema.meta.executedQueryString;
  expect(executed).toMatch(/cast\(\d+ as timestamp\)/);
  expect(executed).not.toContain('$1');
  expect(executed).not.toContain('__qdbTimeParam');
  expect(frame.data.values[0][0]).toBe(2);
});
