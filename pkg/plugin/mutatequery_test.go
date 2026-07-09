package plugin_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/sqlds/v4"
	"github.com/questdb/grafana-questdb-datasource/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	pipelineFrom = time.Date(2024, 1, 20, 12, 34, 56, 789000000, time.UTC)
	pipelineTo   = time.Date(2024, 2, 10, 9, 21, 2, 123000000, time.UTC)
)

func dataQuery(rawSQL string, interval time.Duration, from, to time.Time) backend.DataQuery {
	return backend.DataQuery{
		RefID:     "A",
		Interval:  interval,
		TimeRange: backend.TimeRange{From: from, To: to},
		JSON:      []byte(fmt.Sprintf(`{"rawSql": %s, "format": 1, "meta": {"timezone": "UTC"}}`, strconv.Quote(rawSQL))),
	}
}

// newTestDriver returns a QuestDB driver with bind-parameter support force-enabled,
// standing in for the Connect-time capability probe (no live server in unit tests).
func newTestDriver() *plugin.QuestDB {
	h := &plugin.QuestDB{}
	plugin.EnableBindParams(h)
	return h
}

// runQueryPipeline pushes a raw dashboard SQL through the exact pipeline sqlds runs per
// query — QuestDB.MutateQuery, then GetQuery + Interpolate on the mutated request — and
// returns the SQL text that reaches the database plus the bind args SetQueryArgs
// supplies for it.
func runQueryPipeline(t *testing.T, rawSQL string, interval time.Duration, from, to time.Time) (string, []interface{}) {
	t.Helper()
	h := newTestDriver()
	ctx, req := h.MutateQuery(context.Background(), dataQuery(rawSQL, interval, from, to))
	q, err := sqlds.GetQuery(req, nil, false)
	require.NoError(t, err)
	interpolated, err := sqlds.Interpolate(h, q)
	require.NoError(t, err)
	return interpolated, h.SetQueryArgs(ctx, nil)
}

func TestMutateQueryParameterizesTimeBounds(t *testing.T) {
	sql, args := runQueryPipeline(t,
		"SELECT count(*) FROM trades WHERE $__timeFilter(ts) SAMPLE BY $__sampleByInterval",
		30*time.Second, pipelineFrom, pipelineTo)

	assert.Equal(t,
		"SELECT count(*) FROM trades WHERE ts >= cast($1 as timestamp) AND ts <= cast($2 as timestamp) SAMPLE BY 30s",
		sql)
	assert.Equal(t, []interface{}{pipelineFrom.UnixMicro(), pipelineTo.UnixMicro()}, args)
}

func TestMutateQueryTextStability(t *testing.T) {
	const rawSQL = "SELECT * FROM trades WHERE ts >= $__fromTime AND ts <= $__toTime"
	sqlA, argsA := runQueryPipeline(t, rawSQL, time.Second, pipelineFrom, pipelineTo)
	sqlB, argsB := runQueryPipeline(t, rawSQL, time.Second, pipelineFrom.Add(time.Hour), pipelineTo.Add(time.Hour))

	assert.Equal(t, sqlA, sqlB, "SQL text must be byte-identical across time windows")
	assert.NotEqual(t, argsA, argsB, "bound values must differ across time windows")
}

func TestMutateQueryPreservesOtherJSONFields(t *testing.T) {
	h := newTestDriver()
	_, req := h.MutateQuery(context.Background(),
		dataQuery("SELECT 1 FROM t WHERE $__timeFilter(ts)", time.Second, pipelineFrom, pipelineTo))

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(req.JSON, &fields))
	assert.JSONEq(t, `1`, string(fields["format"]))
	assert.JSONEq(t, `{"timezone": "UTC"}`, string(fields["meta"]))
	assert.Contains(t, string(fields["rawSql"]), "$1")
}

// TestMutateQueryPreservesSiblingBytes pins the splice-based JSON rewrite: every byte
// outside the rawSql value must survive verbatim — sqlds keys its connection cache on
// the raw bytes of connectionArgs, so compaction or HTML-escaping is not acceptable.
func TestMutateQueryPreservesSiblingBytes(t *testing.T) {
	in := `{ "connectionArgs": { "tag": "a<b&c" },  "rawSql": "SELECT 1 FROM t WHERE ts >= $__fromTime",  "format": 1 }`
	h := newTestDriver()
	ctx, req := h.MutateQuery(context.Background(), backend.DataQuery{
		RefID:     "A",
		JSON:      []byte(in),
		TimeRange: backend.TimeRange{From: pipelineFrom, To: pipelineTo},
	})

	want := `{ "connectionArgs": { "tag": "a<b&c" },  "rawSql": "SELECT 1 FROM t WHERE ts >= cast($1 as timestamp)",  "format": 1 }`
	assert.Equal(t, want, string(req.JSON))
	assert.Equal(t, []interface{}{pipelineFrom.UnixMicro()}, h.SetQueryArgs(ctx, nil))
}

// TestMutateQueryCaseVariantRawSQLKey guards against Go's case-insensitive JSON field
// matching: a query model using "rawsql" (accepted by every decoder in the chain) must
// have ITS value rewritten — leaving it untouched would let it win sqlds's later decode,
// executing the un-parameterized SQL while the bind args still ship.
func TestMutateQueryCaseVariantRawSQLKey(t *testing.T) {
	h := newTestDriver()
	ctx, req := h.MutateQuery(context.Background(), backend.DataQuery{
		RefID:     "A",
		JSON:      []byte(`{"rawsql": "SELECT count(*) FROM t WHERE $__timeFilter(ts)", "format": 1}`),
		TimeRange: backend.TimeRange{From: pipelineFrom, To: pipelineTo},
	})

	q, err := sqlds.GetQuery(req, nil, false)
	require.NoError(t, err)
	assert.Contains(t, q.RawSQL, "cast($1 as timestamp)",
		"the SQL sqlds decodes back out must be the rewritten form")
	assert.NotContains(t, q.RawSQL, "$__timeFilter")
	assert.Len(t, h.SetQueryArgs(ctx, nil), 2)
}

func TestMutateQueryLeavesQueriesAlone(t *testing.T) {
	tests := []struct {
		name   string
		rawSQL string
	}{
		{"no time macro", "SELECT count(*) FROM trades"},
		{"sampleByInterval only", "SELECT count(*) FROM trades SAMPLE BY $__sampleByInterval"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestDriver()
			in := dataQuery(tt.rawSQL, time.Second, pipelineFrom, pipelineTo)
			ctx, out := h.MutateQuery(context.Background(), in)
			assert.Equal(t, string(in.JSON), string(out.JSON), "request JSON must be untouched")
			assert.Nil(t, h.SetQueryArgs(ctx, nil))
		})
	}
}

func TestMutateQueryFallsBackToLiterals(t *testing.T) {
	tests := []struct {
		name   string
		rawSQL string
	}{
		{"multi-statement", "SELECT count(*) FROM t WHERE $__timeFilter(ts); SELECT 1"},
		{"hand-written placeholder", "SELECT * FROM t WHERE v = $1 AND ts >= $__fromTime"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args := runQueryPipeline(t, tt.rawSQL, time.Second, pipelineFrom, pipelineTo)
			assert.Nil(t, args, "fallback queries must carry no bind args")
			assert.Contains(t, sql, fmt.Sprintf("cast(%d as timestamp)", pipelineFrom.UnixMicro()),
				"time bounds must be inlined as literals")
			assert.NotContains(t, sql, "__qdbTimeParam", "the internal marker must never escape")
		})
	}
}

func TestMutateQueryInvalidJSONIsUntouched(t *testing.T) {
	h := newTestDriver()
	in := backend.DataQuery{JSON: []byte(`{not json`)}
	ctx, out := h.MutateQuery(context.Background(), in)
	assert.Equal(t, string(in.JSON), string(out.JSON))
	assert.Nil(t, h.SetQueryArgs(ctx, nil))
}

func TestSetQueryArgsBareContext(t *testing.T) {
	h := newTestDriver()
	assert.Nil(t, h.SetQueryArgs(context.Background(), nil))
}

// connectTestDriver runs the real Connect with the given extra jsonData fields,
// returning a driver with the prepared-statements setting applied exactly as the
// production wiring applies it.
func connectTestDriver(t *testing.T, extraJSON string) *plugin.QuestDB {
	t.Helper()
	h := &plugin.QuestDB{}
	settingsJSON := fmt.Sprintf(`{ "server": %q, "port": %q, "username": %q, "tlsMode": "disable"%s }`,
		getEnv("QUESTDB_HOST", "localhost"), getEnv("QUESTDB_PORT", "8812"),
		getEnv("QUESTDB_USERNAME", "admin"), extraJSON)
	db, err := h.Connect(context.Background(), backend.DataSourceInstanceSettings{
		JSONData:                []byte(settingsJSON),
		DecryptedSecureJSONData: map[string]string{"password": getEnv("QUESTDB_PASSWORD", "quest")},
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return h
}

// TestConnectAppliesPreparedStatementsSetting drives the disablePreparedStatements
// datasource option through Connect: by default MutateQuery parameterizes; with the
// opt-out (bool, or the string form provisioning env interpolation produces) it
// leaves the query untouched so the bounds are inlined as literals.
func TestConnectAppliesPreparedStatementsSetting(t *testing.T) {
	if getEnv("QUESTDB_TLS_ENABLED", "false") == "true" {
		t.Skip("uses the non-TLS settings path")
	}
	in := dataQuery("SELECT count(*) FROM trades WHERE $__timeFilter(ts)",
		time.Second, pipelineFrom, pipelineTo)

	t.Run("default parameterizes", func(t *testing.T) {
		h := connectTestDriver(t, "")
		ctx, out := h.MutateQuery(context.Background(), in)
		assert.Contains(t, string(out.JSON), "cast($1 as timestamp)")
		assert.Len(t, h.SetQueryArgs(ctx, nil), 2)
	})
	t.Run("opt-out keeps literal bounds", func(t *testing.T) {
		h := connectTestDriver(t, `, "disablePreparedStatements": true`)
		ctx, out := h.MutateQuery(context.Background(), in)
		assert.Equal(t, string(in.JSON), string(out.JSON), "request JSON must be untouched")
		assert.Nil(t, h.SetQueryArgs(ctx, nil))
	})
	t.Run("provisioning-style string opt-out", func(t *testing.T) {
		h := connectTestDriver(t, `, "disablePreparedStatements": "true"`)
		_, out := h.MutateQuery(context.Background(), in)
		assert.Equal(t, string(in.JSON), string(out.JSON), "request JSON must be untouched")
	})
}
