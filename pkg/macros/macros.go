package macros

import (
	"fmt"
	"math"
	
	"github.com/grafana/grafana-plugin-sdk-go/data/sqlutil"
	"github.com/grafana/sqlds/v4"
)

type timeQueryType string

const (
	timeQueryTypeFrom timeQueryType = "from"
	timeQueryTypeTo   timeQueryType = "to"
)

func newTimeFilter(queryType timeQueryType, query *sqlds.Query) (string, error) {
	date := query.TimeRange.From
	if queryType == timeQueryTypeTo {
		date = query.TimeRange.To
	}

	return fmt.Sprintf("cast(%d as timestamp)", date.UnixMicro()), nil
}

// FromTimeFilter return time filter query based on grafana's timepicker's from time
func FromTimeFilter(query *sqlds.Query, args []string) (string, error) {
	return newTimeFilter(timeQueryTypeFrom, query)
}

// ToTimeFilter return time filter query based on grafana's timepicker's to time
func ToTimeFilter(query *sqlds.Query, args []string) (string, error) {
	return newTimeFilter(timeQueryTypeTo, query)
}

func TimeFilter(query *sqlds.Query, args []string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("%w: expected 1 argument, received %d", sqlutil.ErrorBadArgumentCount, len(args))
	}

	var (
		column = args[0]
		from   = query.TimeRange.From.UTC().UnixMicro()
		to     = query.TimeRange.To.UTC().UnixMicro()
	)

	return fmt.Sprintf("%s >= cast(%d as timestamp) AND %s <= cast(%d as timestamp)", column, from, column, to), nil
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
