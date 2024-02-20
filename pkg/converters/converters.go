package converters

import (
	"fmt"
	"reflect"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/grafana/grafana-plugin-sdk-go/data/sqlutil"
)

type Converter struct {
	convert   func(in interface{}) (interface{}, error)
	fieldType data.FieldType
	scanType  reflect.Type
}

var Converters = map[string]Converter{
	"BOOL": {
		fieldType: data.FieldTypeBool,
		scanType:  reflect.PtrTo(reflect.TypeOf(bool(false))),
	},
	"INT2": {
		fieldType: data.FieldTypeInt16,
		scanType:  reflect.PtrTo(reflect.TypeOf(int16(0))),
	},
	"FLOAT4": {
		//convert:   floatNullableConvert,
		fieldType: data.FieldTypeNullableFloat32,
		scanType:  reflect.PtrTo(reflect.PtrTo(reflect.TypeOf(float32(0)))),
	},
	"FLOAT8": {
		//convert:   doubleNullableConvert,
		fieldType: data.FieldTypeNullableFloat64,
		scanType:  reflect.PtrTo(reflect.PtrTo(reflect.TypeOf(float64(0)))),
	},
	"TIMESTAMP": {
		convert:   timestampNullableConvert,
		fieldType: data.FieldTypeNullableTime,
		scanType:  reflect.PtrTo(reflect.PtrTo(reflect.TypeOf(time.Time{}))),
	},
}

var QdbConverters = QuestDBConverters()

func QuestDBConverters() []sqlutil.Converter {
	var list []sqlutil.Converter
	for name, converter := range Converters {
		list = append(list, createConverter(name, converter))
	}
	/*
		// copied from pg core plugin
		pgConverters := []sqlutil.StringConverter{
			{
				Name:           "handle FLOAT4",
				InputScanKind:  reflect.Interface,
				InputTypeName:  "FLOAT4",
				ConversionFunc: func(in *string) (*string, error) { return in, nil },
				Replacer: &sqlutil.StringFieldReplacer{
					OutputFieldType: data.FieldTypeNullableFloat64,
					ReplaceFunc: func(in *string) (any, error) {
						if in == nil {
							return nil, nil
						}
						v, err := strconv.ParseFloat(*in, 64)
						if err != nil {
							return nil, err
						}
						return &v, nil
					},
				},
			},
			{
				Name:           "handle FLOAT8",
				InputScanKind:  reflect.Interface,
				InputTypeName:  "FLOAT8",
				ConversionFunc: func(in *string) (*string, error) { return in, nil },
				Replacer: &sqlutil.StringFieldReplacer{
					OutputFieldType: data.FieldTypeNullableFloat64,
					ReplaceFunc: func(in *string) (any, error) {
						if in == nil {
							return nil, nil
						}
						v, err := strconv.ParseFloat(*in, 64)
						if err != nil {
							return nil, err
						}
						return &v, nil
					},
				},
			},
			{
				Name:           "handle INT2",
				InputScanKind:  reflect.Interface,
				InputTypeName:  "INT2",
				ConversionFunc: func(in *string) (*string, error) { return in, nil },
				Replacer: &sqlutil.StringFieldReplacer{
					OutputFieldType: data.FieldTypeNullableInt16,
					ReplaceFunc: func(in *string) (any, error) {
						if in == nil {
							return nil, nil
						}
						i64, err := strconv.ParseInt(*in, 10, 16)
						if err != nil {
							return nil, err
						}
						v := int16(i64)
						return &v, nil
					},
				},
			},
		}

		for _, converter := range pgConverters {
			list = append(list, converter.ToConverter())
		}*/

	return list
}

func GetConverter(columnType string) sqlutil.Converter {
	converter, ok := Converters[columnType]
	if ok {
		return createConverter(columnType, converter)
	}
	for name, converter := range Converters {
		if name == columnType {
			return createConverter(name, converter)
		}
	}
	return sqlutil.Converter{}
}

func createConverter(name string, converter Converter) sqlutil.Converter {
	convert := defaultConvert
	if converter.convert != nil {
		convert = converter.convert
	}
	return sqlutil.Converter{
		Name:          name,
		InputScanType: converter.scanType,
		InputTypeName: name,
		FrameConverter: sqlutil.FrameConverter{
			FieldType:     converter.fieldType,
			ConverterFunc: convert,
		},
	}
}

func timestampNullableConvert(in interface{}) (interface{}, error) {
	if in == nil {
		return (*time.Time)(nil), nil
	}
	v, ok := in.(**time.Time)
	if !ok {
		return nil, fmt.Errorf("invalid timestamp - %v", in)
	}
	if v == nil || *v == nil {
		return (*time.Time)(nil), nil
	}
	f := (**v).UTC()
	return &f, nil
}

func defaultConvert(in interface{}) (interface{}, error) {
	if in == nil {
		return reflect.Zero(reflect.TypeOf(in)).Interface(), nil
	}
	return reflect.ValueOf(in).Elem().Interface(), nil
}
