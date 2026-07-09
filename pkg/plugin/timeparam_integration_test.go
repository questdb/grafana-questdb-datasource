package plugin_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/sqlds/v4"
	"github.com/questdb/grafana-questdb-datasource/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// literalCountSQL is the pre-parameterization form: bare microsecond literals cast to
// timestamp. The parameterized form must return identical results.
func literalCountSQL(table string, from, to time.Time) string {
	return fmt.Sprintf("SELECT count(*) FROM %s WHERE ts >= cast(%d as timestamp) AND ts <= cast(%d as timestamp)",
		table, from.UnixMicro(), to.UnixMicro())
}

// mustTime parses an RFC3339 timestamp, failing the test on a typo instead of silently
// yielding the zero time (which would turn window assertions vacuous).
func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	require.NoError(t, err)
	return v
}

// mustRecreateTable drops, creates and seeds a test table, registering a cleanup drop.
// schema is the parenthesized column list plus designated-timestamp/partition clauses.
func mustRecreateTable(t *testing.T, db *sql.DB, table, schema string, inserts ...string) {
	t.Helper()
	_, err := db.Exec("DROP TABLE IF EXISTS " + table)
	require.NoError(t, err)
	_, err = db.Exec("CREATE TABLE " + table + " " + schema)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE IF EXISTS " + table) })
	for _, insert := range inserts {
		_, err = db.Exec(insert)
		require.NoError(t, err)
	}
}

func queryCount(t *testing.T, db *sql.DB, query string, args ...interface{}) int64 {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), query, args...)
	require.NoError(t, err, "query: %s", query)
	defer rows.Close()
	require.True(t, rows.Next(), "expected a row for: %s", query)
	var c int64
	require.NoError(t, rows.Scan(&c))
	require.NoError(t, rows.Err())
	return c
}

// TestParameterizedTimeBoundsIntegration verifies, on a real QuestDB, that the
// parameterized form produced by the MutateQuery/SetQueryArgs pipeline (a) returns
// results identical to the literal form, (b) returns correct and distinct counts when
// the same pooled connection is re-bound with a different window (proving the interval
// is resolved at bind time, not baked into a cached plan), and (c) still uses a
// designated-timestamp interval scan. It runs against both a timestamp- and a
// timestamp_ns-designated table.
func TestParameterizedTimeBoundsIntegration(t *testing.T) {
	db := setupConnection(t)
	t.Cleanup(func() { _ = db.Close() })

	ts := func(s string) time.Time { return mustTime(t, s) }

	rows := []string{
		"2024-01-20T11:00:00.000000Z",
		"2024-01-20T12:00:00.000000Z",
		"2024-01-20T13:00:00.000000Z",
		"2024-01-20T14:00:00.000000Z",
	}

	// narrow window: 12:00 and 13:00 -> 2 rows; wide window: all 4.
	narrowFrom, narrowTo := ts("2024-01-20T11:30:00Z"), ts("2024-01-20T13:30:00Z")
	wideFrom, wideTo := ts("2024-01-20T10:30:00Z"), ts("2024-01-20T14:30:00Z")

	for _, tsType := range []string{"timestamp", "timestamp_ns"} {
		t.Run(tsType, func(t *testing.T) {
			table := "bind_it_" + tsType
			_, err := db.Exec("DROP TABLE IF EXISTS " + table)
			require.NoError(t, err)

			create := fmt.Sprintf("CREATE TABLE %s (ts %s) TIMESTAMP(ts) PARTITION BY DAY BYPASS WAL", table, tsType)
			if _, err := db.Exec(create); err != nil {
				if tsType == "timestamp_ns" {
					t.Skipf("timestamp_ns not supported by this QuestDB build: %v", err)
				}
				require.NoError(t, err)
			}
			t.Cleanup(func() { _, _ = db.Exec("DROP TABLE IF EXISTS " + table) })

			for _, r := range rows {
				_, err = db.Exec(fmt.Sprintf("INSERT INTO %s VALUES ('%s')", table, r))
				require.NoError(t, err)
			}

			rawSQL := fmt.Sprintf("SELECT count(*) FROM %s WHERE $__timeFilter(ts)", table)
			narrowSQL, narrowArgs := runQueryPipeline(t, rawSQL, time.Second, narrowFrom, narrowTo)
			wideSQL, wideArgs := runQueryPipeline(t, rawSQL, time.Second, wideFrom, wideTo)

			require.Contains(t, narrowSQL, "$1", "time bounds should be parameterized")
			require.Len(t, narrowArgs, 2)
			assert.Equal(t, narrowSQL, wideSQL, "SQL text must be byte-identical across windows")

			// (a) result-equivalence with the literal form + correct counts.
			assert.Equal(t, int64(2), queryCount(t, db, narrowSQL, narrowArgs...), "narrow window count")
			assert.Equal(t, queryCount(t, db, literalCountSQL(table, narrowFrom, narrowTo)),
				queryCount(t, db, narrowSQL, narrowArgs...), "parameterized form must equal literal form (narrow)")

			// (b) re-bind a different window through the same pool; correct, distinct result.
			assert.Equal(t, int64(4), queryCount(t, db, wideSQL, wideArgs...), "wide window count")
			assert.Equal(t, queryCount(t, db, literalCountSQL(table, wideFrom, wideTo)),
				queryCount(t, db, wideSQL, wideArgs...), "parameterized form must equal literal form (wide)")

			// (c) the parameterized form keeps a designated-timestamp interval scan.
			scanSQL, scanArgs := runQueryPipeline(t,
				fmt.Sprintf("SELECT ts FROM %s WHERE $__timeFilter(ts)", table), time.Second, narrowFrom, narrowTo)
			assertIntervalScan(t, db, scanSQL, scanArgs...)
		})
	}
}

// TestParameterizedBinaryResultDecoding guards the one behavioural change the
// parameterization introduces: queries with bind args flip lib/pq from the simple
// protocol (all-text results) to the extended protocol, where lib/pq requests BINARY
// result format for int2/int4/int8/uuid/bytea columns. This asserts QuestDB's binary
// wire encoding for those at-risk types decodes to the same values as the literal
// (text) form would, so existing converters/result-shaping stay correct.
func TestParameterizedBinaryResultDecoding(t *testing.T) {
	db := setupConnection(t)
	t.Cleanup(func() { _ = db.Close() })

	const uuidVal = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"
	mustRecreateTable(t, db, "bind_types",
		"(long_ long, int_ int, short_ short, uuid_ uuid, dbl double, str string, ts timestamp) TIMESTAMP(ts) PARTITION BY DAY BYPASS WAL",
		fmt.Sprintf("INSERT INTO bind_types VALUES (42, 7, 3, '%s', 1.5, 'hello', '2024-01-20T12:00:00.000000Z')", uuidVal))

	from, to := mustTime(t, "2024-01-20T11:00:00Z"), mustTime(t, "2024-01-20T13:00:00Z")
	// $__timeFilter -> $N bind args -> lib/pq extended protocol -> binary ints/uuid.
	query, args := runQueryPipeline(t,
		"SELECT long_, int_, short_, uuid_, dbl, str FROM bind_types WHERE $__timeFilter(ts)",
		time.Second, from, to)
	require.Len(t, args, 2)

	rows, err := db.QueryContext(context.Background(), query, args...)
	require.NoError(t, err)
	defer rows.Close()
	require.True(t, rows.Next(), "expected one row")

	var (
		longV, intV, shortV int64
		uuidV, strV         string
		dblV                float64
	)
	require.NoError(t, rows.Scan(&longV, &intV, &shortV, &uuidV, &dblV, &strV))
	require.NoError(t, rows.Err())

	assert.Equal(t, int64(42), longV, "int8/long binary decode")
	assert.Equal(t, int64(7), intV, "int4/int binary decode")
	assert.Equal(t, int64(3), shortV, "int2/short binary decode")
	assert.Equal(t, uuidVal, uuidV, "uuid binary decode")
	assert.InDelta(t, 1.5, dblV, 1e-9, "double (text) decode")
	assert.Equal(t, "hello", strV, "string (text) decode")
}

// TestParameterizedMultiStatementFallback proves the multi-statement guard works
// end-to-end: a hand-written multi-statement query carrying a time macro would hard-fail
// under the extended protocol, but the fallback inlines the bounds as literals so it
// keeps working over the simple protocol exactly as before the change.
func TestParameterizedMultiStatementFallback(t *testing.T) {
	db := setupConnection(t)
	t.Cleanup(func() { _ = db.Close() })

	mustRecreateTable(t, db, "bind_multi",
		"(ts timestamp) TIMESTAMP(ts) PARTITION BY DAY BYPASS WAL",
		"INSERT INTO bind_multi VALUES ('2024-01-20T12:00:00.000000Z')")

	from, to := mustTime(t, "2024-01-20T11:00:00Z"), mustTime(t, "2024-01-20T13:00:00Z")

	// The second statement must NOT error (it would under the extended protocol).
	// QuestDB's simple protocol returns the first statement's result set.
	query, args := runQueryPipeline(t,
		"SELECT count(*) FROM bind_multi WHERE $__timeFilter(ts); SELECT 1", time.Second, from, to)
	require.Nil(t, args, "multi-statement queries must fall back to literal bounds")
	assert.Equal(t, int64(1), queryCount(t, db, query),
		"multi-statement query with a time macro should fall back to literals and still run")
}

// TestQueryDataEndToEnd drives queries through the real sqlds QueryData path against a
// live QuestDB, proving the whole wiring: MutateQuery parameterizes the bounds, sqlds
// threads the ctx to SetQueryArgs, the args flow to the database, and the query
// inspector's ExecutedQueryString shows the $N form (never the internal marker) on both
// success and error paths.
func TestQueryDataEndToEnd(t *testing.T) {
	if getEnv("QUESTDB_TLS_ENABLED", "false") == "true" {
		t.Skip("end-to-end QueryData test uses the non-TLS settings path")
	}

	db := setupConnection(t)
	t.Cleanup(func() { _ = db.Close() })
	mustRecreateTable(t, db, "bind_e2e",
		"(ts timestamp) TIMESTAMP(ts) PARTITION BY DAY BYPASS WAL",
		"INSERT INTO bind_e2e VALUES ('2024-01-20T12:00:00.000000Z')")

	settingsJSON := fmt.Sprintf(
		`{ "server": %q, "port": %q, "username": %q, "tlsMode": "disable", "queryTimeout": "3600", "timeout": "1000", "maxOpenConnections": "5", "maxIdleConnections": "2", "maxConnectionLifetime": "14400" }`,
		getEnv("QUESTDB_HOST", "localhost"), getEnv("QUESTDB_PORT", "8812"), getEnv("QUESTDB_USERNAME", "admin"))
	settings := backend.DataSourceInstanceSettings{
		JSONData:                []byte(settingsJSON),
		DecryptedSecureJSONData: map[string]string{"password": getEnv("QUESTDB_PASSWORD", "quest")},
	}

	sqlDS := sqlds.NewDatasource(&plugin.QuestDB{})
	inst, err := sqlDS.NewDatasource(context.Background(), settings)
	require.NoError(t, err)
	ds, ok := inst.(*sqlds.SQLDatasource)
	require.True(t, ok)
	t.Cleanup(ds.Dispose)

	from, to := mustTime(t, "2024-01-20T11:00:00Z"), mustTime(t, "2024-01-20T13:00:00Z")
	mkReq := func(table string) *backend.QueryDataRequest {
		return &backend.QueryDataRequest{
			Queries: []backend.DataQuery{dataQuery(
				fmt.Sprintf("SELECT count(*) FROM %s WHERE $__timeFilter(ts)", table),
				time.Second, from, to)},
		}
	}

	executedSQL := func(resp *backend.QueryDataResponse) string {
		require.NotNil(t, resp)
		dr := resp.Responses["A"]
		require.NotEmpty(t, dr.Frames)
		require.NotNil(t, dr.Frames[0].Meta)
		return dr.Frames[0].Meta.ExecutedQueryString
	}

	t.Run("success path binds the time window", func(t *testing.T) {
		resp, err := ds.QueryData(context.Background(), mkReq("bind_e2e"))
		require.NoError(t, err)
		dr := resp.Responses["A"]
		require.NoError(t, dr.Error)
		got := executedSQL(resp)
		assert.Contains(t, got, "cast($1 as timestamp)", "inspector should show the placeholder form")
		assert.NotContains(t, got, "__qdbTimeParam", "the internal marker must never escape")
		require.NotEmpty(t, dr.Frames[0].Fields)
		require.Equal(t, 1, dr.Frames[0].Fields[0].Len())
		count, ok := dr.Frames[0].Fields[0].At(0).(*int64)
		require.True(t, ok, "count(*) should convert to *int64, got %T", dr.Frames[0].Fields[0].At(0))
		require.NotNil(t, count)
		assert.EqualValues(t, 1, *count, "the bound window should match the inserted row")
	})

	t.Run("error frame carries no marker", func(t *testing.T) {
		resp, _ := ds.QueryData(context.Background(), mkReq("no_such_table_zzz"))
		dr := resp.Responses["A"]
		require.Error(t, dr.Error, "query against a missing table should error")
		got := executedSQL(resp)
		assert.Contains(t, got, "cast($1 as timestamp)", "inspector should show the placeholder form")
		assert.NotContains(t, got, "__qdbTimeParam", "the internal marker must never escape")
	})
}

// scrapeSelectCacheHits reads QuestDB's prometheus endpoint and returns the current
// value of the pgwire select-cache hit counter. The counter is registered as
// "pg_wire_select_cache_hits" (PGMetrics) and exposed with the "questdb_" prefix and
// "_total" counter suffix; the exact-token match below cannot be fooled by sibling
// series (_misses, # comments, labelled or future variants), and a rename fails loudly
// rather than feeding the assertions a wrong value.
func scrapeSelectCacheHits(t *testing.T, metricsPort string) float64 {
	t.Helper()
	const hitsMetric = "questdb_pg_wire_select_cache_hits_total"

	resp, err := http.Get(fmt.Sprintf("http://%s:%s/metrics", getEnv("QUESTDB_HOST", "localhost"), metricsPort))
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		// A sample line is "<name> <value>" with an optional trailing timestamp.
		if len(fields) < 2 || fields[0] != hitsMetric {
			continue
		}
		v, err := strconv.ParseFloat(fields[1], 64)
		require.NoError(t, err)
		return v
	}
	t.Fatalf("%s not found on the metrics endpoint; did the QuestDB version rename it?", hitsMetric)
	return 0
}

// TestPlanCacheReuseIntegration proves the feature's headline claim end to end on a
// real QuestDB: executing the SAME dashboard query over rolling time windows must hit
// the server's compiled-plan cache (byte-stable SQL, only bind values change), while
// the pre-change literal form — a distinct SQL text per window — must not. Requires the
// metrics endpoint, so it runs only against the dockerized QuestDB started by TestMain.
func TestPlanCacheReuseIntegration(t *testing.T) {
	metricsPort := getEnv("QUESTDB_METRICS_PORT", "")
	if metricsPort == "" {
		t.Skip("QuestDB metrics endpoint not available (external server); skipping plan-cache validation")
	}

	db := setupConnection(t)
	t.Cleanup(func() { _ = db.Close() })
	mustRecreateTable(t, db, "plan_cache_it",
		"(ts timestamp) TIMESTAMP(ts) PARTITION BY DAY BYPASS WAL",
		"INSERT INTO plan_cache_it VALUES ('2024-01-20T12:00:00.000000Z')")

	const refreshes = 5
	base := mustTime(t, "2024-01-20T11:00:00Z")
	window := func(i int) (time.Time, time.Time) { // a rolling dashboard window
		return base.Add(time.Duration(i) * time.Second), base.Add(2*time.Hour + time.Duration(i)*time.Second)
	}

	// Parameterized form: same SQL text every refresh -> plan-cache hits.
	before := scrapeSelectCacheHits(t, metricsPort)
	var lastSQL string
	for i := 0; i < refreshes; i++ {
		from, to := window(i)
		sql, args := runQueryPipeline(t, "SELECT count(*) FROM plan_cache_it WHERE $__timeFilter(ts)", time.Second, from, to)
		require.Len(t, args, 2)
		if i > 0 {
			require.Equal(t, lastSQL, sql, "SQL text must be byte-stable across refreshes")
		}
		lastSQL = sql
		require.Equal(t, int64(1), queryCount(t, db, sql, args...))
	}
	paramHits := scrapeSelectCacheHits(t, metricsPort) - before

	// Pre-change literal form: a distinct SQL text per refresh -> no reuse.
	before = scrapeSelectCacheHits(t, metricsPort)
	for i := 0; i < refreshes; i++ {
		from, to := window(i)
		require.Equal(t, int64(1), queryCount(t, db, literalCountSQL("plan_cache_it", from, to)))
	}
	literalHits := scrapeSelectCacheHits(t, metricsPort) - before

	assert.GreaterOrEqual(t, paramHits, float64(refreshes-1),
		"parameterized refreshes after the first must be served from QuestDB's plan cache")
	assert.Zero(t, literalHits,
		"sanity: distinct literal SQL texts must not produce plan-cache hits")
}

// TestQuestDBRejectsDollarQuotes pins the lexer assumption behind the SQL scanner in
// pkg/macros/timeparam.go: it treats '$' as ordinary code BECAUSE QuestDB has no
// dollar-quoted strings (while '$' inside identifiers is legal). If this test ever
// fails — QuestDB grew dollar-quote support — revisit skipQuotedOrComment: a time macro
// expanded inside $$...$$ would then be misclassified as executable and break the query.
func TestQuestDBRejectsDollarQuotes(t *testing.T) {
	db := setupConnection(t)
	t.Cleanup(func() { _ = db.Close() })

	for _, q := range []string{"SELECT $$hello$$", "SELECT $tag$hello$tag$"} {
		rows, err := db.Query(q)
		if err == nil {
			_ = rows.Close()
		}
		require.Error(t, err, "QuestDB now accepts dollar-quoted strings (%s); update the scanner in pkg/macros/timeparam.go", q)
	}
}

func assertIntervalScan(t *testing.T, db *sql.DB, query string, args ...interface{}) {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), "EXPLAIN "+query, args...)
	require.NoError(t, err)
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	require.NoError(t, rows.Err())
	assert.Contains(t, plan.String(), "Interval",
		"parameterized bounds should keep a designated-timestamp interval scan; plan was:\n%s", plan.String())
}
