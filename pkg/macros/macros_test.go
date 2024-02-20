package macros_test

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/sqlds/v2"
	"github.com/questdb/grafana-questdb-datasource/pkg/macros"
	"github.com/questdb/grafana-questdb-datasource/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type QuestDBDriver struct {
	sqlds.Driver
}

type MockDB struct {
	QuestDBDriver
}

func (h *QuestDBDriver) Macros() sqlds.Macros {
	var C = plugin.QuestDB{}

	return C.Macros()
}

func TestMacroFromTimeFilter(t *testing.T) {
	query := sqlds.Query{
		TimeRange: backend.TimeRange{},
		RawSQL:    "select foo from foo where bar > $__fromTime",
	}
	tests := []struct {
		want string
		from string
	}{
		{want: "cast(-315619200000000 as timestamp)", from: "1960-01-01T00:00:00.000Z"},
		{want: "cast(-1000 as timestamp)", from: "1969-12-31T23:59:59.999Z"},
		{want: "cast(0 as timestamp)", from: "1970-01-01T00:00:00.000Z"},
		{want: "cast(1000 as timestamp)", from: "1970-01-01T00:00:00.001Z"},
		{want: "cast(1705754096789000 as timestamp)", from: "2024-01-20T12:34:56.789Z"},
	}
	for i, tt := range tests {
		t.Run("FromTimeFilterTest_"+strconv.FormatInt(int64(i), 10), func(t *testing.T) {
			query.TimeRange.From, _ = time.Parse("2006-01-02T15:04:05.000Z", tt.from)
			got, err := macros.FromTimeFilter(&query, []string{})
			if err != nil {
				t.Errorf("macroFromTimeFilter error = %v", err)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMacroToTimeFilter(t *testing.T) {
	query := sqlds.Query{
		TimeRange: backend.TimeRange{},
		RawSQL:    "select foo from foo where bar < $__toTime",
	}
	tests := []struct {
		want string
		to   string
	}{
		{want: "cast(-315619200000000 as timestamp)", to: "1960-01-01T00:00:00.000Z"},
		{want: "cast(-1000 as timestamp)", to: "1969-12-31T23:59:59.999Z"},
		{want: "cast(0 as timestamp)", to: "1970-01-01T00:00:00.000Z"},
		{want: "cast(1000 as timestamp)", to: "1970-01-01T00:00:00.001Z"},
		{want: "cast(1705754096789000 as timestamp)", to: "2024-01-20T12:34:56.789Z"},
	}
	for i, tt := range tests {
		t.Run("FromTimeFilterTest_"+strconv.FormatInt(int64(i), 10), func(t *testing.T) {
			query.TimeRange.From, _ = time.Parse("2006-01-02T15:04:05.000Z", tt.to)
			got, err := macros.FromTimeFilter(&query, []string{})
			if err != nil {
				t.Errorf("macroToTimeFilter error = %v", err)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMacroInterval1Millis(t *testing.T) {
	query := sqlds.Query{
		RawSQL: "select *  from foo sample by $__interval",
	}

	tests := []struct {
		expected string
		input    int64
	}{
		{"1T", 1000000},
		{"459T", 459000000},
		{"1s", 1000000000},
		{"59s", 59 * 1000000000},
		{"1h", 3600000000000},
		{"23h", 23 * 3600000000000},
		{"1d", 86400000000000},
		{"5d", 5 * 86400000000000},
	}

	for _, data := range tests {
		t.Run("FromTimeFilterTest_"+data.expected, func(t *testing.T) {
			query.Interval = time.Duration(data.input)
			actual, err := macros.SampleByInterval(&query, []string{})
			if err != nil {
				t.Errorf("TestMacroInterval1Millis error = %v", err)
				return
			}
			assert.Equal(t, data.expected, actual)
		})
	}
}

func TestInterpolate(t *testing.T) {
	from, _ := time.Parse("2006-01-02T15:04:05.000Z", "2024-01-20T12:34:56.789Z")
	to, _ := time.Parse("2006-01-02T15:04:05.000Z", "2024-02-10T10:01:02.123Z")

	type test struct {
		input    string
		output   string
		duration time.Duration
	}

	tests := []test{
		{input: "select * from tab where tstmp >= $__fromTime ", output: "select * from tab where tstmp >= cast(1705754096789000 as timestamp) "},
		{input: "select * from tab where tstmp < $__toTime ", output: "select * from tab where tstmp < cast(1707559262123000 as timestamp) "},
		{input: "select * from tab where ( tstmp >= $__fromTime and tstmp <= $__toTime )", output: "select * from tab where ( tstmp >= cast(1705754096789000 as timestamp) and tstmp <= cast(1707559262123000 as timestamp) )"},
		{input: "select * from tab where ( tstmp >= $__fromTime ) and ( tstmp <= $__toTime )", output: "select * from tab where ( tstmp >= cast(1705754096789000 as timestamp) ) and ( tstmp <= cast(1707559262123000 as timestamp) )"},
		{input: "select * from tab where $__timeFilter(tstmp)", output: "select * from tab where tstmp >= 1705754096789000 AND tstmp <= 1707559262123000"},
		{input: "select * from tab where $__timeFilter( tstmp )", output: "select * from tab where tstmp >= 1705754096789000 AND tstmp <= 1707559262123000"},
		{input: "select * from tab where $__timeFilter( tstmp ) sample by $__sampleByInterval", output: "select * from tab where tstmp >= 1705754096789000 AND tstmp <= 1707559262123000 sample by 30s", duration: time.Duration(30000000000)},
		{input: "select * from tab where $__timeFilter( tstmp ) sample by $__sampleByInterval", output: "select * from tab where tstmp >= 1705754096789000 AND tstmp <= 1707559262123000 sample by 1T", duration: time.Duration(1000000)},
	}

	for i, tc := range tests {
		driver := MockDB{}
		t.Run(fmt.Sprintf("TestInterpolate [%d/%d]", i+1, len(tests)), func(t *testing.T) {
			query := &sqlds.Query{
				RawSQL: tc.input,
				Table:  "tab",
				Column: "tstmp",
				TimeRange: backend.TimeRange{
					From: from,
					To:   to,
				},
				Interval: tc.duration,
			}
			interpolatedQuery, err := sqlds.Interpolate(&driver, query)
			require.Nil(t, err)
			assert.Equal(t, tc.output, interpolatedQuery)
		})
	}
}
