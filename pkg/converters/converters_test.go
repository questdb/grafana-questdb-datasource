package converters_test

import (
	"testing"
	"time"

	"github.com/questdb/grafana-questdb-datasource/pkg/converters"
	"github.com/stretchr/testify/assert"
)

func TestTimestamp(t *testing.T) {
	d, _ := time.Parse("2006-01-02T15:04:05.000000Z", "2014-11-12T11:45:26.371123Z")
	in := &d
	out, err := converters.GetConverter("TIMESTAMP").FrameConverter.ConverterFunc(&in)
	assert.Nil(t, err)
	actual := out.(*time.Time)
	assert.Equal(t, in, actual)
}

func TestEmptyTimestampShouldBeNil_1(t *testing.T) {
	var in *time.Time
	out, err := converters.GetConverter("TIMESTAMP").FrameConverter.ConverterFunc(&in)
	assert.Nil(t, err)
	assert.Nil(t, out)
}

func TestEmptyTimestampShouldBeNil_2(t *testing.T) {
	var in **time.Time
	out, err := converters.GetConverter("TIMESTAMP").FrameConverter.ConverterFunc(in)
	assert.Nil(t, err)
	assert.Nil(t, out)
}

func TestFloat4(t *testing.T) {
	val := float32(12.045)
	in := &val
	out, err := converters.GetConverter("FLOAT4").FrameConverter.ConverterFunc(in)
	assert.Nil(t, err)
	actual := out.(float32)
	assert.Equal(t, in, &actual)
}

func TestEmptyFloat4ShouldBeNil(t *testing.T) {
	var in *float32
	out, err := converters.GetConverter("FLOAT4").FrameConverter.ConverterFunc(&in)
	assert.Nil(t, err)
	assert.Nil(t, out)
}

func TestFloat8(t *testing.T) {
	val := float64(120.041237)
	in := &val
	out, err := converters.GetConverter("FLOAT8").FrameConverter.ConverterFunc(in)
	assert.Nil(t, err)
	actual := out.(float64)
	assert.Equal(t, in, &actual)
}

func TestEmptyFloat8ShouldBeNil(t *testing.T) {
	var in *float64
	out, err := converters.GetConverter("FLOAT8").FrameConverter.ConverterFunc(&in)
	assert.Nil(t, err)
	assert.Nil(t, out)
}
