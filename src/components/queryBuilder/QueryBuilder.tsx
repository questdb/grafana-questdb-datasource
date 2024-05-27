import React, { useEffect, useState } from 'react';
import defaultsDeep from 'lodash/defaultsDeep';
import { Datasource } from '../../data/QuestDbDatasource';
import { TableSelect } from './TableSelect';
import { ModeEditor } from './ModeEditor';
import { FieldsEditor } from './Fields';
import { MetricsEditor } from './Metrics';
import { SampleByAlignEditor } from './SampleByAlignment';
import { FiltersEditor } from './Filters';
import { GroupByEditor } from './GroupBy';
import { getOrderByFields, OrderByEditor } from './OrderBy';
import { LimitEditor } from './Limit';
import {
  BuilderMetricField,
  BuilderMode,
  defaultBuilderQuery,
  Filter,
  Format,
  FullField,
  OrderBy,
  SampleByAlignToMode,
  SqlBuilderOptions,
  SqlBuilderOptionsTrend,
} from '../../types';
import { isDateType /*, isTimestampType*/ } from './utils';
//import {selectors} from '../../selectors';
import { CoreApp } from '@grafana/data';
import { EditorFieldGroup, EditorRow, EditorRows } from '@grafana/plugin-ui';
import { selectors } from '../../selectors';
import { SampleByFillEditor } from './SampleByFillEditor';
import { PartitionByEditor } from './PartitionByEditor';

interface QueryBuilderProps {
  builderOptions: SqlBuilderOptions;
  onBuilderOptionsChange: (builderOptions: SqlBuilderOptions) => void;
  datasource: Datasource;
  format: Format;
  app: CoreApp | undefined;
}

export const QueryBuilder = (props: QueryBuilderProps) => {
  const [baseFieldsList, setBaseFieldsList] = useState<FullField[]>([]);
  const builder = defaultsDeep(props.builderOptions, defaultBuilderQuery.builderOptions);
  useEffect(() => {
    const fetchBaseFields = async (table: string) => {
      props.datasource
        .fetchFields(table)
        .then(async (fields) => {
          setBaseFieldsList(fields);

          // When changing from SQL Editor to Query Builder, we need to find out if the
          // first value is a timestamp or date, so we can change the mode to Time Series
          if (builder.fields?.length > 0) {
            const fieldName = builder.fields[0];
            const timeFields = fields.filter((f) => isDateType(f.type));
            const timeField = timeFields.find((x) => x.name === fieldName);
            if (timeField) {
              const queryOptions: SqlBuilderOptions = {
                ...builder,
                timeField: timeField.name,
                mode: BuilderMode.Trend,
                fields: builder.fields.slice(1, builder.fields.length),
              };
              props.onBuilderOptionsChange(queryOptions);
            }
          }
        })
        .catch((ex: any) => {
          console.error(ex);
          throw ex;
        });
    };

    if (builder.table) {
      fetchBaseFields(builder.table);
    }
    // We want to run this only when the table changes or first time load.
    // If we add 'builder.fields' / 'builder.groupBy' / 'builder.metrics' / 'builder.filters' to the deps array, this will be called every time query editor changes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [props.datasource, builder.table]);

  const onTableChange = (table = '', designatedTimestamp?: string) => {
    if (table === builder.table) {
      return;
    }
    setBaseFieldsList([]);
    const queryOptions: SqlBuilderOptions = {
      ...builder,
      table,
      fields: [],
      filters: [],
      groupBy: [],
      orderBy: [],
      timeField: designatedTimestamp,
      partitionBy: [],
    };
    props.onBuilderOptionsChange(queryOptions);
  };

  const onModeChange = (mode: BuilderMode) => {
    if (mode === BuilderMode.List) {
      const queryOptions: SqlBuilderOptions = { ...builder, mode, fields: builder.fields || [], orderBy: [] };
      props.onBuilderOptionsChange(queryOptions);
    } else if (mode === BuilderMode.Aggregate) {
      const queryOptions: SqlBuilderOptions = {
        ...builder,
        mode,
        orderBy: [],
        metrics: builder.metrics || [],
      };
      props.onBuilderOptionsChange(queryOptions);
    } else if (mode === BuilderMode.Trend) {
      const queryOptions: SqlBuilderOptionsTrend = {
        ...builder,
        mode: BuilderMode.Trend,
        timeField: builder.timeField || '',
        metrics: builder.metrics || [],
        sampleByAlignTo: builder.sampleByAlignTo || SampleByAlignToMode.Calendar,
      };
      props.onBuilderOptionsChange(queryOptions);
    }
  };

  const onFieldsChange = (fields: string[] = []) => {
    const queryOptions: SqlBuilderOptions = { ...builder, fields };
    props.onBuilderOptionsChange(queryOptions);
  };

  const onFillChange = (sampleByFill: string[] = []) => {
    const queryOptions: SqlBuilderOptions = { ...builder, sampleByFill };
    props.onBuilderOptionsChange(queryOptions);
  };

  const onLatestOnPartitionByChange = (partitionBy: string[] = []) => {
    const queryOptions: SqlBuilderOptions = { ...builder, partitionBy };
    props.onBuilderOptionsChange(queryOptions);
  };

  const onMetricsChange = (metrics: BuilderMetricField[] = []) => {
    const queryOptions: SqlBuilderOptions = { ...builder, metrics };
    props.onBuilderOptionsChange(queryOptions);
  };

  const onFiltersChange = (filters: Filter[] = []) => {
    const queryOptions: SqlBuilderOptions = { ...builder, filters };
    props.onBuilderOptionsChange(queryOptions);
  };

  const onGroupByChange = (groupBy: string[] = []) => {
    const queryOptions: SqlBuilderOptions = { ...builder, groupBy };
    props.onBuilderOptionsChange(queryOptions);
  };

  const onSampleByAlignToFieldChange = (sampleByAlignTo = '') => {
    let sampleByAlignToValue = builder.sampleByAlignToValue;
    if (sampleByAlignToValue === undefined || sampleByAlignToValue.length === 0) {
      if (sampleByAlignTo === SampleByAlignToMode.CalendarOffset) {
        sampleByAlignToValue = '00:00';
      } else if (sampleByAlignTo === SampleByAlignToMode.CalendarTimeZone) {
        sampleByAlignToValue = 'UTC';
      }
    } else if (
      sampleByAlignTo === SampleByAlignToMode.Calendar ||
      sampleByAlignTo === SampleByAlignToMode.FirstObservation
    ) {
      sampleByAlignToValue = '';
    } else if (
      sampleByAlignTo === SampleByAlignToMode.CalendarOffset &&
      !sampleByAlignToValue.matches('(-|+)[[0-9][0-9]:[0-9][0-9]')
    ) {
      sampleByAlignToValue = '00:00';
    } else if (sampleByAlignTo === SampleByAlignToMode.CalendarTimeZone && !sampleByAlignToValue.matches('[A-Z]+')) {
      sampleByAlignToValue = 'UTC';
    }

    const queryOptions: SqlBuilderOptions = { ...builder, sampleByAlignTo, sampleByAlignToValue };
    props.onBuilderOptionsChange(queryOptions);
  };

  const onSampleByAlignToValueChange = (sampleByAlignToValue = '') => {
    const queryOptions: SqlBuilderOptions = { ...builder, sampleByAlignToValue };
    props.onBuilderOptionsChange(queryOptions);
  };

  const onOrderByChange = (orderBy: OrderBy[] = []) => {
    const queryOptions: SqlBuilderOptions = { ...builder, orderBy };
    props.onBuilderOptionsChange(queryOptions);
  };

  const onLimitChange = (limit = '100') => {
    const queryOptions: SqlBuilderOptions = { ...builder, limit };
    props.onBuilderOptionsChange(queryOptions);
  };

  const getFieldList = (): FullField[] => {
    const newArray: FullField[] = [];
    baseFieldsList.forEach((bf) => {
      newArray.push(bf);
    });
    return newArray;
  };
  const getFieldListWithAllOption = (): FullField[] => {
    const newArray: FullField[] = [];
    newArray.push({ name: '*', label: 'ALL', type: 'string', picklistValues: [] });
    baseFieldsList.forEach((bf) => {
      newArray.push(bf);
    });
    return newArray;
  };
  const fieldsList = getFieldList();
  const fieldsListWithAll = getFieldListWithAllOption();
  return builder ? (
    <EditorRows>
      <EditorRow>
        <EditorFieldGroup>
          <TableSelect
            datasource={props.datasource}
            table={builder.table}
            onTableChange={onTableChange}
            mode={builder.mode}
          />
          <ModeEditor mode={builder.mode} onModeChange={onModeChange} />
        </EditorFieldGroup>
      </EditorRow>

      {builder.mode === BuilderMode.Trend && (
        <EditorRow>
          <GroupByEditor
            groupBy={builder.groupBy || []}
            onGroupByChange={onGroupByChange}
            fieldsList={fieldsList}
            labelAndTooltip={selectors.components.QueryEditor.QueryBuilder.SAMPLE_BY}
          />
        </EditorRow>
      )}

      {builder.mode !== BuilderMode.Trend && (
        <EditorRow>
          <FieldsEditor fields={builder.fields || []} onFieldsChange={onFieldsChange} fieldsList={fieldsListWithAll} />
        </EditorRow>
      )}

      {(builder.mode === BuilderMode.Aggregate || builder.mode === BuilderMode.Trend) && (
        <EditorRow>
          <MetricsEditor metrics={builder.metrics || []} onMetricsChange={onMetricsChange} fieldsList={fieldsList} />
        </EditorRow>
      )}
      <EditorRow>
        <FiltersEditor filters={builder.filters || []} onFiltersChange={onFiltersChange} fieldsList={fieldsList} />
      </EditorRow>

      {builder.mode === BuilderMode.Trend && (
        <EditorRow>
          <SampleByAlignEditor
            timeField={builder.timeField}
            fieldsList={fieldsList}
            sampleByAlignToMode={builder.sampleByAlignTo}
            sampleByAlignToValue={builder.sampleByAlignToValue}
            onSampleByAlignToModeChange={onSampleByAlignToFieldChange}
            onSampleByAlignToValueChange={onSampleByAlignToValueChange}
          />
        </EditorRow>
      )}

      {builder.mode === BuilderMode.Trend && (
        <EditorRow>
          <SampleByFillEditor fills={builder.sampleByFill || []} onFillsChange={onFillChange} />
        </EditorRow>
      )}

      {builder.mode === BuilderMode.Aggregate && (
        <EditorRow>
          <GroupByEditor
            groupBy={builder.groupBy || []}
            onGroupByChange={onGroupByChange}
            fieldsList={fieldsList}
            labelAndTooltip={selectors.components.QueryEditor.QueryBuilder.GROUP_BY}
          />
        </EditorRow>
      )}

      {builder.mode === BuilderMode.List && (
        <PartitionByEditor
          fields={builder.partitionBy || []}
          fieldsList={fieldsList}
          onFieldsChange={onLatestOnPartitionByChange}
          timeField={builder.timeField}
          isDisabled={builder.timeField.length === 0}
        />
      )}

      <OrderByEditor
        orderBy={builder.orderBy || []}
        onOrderByItemsChange={onOrderByChange}
        fieldsList={getOrderByFields(builder, fieldsList)}
      />
      <EditorRow>
        <LimitEditor limit={builder.limit || 100} onLimitChange={onLimitChange} />
      </EditorRow>
    </EditorRows>
  ) : null;
};
