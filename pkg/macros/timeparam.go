package macros

import (
	"strconv"
	"strings"

	"github.com/grafana/grafana-plugin-sdk-go/data/sqlutil"
)

// timeParamMarkerName is the sentinel, function-like token that the parameterizing
// time macros wrap around a time bound's int64 microsecond-epoch value, e.g.
// "__qdbTimeParam(1705754096789000)". ParameterizeTimeMacros rewrites every marker that
// appears in an executable SQL position into a positional bind placeholder ($N) plus an
// argument value (see rewriteTimeParams). This makes the SQL text a dashboard sends
// byte-stable across refreshes — only the bound values change — so QuestDB serves it
// from its compiled-plan cache instead of recompiling on every refresh.
//
// The name is deliberately unlikely to occur in hand-written SQL. The rewriter only
// ever touches markers in executable positions: occurrences inside string literals,
// quoted identifiers, or comments are left untouched.
const timeParamMarkerName = "__qdbTimeParam"

const timeParamMarkerOpen = timeParamMarkerName + "("

// timeParamMarker renders the sentinel marker carrying the given int64 micros. The
// value may be negative (pre-1970 epochs); the sign is preserved.
func timeParamMarker(micros int64) string {
	return timeParamMarkerOpen + strconv.FormatInt(micros, 10) + ")"
}

// markerSpan locates one time-param marker within a SQL string.
type markerSpan struct {
	start, end int   // sql[start:end] == "__qdbTimeParam(<value>)"
	value      int64 // the int64 micros carried by the marker
	executable bool  // false when the marker sits inside a string, identifier or comment
}

// skipQuotedOrComment reports whether sql[i] begins a region that must not be parsed as
// code — a single-quoted string literal, a double-quoted identifier, a line comment
// (dash-dash to end of line) or a block comment (slash-star to star-slash) — and if so
// returns the index just past that region. A doubled quote within a quoted region is an
// escaped quote, not a terminator. Block comments nest, as they do in QuestDB's and
// PostgreSQL's lexers. When sql[i] is ordinary code it returns (i, false).
//
// Deliberately NOT modeled: PostgreSQL dollar-quoted strings ($$...$$, $tag$...$tag$).
// QuestDB rejects them (verified live: "SELECT $$hello$$" errors), and treating '$' as a
// quote opener would misclassify identifiers that legally contain '$' (QuestDB accepts
// "a$b$c" as an identifier), hiding a later ';' or '$N' from the fallback guards. A '$'
// is ordinary code here.
func skipQuotedOrComment(sql string, i int) (int, bool) {
	n := len(sql)
	switch c := sql[i]; {
	case c == '\'' || c == '"':
		i++
		for i < n {
			if sql[i] == c {
				if i+1 < n && sql[i+1] == c {
					i += 2
					continue
				}
				return i + 1, true
			}
			i++
		}
		return n, true
	case c == '-' && i+1 < n && sql[i+1] == '-':
		i += 2
		for i < n && sql[i] != '\n' && sql[i] != '\r' {
			i++
		}
		return i, true
	case c == '/' && i+1 < n && sql[i+1] == '*':
		depth := 1
		i += 2
		for i < n && depth > 0 {
			switch {
			case i+1 < n && sql[i] == '/' && sql[i+1] == '*':
				depth++
				i += 2
			case i+1 < n && sql[i] == '*' && sql[i+1] == '/':
				depth--
				i += 2
			default:
				i++
			}
		}
		return i, true
	}
	return i, false
}

func isLetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }
func isDigit(b byte) bool  { return b >= '0' && b <= '9' }

// isIdentByte reports whether b can continue a SQL identifier (or a '$', so that a
// literal "$__qdbTimeParam(...)" in user SQL is not mistaken for a marker either).
func isIdentByte(b byte) bool { return isLetter(b) || isDigit(b) || b == '_' || b == '$' }

// hasExecutableDollarPlaceholder reports whether sql contains a "$<digit>" positional
// placeholder in an executable position (outside strings, identifiers and comments).
// sqlds never supplies bind arguments of its own, so such a placeholder is hand-written;
// parameterizing the time bounds on top of it would renumber or collide with it and
// silently bind the time value to the wrong site. An identifier containing "$<digit>"
// (e.g. "a$1b", legal in QuestDB) also matches — that conservatively inlines the bounds
// as literals, trading the plan-cache win for correctness.
func hasExecutableDollarPlaceholder(sql string) bool {
	if !strings.ContainsRune(sql, '$') {
		return false
	}
	n := len(sql)
	for i := 0; i < n; {
		if next, skipped := skipQuotedOrComment(sql, i); skipped {
			i = next
			continue
		}
		if sql[i] == '$' && i+1 < n && isDigit(sql[i+1]) {
			return true
		}
		i++
	}
	return false
}

// findTimeParamMarkers scans sql once, left to right, and returns every time-param
// marker in text order, each tagged with whether it sits in an executable position.
// Markers inside string literals, quoted identifiers or comments are reported with
// executable=false: sqlds macro interpolation is purely textual, so a time macro can
// expand into those regions, and such a marker must be inlined as a literal (a $N there
// would be literal text, not a bind parameter). All non-marker text is reported back
// verbatim by the callers, which copy the original bytes between spans.
func findTimeParamMarkers(sql string) []markerSpan {
	if !strings.Contains(sql, timeParamMarkerOpen) {
		return nil
	}
	var spans []markerSpan
	n := len(sql)
	for i := 0; i < n; {
		if end, region := skipQuotedOrComment(sql, i); region {
			for j := i; j < end; {
				if m, ok := parseMarker(sql, j); ok && m.end <= end {
					spans = append(spans, m) // executable stays false
					j = m.end
					continue
				}
				j++
			}
			i = end
			continue
		}
		if m, ok := parseMarker(sql, i); ok {
			m.executable = true
			spans = append(spans, m)
			i = m.end
			continue
		}
		i++
	}
	return spans
}

// hasStatementSeparator reports whether sql contains a statement separator: a ';' in an
// executable position (outside any string, identifier or comment) that is followed by
// further content. A trailing ';' — followed only by whitespace and/or comments — is
// not a separator; a quoted token IS content (QuestDB runs a bare quoted table name as
// an implicit SELECT, so `select 1; "trades"` is two statements). Such multi-statement
// SQL cannot be parameterized, because the PostgreSQL extended protocol rejects
// multiple statements in one Parse.
func hasStatementSeparator(sql string) bool {
	if !strings.ContainsRune(sql, ';') {
		return false
	}
	n := len(sql)
	for i := 0; i < n; {
		if next, skipped := skipQuotedOrComment(sql, i); skipped {
			i = next
			continue
		}
		if sql[i] == ';' {
			for j := i + 1; j < n; {
				if next, skipped := skipQuotedOrComment(sql, j); skipped {
					if sql[j] == '\'' || sql[j] == '"' {
						// A quoted token is real content — the start of a second
						// statement — unlike a comment, which is ignorable.
						return true
					}
					j = next
					continue
				}
				if !isSQLSpace(sql[j]) {
					return true
				}
				j++
			}
			return false
		}
		i++
	}
	return false
}

func isSQLSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' || b == '\v'
}

// parseMarker attempts to parse a marker at sql[i]. It returns the span and true on
// success, or false if sql[i] does not begin with timeParamMarkerOpen, sits mid-token
// (the preceding byte continues an identifier, e.g. "my__qdbTimeParam(42)" names a UDF,
// not a marker), or the bytes after the open paren are not "<optional sign><digits>)".
// The returned span is executable=false; callers set it as appropriate.
func parseMarker(sql string, i int) (markerSpan, bool) {
	if !strings.HasPrefix(sql[i:], timeParamMarkerOpen) {
		return markerSpan{}, false
	}
	if i > 0 && isIdentByte(sql[i-1]) {
		return markerSpan{}, false
	}
	n := len(sql)
	numStart := i + len(timeParamMarkerOpen)
	j := numStart
	if j < n && (sql[j] == '-' || sql[j] == '+') {
		j++
	}
	digitsStart := j
	for j < n && sql[j] >= '0' && sql[j] <= '9' {
		j++
	}
	if j == digitsStart || j >= n || sql[j] != ')' {
		return markerSpan{}, false
	}
	v, err := strconv.ParseInt(sql[numStart:j], 10, 64)
	if err != nil {
		return markerSpan{}, false
	}
	return markerSpan{start: i, end: j + 1, value: v}, true
}

// rewriteTimeParams replaces each time-param marker in an executable position with a
// positional bind placeholder ($1, $2, ... numbered in left-to-right text order) and
// returns those bound values in order; a marker that a macro expanded inside a
// string/identifier/comment is replaced with its bare literal value instead, since a
// placeholder there would be literal text rather than a parameter. sqlds never supplies
// bind arguments of its own (the args come exclusively from the driver's SetQueryArgs),
// so numbering always starts at $1. Marker-free SQL is returned unchanged.
func rewriteTimeParams(sql string) (string, []int64) {
	spans := findTimeParamMarkers(sql)
	if len(spans) == 0 {
		return sql, nil
	}
	var values []int64
	var b strings.Builder
	b.Grow(len(sql))
	prev := 0
	for _, s := range spans {
		b.WriteString(sql[prev:s.start])
		if s.executable {
			values = append(values, s.value)
			b.WriteString("$" + strconv.Itoa(len(values))) // text-order numbering
		} else {
			b.WriteString(strconv.FormatInt(s.value, 10))
		}
		prev = s.end
	}
	b.WriteString(sql[prev:])
	return b.String(), values
}

// paramMacros is the macro set ParameterizeTimeMacros interpolates with: the same set
// the driver registers with sqlds (macroSet with the literal renderer), except the
// time-bound macros wrap their value in a marker for the rewrite step.
var paramMacros = macroSet(timeParamMarker)

// ParameterizeTimeMacros interpolates the query's macros the same way sqlds would, but
// with the time-bound macros ($__fromTime, $__toTime, $__timeFilter) expanding into
// positional bind placeholders ($1, $2, ... in text order); the corresponding
// microsecond-epoch values are returned alongside (always at least one). The
// interpolated SQL is byte-stable across dashboard refreshes — only the values change —
// so QuestDB serves it from its compiled-plan cache instead of recompiling on every
// refresh.
//
// It returns ("", nil, nil) when the query must NOT be parameterized; the caller then
// leaves the query untouched and sqlds's own interpolation pass inlines the bounds as
// literals, reproducing the pre-parameterization behavior exactly. That fallback covers:
//   - multi-statement SQL: binding args switches lib/pq to the extended protocol, which
//     rejects multiple statements in one Parse;
//   - SQL already carrying a hand-written $N placeholder: a generated placeholder would
//     collide with it and silently bind the time value to the wrong site;
//   - every time macro expanded inside a string literal, identifier or comment: a $N
//     there would be literal text, not a parameter (and QuestDB rejects bind args the
//     statement does not reference);
//   - SQL that already contains the internal sentinel text (or where interpolation
//     produced one the scanner cannot attribute): user text and macro output would be
//     indistinguishable, so nothing is rewritten;
//   - a Table/Column model field containing macro text ($__...): it splices in after
//     the time macros already ran, so only sqlds's second interpolation pass would
//     expand it — silently executing SQL the single-pass pipeline rejects.
//
// Interpolation errors are returned as-is; callers fall back the same way and sqlds's
// own interpolation pass surfaces the error through its usual path.
func ParameterizeTimeMacros(query *sqlutil.Query) (string, []int64, error) {
	if strings.Contains(query.RawSQL, timeParamMarkerOpen) ||
		strings.Contains(query.Table, timeParamMarkerOpen) ||
		strings.Contains(query.Column, timeParamMarkerOpen) {
		// The query model already contains the sentinel text; rewriting could silently
		// turn user text into a bind placeholder (or corrupt it inside a string
		// literal). Table and Column are checked too: sqlutil's default $__table and
		// $__column macros splice those user-controlled fields into the SQL after this
		// guard would otherwise have run.
		return "", nil, nil
	}
	if strings.Contains(query.Table, "$__") || strings.Contains(query.Column, "$__") {
		// Macro text in Table/Column would splice in too late to be expanded here:
		// $__table and $__column run after the longer-named time macros within one
		// interpolation pass, so the spliced text would survive this pass — and then
		// be expanded by sqlds's second interpolation pass over the rewritten SQL.
		// The single-pass pipeline sends such text to the server verbatim (which
		// rejects it); falling back preserves exactly that behavior.
		return "", nil, nil
	}
	interpolated, err := sqlutil.Interpolate(query, paramMacros)
	if err != nil {
		return "", nil, err
	}
	if !strings.Contains(interpolated, timeParamMarkerOpen) {
		return "", nil, nil
	}
	if hasStatementSeparator(interpolated) || hasExecutableDollarPlaceholder(interpolated) {
		return "", nil, nil
	}
	rewritten, values := rewriteTimeParams(interpolated)
	if len(values) == 0 {
		// No marker sat in an executable position (all expanded into strings/comments).
		// The second arm is pure insurance: with the current macro output shape
		// ("cast(<marker> as timestamp)") and the pre-guard above, every marker in the
		// interpolated text parses and gets replaced — but if a future macro shape ever
		// lets one survive, running unmodified beats shipping the sentinel.
		return "", nil, nil
	}
	return rewritten, values, nil
}
