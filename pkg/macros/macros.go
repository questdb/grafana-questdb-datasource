package macros

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/grafana/grafana-plugin-sdk-go/data/sqlutil"
	"github.com/grafana/sqlds/v4"
)

type timeQueryType string

const (
	timeQueryTypeFrom timeQueryType = "from"
	timeQueryTypeTo   timeQueryType = "to"
)

// timeBoundMacros are the macros that expand a dashboard time bound into the SQL, each
// parameterized by how the microsecond-epoch value is rendered. This map is the single
// source of the time-bound macro names for macroSet, HasTimeBoundMacro and the
// parameterizing macro set in timeparam.go — sqlutil.Interpolate merges sqlutil's
// default macros underneath these names, so a name present in one set but not another
// would silently fall back to a default implementation with different SQL output.
var timeBoundMacros = map[string]func(render func(int64) string) sqlutil.MacroFunc{
	"fromTime": func(render func(int64) string) sqlutil.MacroFunc {
		return func(query *sqlutil.Query, args []string) (string, error) {
			return newTimeFilter(timeQueryTypeFrom, query, render)
		}
	},
	"toTime": func(render func(int64) string) sqlutil.MacroFunc {
		return func(query *sqlutil.Query, args []string) (string, error) {
			return newTimeFilter(timeQueryTypeTo, query, render)
		}
	},
	"timeFilter": func(render func(int64) string) sqlutil.MacroFunc {
		return func(query *sqlutil.Query, args []string) (string, error) {
			return timeFilter(query, args, render)
		}
	},
}

// macroSet returns the driver's macro set with the time-bound macros rendering their
// microsecond-epoch values through render: literalMicros for the set registered with
// sqlds (QuestDB.Macros), timeParamMarker for the parameterizing pass (timeparam.go).
func macroSet(render func(int64) string) sqlutil.Macros {
	m := sqlutil.Macros{"sampleByInterval": SampleByInterval}
	for name, build := range timeBoundMacros {
		m[name] = build(render)
	}
	return m
}

// LiteralMacros is the macro set the driver registers with sqlds: time bounds inlined
// as microsecond-epoch literals, the pre-parameterization behavior. It serves as the
// fallback interpolation path when MutateQuery skips parameterization.
func LiteralMacros() sqlutil.Macros {
	return macroSet(literalMicros)
}

// HasTimeBoundMacro reports whether rawSQL mentions any time-bound macro, e.g.
// "$__fromTime". It is the cheap gate deciding whether parameterization can pay off.
func HasTimeBoundMacro(rawSQL string) bool {
	for name := range timeBoundMacros {
		if strings.Contains(rawSQL, "$__"+name) {
			return true
		}
	}
	return false
}

// literalMicros renders a time bound the way the macros always did: as a bare
// microsecond-epoch literal. ParameterizeTimeMacros uses timeParamMarker instead,
// turning the bound into a bind parameter (see timeparam.go).
func literalMicros(micros int64) string {
	return strconv.FormatInt(micros, 10)
}

func newTimeFilter(queryType timeQueryType, query *sqlds.Query, render func(int64) string) (string, error) {
	date := query.TimeRange.From
	if queryType == timeQueryTypeTo {
		date = query.TimeRange.To
	}

	return fmt.Sprintf("cast(%s as timestamp)", render(date.UnixMicro())), nil
}

func timeFilter(query *sqlds.Query, args []string, render func(int64) string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("%w: expected 1 argument, received %d", sqlutil.ErrorBadArgumentCount, len(args))
	}

	var (
		column = args[0]
		from   = query.TimeRange.From.UTC().UnixMicro()
		to     = query.TimeRange.To.UTC().UnixMicro()
	)

	return fmt.Sprintf("%s >= cast(%s as timestamp) AND %s <= cast(%s as timestamp)", column, render(from), column, render(to)), nil
}

func SampleByInterval(query *sqlds.Query, args []string) (string, error) {
	hours := int(math.Max(query.Interval.Hours(), 0))
	seconds := int(math.Max(query.Interval.Seconds(), 0))
	if hours > 0 {
		if hours >= 24 && query.Interval.Hours() == float64(hours) {
			return fmt.Sprintf("%dd", hours/24), nil
		}
		return fmt.Sprintf("%dh", hours), nil
	} else if seconds > 0 {
		return fmt.Sprintf("%ds", seconds), nil
	} else {
		millis := query.Interval.Milliseconds()
		if millis < 1 {
			millis = 1
		}
		return fmt.Sprintf("%dT", millis), nil
	}
}
