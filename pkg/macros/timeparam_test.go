package macros

import (
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data/sqlutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRewriteTimeParams(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		wantSQL    string
		wantValues []int64
	}{
		{
			name:       "no markers returns input unchanged",
			in:         "select * from t where ts > now()",
			wantSQL:    "select * from t where ts > now()",
			wantValues: nil,
		},
		{
			name:       "single marker",
			in:         "select * from t where ts >= cast(__qdbTimeParam(1705754096789000) as timestamp)",
			wantSQL:    "select * from t where ts >= cast($1 as timestamp)",
			wantValues: []int64{1705754096789000},
		},
		{
			name:       "two markers numbered in text order",
			in:         "ts >= cast(__qdbTimeParam(100) as timestamp) AND ts <= cast(__qdbTimeParam(200) as timestamp)",
			wantSQL:    "ts >= cast($1 as timestamp) AND ts <= cast($2 as timestamp)",
			wantValues: []int64{100, 200},
		},
		{
			name:       "duplicate macro call - same value pushed twice",
			in:         "(cast(__qdbTimeParam(7) as timestamp)) or (cast(__qdbTimeParam(7) as timestamp))",
			wantSQL:    "(cast($1 as timestamp)) or (cast($2 as timestamp))",
			wantValues: []int64{7, 7},
		},
		{
			name:       "negative (pre-1970) value preserves sign",
			in:         "ts >= cast(__qdbTimeParam(-315619200000000) as timestamp)",
			wantSQL:    "ts >= cast($1 as timestamp)",
			wantValues: []int64{-315619200000000},
		},
		{
			name:       "zero epoch",
			in:         "ts >= cast(__qdbTimeParam(0) as timestamp)",
			wantSQL:    "ts >= cast($1 as timestamp)",
			wantValues: []int64{0},
		},
		{
			// A macro can expand into a string literal (sqlds interpolation is textual),
			// so a marker there is inlined as a literal, never turned into a placeholder.
			name:       "marker inside a string literal is inlined as a literal",
			in:         "select '__qdbTimeParam(999)', cast(__qdbTimeParam(5) as timestamp)",
			wantSQL:    "select '999', cast($1 as timestamp)",
			wantValues: []int64{5},
		},
		{
			name:       "marker inside a doubled-quote-escaped string is inlined",
			in:         "select 'it''s __qdbTimeParam(9)', cast(__qdbTimeParam(7) as timestamp)",
			wantSQL:    "select 'it''s 9', cast($1 as timestamp)",
			wantValues: []int64{7},
		},
		{
			name:       "marker inside a double-quoted identifier is inlined",
			in:         `select "__qdbTimeParam(1)" from t where x = cast(__qdbTimeParam(8) as timestamp)`,
			wantSQL:    `select "1" from t where x = cast($1 as timestamp)`,
			wantValues: []int64{8},
		},
		{
			name:       "marker inside a line comment is inlined",
			in:         "select 1 -- __qdbTimeParam(2)\nwhere x = cast(__qdbTimeParam(3) as timestamp)",
			wantSQL:    "select 1 -- 2\nwhere x = cast($1 as timestamp)",
			wantValues: []int64{3},
		},
		{
			name:       "marker inside a block comment is inlined",
			in:         "select 1 /* __qdbTimeParam(2) */ where x = cast(__qdbTimeParam(3) as timestamp)",
			wantSQL:    "select 1 /* 2 */ where x = cast($1 as timestamp)",
			wantValues: []int64{3},
		},
		{
			// QuestDB (like PostgreSQL) nests block comments: the comment ends at the
			// matching close, not the first one, so both inner markers are inlined.
			name:       "markers inside a nested block comment are inlined",
			in:         "select 1 /* a /* __qdbTimeParam(2) */ __qdbTimeParam(4) */ where x = cast(__qdbTimeParam(3) as timestamp)",
			wantSQL:    "select 1 /* a /* 2 */ 4 */ where x = cast($1 as timestamp)",
			wantValues: []int64{3},
		},
		{
			name:       "identifier merely ending in the marker name is not a marker",
			in:         "select my__qdbTimeParam(42), cast(__qdbTimeParam(5) as timestamp)",
			wantSQL:    "select my__qdbTimeParam(42), cast($1 as timestamp)",
			wantValues: []int64{5},
		},
		{
			name:       "dollar-prefixed marker name is not a marker",
			in:         "select $__qdbTimeParam(42), cast(__qdbTimeParam(5) as timestamp)",
			wantSQL:    "select $__qdbTimeParam(42), cast($1 as timestamp)",
			wantValues: []int64{5},
		},
		{
			// QuestDB has no dollar-quoted strings but does allow '$' in identifiers, so
			// '$' must be ordinary code and must not swallow the rest of the SQL.
			name:       "identifier containing dollars does not open a string region",
			in:         "select a$b$c from t where ts >= cast(__qdbTimeParam(5) as timestamp)",
			wantSQL:    "select a$b$c from t where ts >= cast($1 as timestamp)",
			wantValues: []int64{5},
		},
		{
			name:       "malformed marker (non-numeric) is left untouched",
			in:         "select __qdbTimeParam(abc), cast(__qdbTimeParam(4) as timestamp)",
			wantSQL:    "select __qdbTimeParam(abc), cast($1 as timestamp)",
			wantValues: []int64{4},
		},
		{
			// The realistic case: a time macro expanded both inside a string and in an
			// executable position. The string one is inlined; the executable one is bound.
			name:       "macro expanded inside a string is inlined; executable one is parameterized",
			in:         "select x where label = 'cast(__qdbTimeParam(1707) as timestamp)' and ts >= cast(__qdbTimeParam(1705) as timestamp)",
			wantSQL:    "select x where label = 'cast(1707 as timestamp)' and ts >= cast($1 as timestamp)",
			wantValues: []int64{1705},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSQL, gotValues := rewriteTimeParams(tt.in)
			assert.Equal(t, tt.wantSQL, gotSQL)
			assert.Equal(t, tt.wantValues, gotValues)
		})
	}
}

func TestSkipQuotedOrComment(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		i        int
		wantNext int
		wantSkip bool
	}{
		{
			name:     "ordinary code",
			sql:      "select 1",
			i:        0,
			wantNext: 0,
			wantSkip: false,
		},
		{
			name:     "single quoted string with doubled quote",
			sql:      "'it''s'; select 1",
			i:        0,
			wantNext: len("'it''s'"),
			wantSkip: true,
		},
		{
			name:     "double quoted identifier with doubled quote",
			sql:      `"a""b"; select 1`,
			i:        0,
			wantNext: len(`"a""b"`),
			wantSkip: true,
		},
		{
			name:     "line comment stops before LF",
			sql:      "-- comment\nselect 1",
			i:        0,
			wantNext: len("-- comment"),
			wantSkip: true,
		},
		{
			name:     "line comment stops before CRLF",
			sql:      "-- comment\r\nselect 1",
			i:        0,
			wantNext: len("-- comment"),
			wantSkip: true,
		},
		{
			name:     "line comment stops before bare CR",
			sql:      "-- comment\rselect 1",
			i:        0,
			wantNext: len("-- comment"),
			wantSkip: true,
		},
		{
			name:     "nested block comment",
			sql:      "/* outer /* inner */ outer */ select 1",
			i:        0,
			wantNext: len("/* outer /* inner */ outer */"),
			wantSkip: true,
		},
		{
			name:     "unterminated single quoted string reaches end",
			sql:      "'unterminated",
			i:        0,
			wantNext: len("'unterminated"),
			wantSkip: true,
		},
		{
			name:     "unterminated block comment reaches end",
			sql:      "/* unterminated",
			i:        0,
			wantNext: len("/* unterminated"),
			wantSkip: true,
		},
		{
			name:     "quoted region can start after offset",
			sql:      "select 'x'",
			i:        len("select "),
			wantNext: len("select 'x'"),
			wantSkip: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNext, gotSkip := skipQuotedOrComment(tt.sql, tt.i)
			assert.Equal(t, tt.wantNext, gotNext)
			assert.Equal(t, tt.wantSkip, gotSkip)
		})
	}
}

func TestHasStatementSeparator(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"single statement", "select * from t where ts > now()", false},
		{"trailing semicolon", "select 1;", false},
		{"trailing semicolon then whitespace", "select 1;  \n\t ", false},
		{"trailing semicolon then line comment", "select 1; -- bye", false},
		{"trailing semicolon then block comment", "select 1; /* bye */", false},
		{"two statements", "select 1; select 2", true},
		// A quoted token after ';' is content, not ignorable like a comment: QuestDB
		// runs a bare quoted table name as an implicit SELECT, a second statement.
		{"quoted identifier after semicolon is a second statement", `select 1; "trades"`, true},
		{"string literal after semicolon is a second statement", "select 1; 'oops'", true},
		{"comment then quoted token after semicolon", "select 1; /* hi */ \"trades\"", true},
		{"two statements no space", "select 1;select 2", true},
		{"semicolon inside string literal", "select ';not a separator' from t", false},
		{"semicolon inside identifier", `select "a;b" from t`, false},
		{"semicolon inside line comment", "select 1 -- a;b\nfrom t", false},
		{"line comment ends at bare carriage return", "select 1 -- comment\r; select 2", true},
		{"semicolon inside block comment", "select 1 /* a;b */ from t", false},
		{"semicolon inside nested block comment", "select 1 /* a /* ; */ ; */ from t", false},
		// '$' is ordinary code (QuestDB has no dollar-quoted strings), so a ';' between
		// '$$' pairs IS a separator — the conservative direction: literal fallback.
		{"semicolon between dollar signs is a separator", "select $$a;b$$ from t", true},
		// Regression: an identifier legally containing '$' (QuestDB accepts a$b$c) must
		// not be mistaken for a dollar-quote that swallows the ';'.
		{"separator after a dollar-bearing identifier", "select 1 from t where a$b$c > 0; select 2", true},
		{"dollar positional param is not a quote; separator still found", "select $1 a; select 2", true},
		{"separator after a string with a semicolon", "select ';' a; select 2", true},
		{"no semicolon at all", "select 1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasStatementSeparator(tt.in))
		})
	}
}

func paramQuery(t *testing.T, rawSQL, fromISO, toISO string) *sqlutil.Query {
	t.Helper()
	from, err := time.Parse(time.RFC3339, fromISO)
	require.NoError(t, err)
	to, err := time.Parse(time.RFC3339, toISO)
	require.NoError(t, err)
	return &sqlutil.Query{RawSQL: rawSQL, TimeRange: backend.TimeRange{From: from, To: to}}
}

func TestParameterizeTimeMacros(t *testing.T) {
	const from, to = "2024-01-20T12:34:56.789Z", "2024-02-10T09:21:02.123Z"
	const fromMicros, toMicros = "1705754096789000", "1707556862123000"

	tests := []struct {
		name       string
		in         string
		wantSQL    string
		wantValues []int64
	}{
		{
			// Empty SQL means: do not parameterize, run the query unmodified through
			// sqlds's normal literal interpolation pass.
			name:       "no macros means run unmodified",
			in:         "select * from t where ts > now()",
			wantSQL:    "",
			wantValues: nil,
		},
		{
			name:       "timeFilter expands to two placeholders",
			in:         "select count(*) from t where $__timeFilter(ts)",
			wantSQL:    "select count(*) from t where ts >= cast($1 as timestamp) AND ts <= cast($2 as timestamp)",
			wantValues: []int64{1705754096789000, 1707556862123000},
		},
		{
			name:       "fromTime and toTime expand to placeholders in text order",
			in:         "select * from t where ts >= $__fromTime and ts <= $__toTime",
			wantSQL:    "select * from t where ts >= cast($1 as timestamp) and ts <= cast($2 as timestamp)",
			wantValues: []int64{1705754096789000, 1707556862123000},
		},
		{
			name:       "multi-statement query runs unmodified",
			in:         "select count(*) from t where $__timeFilter(ts); select 1",
			wantSQL:    "",
			wantValues: nil,
		},
		{
			// QuestDB runs a bare quoted table name as an implicit SELECT, so this is
			// multi-statement too — binding args would switch lib/pq to the extended
			// protocol, which rejects the second statement.
			name:       "quoted second statement is still a separator",
			in:         `select 1 from t where ts >= $__fromTime; "trades"`,
			wantSQL:    "",
			wantValues: nil,
		},
		{
			name:       "trailing semicolon is not a separator and still parameterizes",
			in:         "select count(*) from t where $__timeFilter(ts);",
			wantSQL:    "select count(*) from t where ts >= cast($1 as timestamp) AND ts <= cast($2 as timestamp);",
			wantValues: []int64{1705754096789000, 1707556862123000},
		},
		{
			// A hand-written $N placeholder would collide with the generated ones (sqlds
			// never binds args for it), so the query runs unmodified and fails or
			// succeeds exactly as it did before parameterization.
			name:       "hand-written $N placeholder runs unmodified",
			in:         "select * from t where v = $1 and ts >= $__fromTime",
			wantSQL:    "",
			wantValues: nil,
		},
		{
			name:       "$N inside a string literal does not prevent parameterization",
			in:         "select '$1' from t where ts >= $__fromTime",
			wantSQL:    "select '$1' from t where ts >= cast($1 as timestamp)",
			wantValues: []int64{1705754096789000},
		},
		{
			// Regression (verified against live QuestDB): a$b$c is a legal identifier and
			// must not be read as a dollar-quoted string that hides the ';' — binding args
			// would switch lib/pq to the extended protocol, which rejects the second
			// statement.
			name:       "dollar-bearing identifier does not hide a statement separator",
			in:         "select 1 from t where ts >= $__fromTime and a$b$c > 0; select 2",
			wantSQL:    "",
			wantValues: nil,
		},
		{
			// Regression: the same misread used to hide a hand-written $1 from the
			// collision guard, silently binding the time value to the user's $1.
			name:       "dollar-bearing identifier does not hide a hand-written $N",
			in:         "select 1 from t where ts >= $__fromTime and a$b$c = $1",
			wantSQL:    "",
			wantValues: nil,
		},
		{
			// Sentinel collision: user SQL that literally contains the marker text is
			// indistinguishable from macro output, so nothing is rewritten — pre-change
			// the unknown function failed loudly on the server; it still does.
			name:       "user SQL containing the sentinel text runs unmodified",
			in:         "select __qdbTimeParam(99) from t where ts >= $__fromTime",
			wantSQL:    "",
			wantValues: nil,
		},
		{
			// Same guard for sentinel text inside a string literal: rewriting would
			// silently corrupt the string's content (e.g. a LIKE pattern).
			name:       "sentinel text inside a string literal runs unmodified",
			in:         "select * from t where msg like '%__qdbTimeParam(5)%' and ts >= $__fromTime",
			wantSQL:    "",
			wantValues: nil,
		},
		{
			name:       "macro expanded inside a comment is inlined as a literal",
			in:         "select count(*) from t where $__timeFilter(ts) /* was: ts >= $__fromTime */",
			wantSQL:    "select count(*) from t where ts >= cast($1 as timestamp) AND ts <= cast($2 as timestamp) /* was: ts >= cast(" + fromMicros + " as timestamp) */",
			wantValues: []int64{1705754096789000, 1707556862123000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSQL, gotValues, err := ParameterizeTimeMacros(paramQuery(t, tt.in, from, to))
			require.NoError(t, err)
			assert.Equal(t, tt.wantSQL, gotSQL)
			assert.Equal(t, tt.wantValues, gotValues)
		})
	}
}

// TestParameterizeTimeMacrosSentinelViaTableColumn guards the pre-check's other entry
// points: sqlutil's default $__table/$__column macros splice the user-controlled
// Table/Column model fields into the SQL, so sentinel text there must also disable
// parameterization (verified live: without this guard, {table: "__qdbTimeParam(666)"}
// fabricated a $1 bind arg).
func TestParameterizeTimeMacrosSentinelViaTableColumn(t *testing.T) {
	const from, to = "2024-01-20T12:00:00Z", "2024-01-20T13:00:00Z"

	q := paramQuery(t, "select $__table from t where ts >= $__fromTime", from, to)
	q.Table = "__qdbTimeParam(666)"
	sql, values, err := ParameterizeTimeMacros(q)
	require.NoError(t, err)
	assert.Empty(t, sql)
	assert.Nil(t, values)

	q = paramQuery(t, "select $__column from t where ts >= $__fromTime", from, to)
	q.Column = "'__qdbTimeParam(666)'"
	sql, values, err = ParameterizeTimeMacros(q)
	require.NoError(t, err)
	assert.Empty(t, sql)
	assert.Nil(t, values)
}

// TestParameterizeTimeMacrosMacroTextViaTableColumn: macro text in the user-controlled
// Table/Column model fields splices in too late to be expanded in this pass ($__table
// and $__column run after the longer-named time macros), so only sqlds's SECOND
// interpolation pass over the rewritten SQL would expand it — silently executing SQL
// that the single-pass pipeline sends to the server verbatim (which rejects it). Such
// queries must run unmodified instead.
func TestParameterizeTimeMacrosMacroTextViaTableColumn(t *testing.T) {
	const from, to = "2024-01-20T12:00:00Z", "2024-01-20T13:00:00Z"

	q := paramQuery(t, "select $__table from t where ts >= $__fromTime", from, to)
	q.Table = "$__fromTime"
	sql, values, err := ParameterizeTimeMacros(q)
	require.NoError(t, err)
	assert.Empty(t, sql)
	assert.Nil(t, values)

	q = paramQuery(t, "select $__column from t where ts >= $__fromTime", from, to)
	q.Column = "$__toTime"
	sql, values, err = ParameterizeTimeMacros(q)
	require.NoError(t, err)
	assert.Empty(t, sql)
	assert.Nil(t, values)
}

func TestParameterizeTimeMacrosBadArgsErrors(t *testing.T) {
	_, _, err := ParameterizeTimeMacros(paramQuery(t,
		"select * from t where $__timeFilter(a, b)", "2024-01-20T12:00:00Z", "2024-01-20T13:00:00Z"))
	require.Error(t, err)
}

func TestHasTimeBoundMacro(t *testing.T) {
	assert.True(t, HasTimeBoundMacro("select * from t where $__timeFilter(ts)"))
	assert.True(t, HasTimeBoundMacro("select * from t where ts >= $__fromTime"))
	assert.True(t, HasTimeBoundMacro("select * from t where ts <= $__toTime"))
	assert.False(t, HasTimeBoundMacro("select 1 sample by $__sampleByInterval"))
	assert.False(t, HasTimeBoundMacro("select 1"))
}

// TestParameterizeTimeMacrosTextStability is the falsifiable signal behind the whole
// change: the same panel refreshed with a rolling window must produce a byte-identical
// SQL string so QuestDB reuses its cached plan. We expand the real $__timeFilter macro
// for two different windows and assert the rewritten texts are identical while the
// bound values differ.
func TestParameterizeTimeMacrosTextStability(t *testing.T) {
	const rawSQL = "select count(*) from trades where $__timeFilter(ts)"

	sqlA, valsA, err := ParameterizeTimeMacros(paramQuery(t, rawSQL, "2024-01-20T12:34:56Z", "2024-01-20T13:34:56Z"))
	require.NoError(t, err)
	sqlB, valsB, err := ParameterizeTimeMacros(paramQuery(t, rawSQL, "2024-06-01T00:00:00Z", "2024-06-01T06:00:00Z"))
	require.NoError(t, err)

	assert.Equal(t, "select count(*) from trades where ts >= cast($1 as timestamp) AND ts <= cast($2 as timestamp)", sqlA)
	assert.Equal(t, sqlA, sqlB, "rewritten SQL must be byte-identical across windows")
	assert.NotEqual(t, valsA, valsB, "bound values must differ across windows")
	assert.Len(t, valsA, 2)
}
