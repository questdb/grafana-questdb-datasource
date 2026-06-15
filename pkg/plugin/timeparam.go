package plugin

// Time-bound parameterization, plugin side: the sqlds hooks (MutateQuery/SetQueryArgs),
// the advisory Connect-time bind-capability probe, and their plumbing. Whether queries
// bind at all is decided by the "disable prepared statements" datasource setting
// (Settings.DisablePreparedStatements, applied in Connect). The SQL-text side — macro
// interpolation, the sentinel marker scanner and the fallback guards — lives in
// pkg/macros/timeparam.go.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/sqlds/v4"
	"github.com/lib/pq"

	"github.com/questdb/grafana-questdb-datasource/pkg/macros"
)

// sqlds invokes MutateQuery and SetQueryArgs only when the driver implements these
// optional interfaces; the time-bound parameterization depends on both.
var (
	_ sqlds.QueryMutator   = (*QuestDB)(nil)
	_ sqlds.QueryArgSetter = (*QuestDB)(nil)
)

// bindParamsProbeTimeout bounds the advisory probe round-trip. Connect runs it
// synchronously while the SDK's instance manager holds the per-datasource creation
// lock, so an unresponsive server must not stall instance creation — and every query
// and health check queued behind it — for longer than this.
const bindParamsProbeTimeout = 5 * time.Second

// warnIfBindParamsUnsupported runs one advisory probe, using the same bind shape the
// time macros use, and logs a warning when the server itself rejects it: prepared
// statements are enabled (the default) but the server cannot run them — older QuestDB
// versions reject every lib/pq-style bind (8.2.x and older; 8.3.0+ accept) — so every
// time-macro query is about to fail with this same error until the user enables the
// "disable prepared statements" datasource setting or upgrades the server.
//
// The probe steers nothing: whether queries bind is decided solely by the setting. A
// transport-level failure (server unreachable, timeout) proves nothing and stays
// silent.
func warnIfBindParamsUnsupported(ctx context.Context, db *sql.DB) {
	ctx, cancel := context.WithTimeout(ctx, bindParamsProbeTimeout)
	defer cancel()
	var probed time.Time
	err := db.QueryRowContext(ctx, "SELECT cast($1 as timestamp)", int64(0)).Scan(&probed)
	if err == nil || !isBindRejection(err) {
		return
	}
	log.DefaultLogger.Warn(
		"Server rejected bind parameters (QuestDB older than 8.3.0?); queries using time-bound macros will fail. "+
			"Enable 'Disable prepared statements' in the datasource settings or upgrade the server.",
		"error", err)
}

// isBindRejection reports whether the probe error is a verdict on bind-parameter
// capability, as opposed to a transport-level failure (dial error, timeout, TLS
// failure) that never produced one. Old QuestDB servers surface the rejection in one
// of two shapes:
//   - the server parses the statement but its Describe response reports 0 bind
//     parameters, making lib/pq fail client-side with "pq: got 1 parameters but the
//     statement requires 0" (a plain error, NOT a *pq.Error);
//   - the server rejects the statement outright with a wire-protocol ErrorResponse,
//     which lib/pq surfaces as *pq.Error.
//
// Both mean a healthy round-trip reached the server and the bind was unusable. A
// *pq.Error can in principle also be an unrelated server-side failure, which would
// make the probe log a spurious warning; the probe is advisory only (it runs at
// Connect, not per query, and steers nothing), so a misclassification has no effect
// beyond that log line.
func isBindRejection(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return true
	}
	return strings.Contains(err.Error(), "parameters but the statement requires")
}

// MutateQuery runs before sqlds interpolates the query. When the SQL carries a
// time-bound macro, the macros are expanded here instead, with the time bounds becoming
// positional bind placeholders ($N); the rewritten SQL is written back into req.JSON and
// the bound values are stashed in the returned ctx, which sqlds threads to SetQueryArgs.
// The SQL text then stays byte-stable across dashboard refreshes — only the bound values
// change — so QuestDB serves repeated panel queries from its compiled-plan cache instead
// of recompiling on every refresh.
//
// On any error the query is returned unchanged: sqlds's own interpolation pass then
// expands the macros with the literal-emitting set (Macros()), reproducing the
// pre-parameterization behavior, or surfaces the macro error through its usual path.
func (h *QuestDB) MutateQuery(ctx context.Context, req backend.DataQuery) (context.Context, backend.DataQuery) {
	if !h.bindParamsEnabled.Load() {
		// Prepared statements are disabled — the datasource opt-out for servers
		// older than 8.3.0, which reject bind parameters (or Connect hasn't run
		// yet); run with literal bounds.
		return ctx, req
	}
	q, err := sqlds.GetQuery(req, nil, false)
	if err != nil {
		return ctx, req
	}
	if !macros.HasTimeBoundMacro(q.RawSQL) {
		return ctx, req
	}
	rewritten, values, err := macros.ParameterizeTimeMacros(q)
	if err != nil {
		return ctx, req
	}
	if len(values) == 0 {
		// Parameterizing would change behavior (multi-statement SQL, a hand-written $N
		// placeholder, or every time macro expanded inside a string/comment); leave the
		// query untouched so sqlds's own interpolation pass inlines the bounds as
		// literals, exactly as before this optimization existed.
		log.DefaultLogger.Debug("Time-bound parameterization skipped, falling back to literal bounds", "refId", req.RefID)
		return ctx, req
	}
	newJSON, err := jsonWithRawSQL(req.JSON, rewritten)
	if err != nil {
		log.DefaultLogger.Debug("Time-bound parameterization skipped, query JSON not rewritable", "refId", req.RefID, "error", err)
		return ctx, req
	}
	req.JSON = newJSON
	return withTimeBoundValues(ctx, values), req
}

// SetQueryArgs returns the bind values MutateQuery stashed in ctx for the query's $N
// time-bound placeholders, in placeholder order. It returns nil for queries without
// parameterized time bounds, so those keep flowing with zero bind arguments (lib/pq's
// simple-query path), exactly as before.
func (h *QuestDB) SetQueryArgs(ctx context.Context, _ http.Header) []interface{} {
	return timeBoundValues(ctx)
}

// timeBoundValuesKey carries the bind values for the $N time-bound placeholders that
// MutateQuery wrote into a query's SQL. sqlds calls MutateQuery and SetQueryArgs with
// the same per-query context (each query runs in its own goroutine), so values never
// leak between queries.
type timeBoundValuesKey struct{}

func withTimeBoundValues(ctx context.Context, values []int64) context.Context {
	args := make([]interface{}, len(values))
	for i, v := range values {
		args[i] = v
	}
	return context.WithValue(ctx, timeBoundValuesKey{}, args)
}

// timeBoundValues returns the values stashed by withTimeBoundValues, or nil when the
// query carries no parameterized time bounds.
func timeBoundValues(ctx context.Context) []interface{} {
	args, _ := ctx.Value(timeBoundValuesKey{}).([]interface{})
	return args
}

// jsonWithRawSQL returns the query JSON with the value of its rawSql field replaced by
// sql, splicing into the original bytes so every other field — and the rest of the
// document — stays byte-for-byte identical (sqlds keys its connection cache on the raw
// bytes of sibling fields like connectionArgs, so a re-marshal that compacts or
// HTML-escapes them is not safe). Go's JSON decoding matches keys case-insensitively,
// so ANY case variant of "rawSql" is replaced — leaving a variant untouched would let
// it win the later decode and silently undo the rewrite while the bind args still ship.
func jsonWithRawSQL(raw []byte, sql string) ([]byte, error) {
	type span struct{ start, end int }
	var spans []span

	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("query JSON is not an object")
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected token %v in query JSON object", keyTok)
		}
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return nil, err
		}
		if strings.EqualFold(key, "rawSql") {
			end := int(dec.InputOffset())
			spans = append(spans, span{start: end - len(val), end: end})
		}
	}
	if len(spans) == 0 {
		return nil, fmt.Errorf("query JSON has no rawSql field")
	}

	// Encode without HTML escaping so SQL operators like '>=' stay readable in the
	// stored query model; Encode appends a trailing newline, which the trim drops.
	var encBuf bytes.Buffer
	encoder := json.NewEncoder(&encBuf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(sql); err != nil {
		return nil, err
	}
	enc := bytes.TrimRight(encBuf.Bytes(), "\n")
	var b bytes.Buffer
	b.Grow(len(raw) + len(enc))
	prev := 0
	for _, s := range spans {
		b.Write(raw[prev:s.start])
		b.Write(enc)
		prev = s.end
	}
	b.Write(raw[prev:])
	return b.Bytes(), nil
}
