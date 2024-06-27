import React from 'react';
import { QueryEditorProps, VariableWithMultiSupport } from '@grafana/data';
import { Datasource } from '../data/QuestDbDatasource';
import {
  BuilderMode,
  QuestDBConfig,
  QuestDBQuery,
  defaultBuilderQuery,
  Format,
  QueryType,
  SqlBuilderOptions,
  QuestDBBuilderQuery,
} from '../types';
import { SQLEditor } from 'components/SQLEditor';
import { getSQLFromQueryOptions } from 'components/queryBuilder/utils';
import { QueryBuilder } from 'components/queryBuilder/QueryBuilder';
import { Preview } from 'components/queryBuilder/Preview';
import { getFormat } from 'components/editor';
import { QueryHeader } from 'components/QueryHeader';
import { getTemplateSrv } from '@grafana/runtime';

export type QuestDBQueryEditorProps = QueryEditorProps<Datasource, QuestDBQuery, QuestDBConfig>;

const QuestDBEditorByType = (props: QuestDBQueryEditorProps) => {
  const { query, onChange, app } = props;
  const onBuilderOptionsChange = (builderOptions: SqlBuilderOptions) => {
    const templateVars = getTemplateSrv().getVariables() as VariableWithMultiSupport[];
    const sql = getSQLFromQueryOptions(builderOptions, templateVars);
    const format =
      query.selectedFormat === Format.AUTO
        ? builderOptions.mode === BuilderMode.Trend
          ? Format.TIMESERIES
          : Format.TABLE
        : query.selectedFormat;

    onChange({ ...query, queryType: QueryType.Builder, rawSql: sql, builderOptions, format });
  };

  switch (query.queryType) {
    case QueryType.SQL:
      return (
        <div data-testid="query-editor-section-sql">
          <SQLEditor {...props} />
        </div>
      );
    case QueryType.Builder:
    default:
      let newQuery: QuestDBBuilderQuery = { ...query };
      if (query.rawSql && !query.builderOptions) {
        return (
          <div data-testid="query-editor-section-sql">
            <SQLEditor {...props} />
          </div>
        );
      }
      if (!query.rawSql || !query.builderOptions) {
        newQuery = {
          ...newQuery,
          rawSql: defaultBuilderQuery.rawSql,
          builderOptions: {
            ...defaultBuilderQuery.builderOptions,
          },
        };
      }
      return (
        <div data-testid="query-editor-section-builder">
          <QueryBuilder
            datasource={props.datasource}
            builderOptions={newQuery.builderOptions}
            onBuilderOptionsChange={onBuilderOptionsChange}
            format={newQuery.format}
            app={app}
          />
          <Preview sql={newQuery.rawSql} />
        </div>
      );
  }
};

export const QuestDBQueryEditor = (props: QuestDBQueryEditorProps) => {
  const { query, onChange, onRunQuery } = props;

  React.useEffect(() => {
    if (typeof query.selectedFormat === 'undefined' && query.queryType === QueryType.SQL) {
      const selectedFormat = Format.AUTO;
      const format = getFormat(query.rawSql, selectedFormat);
      onChange({ ...query, selectedFormat, format });
    }
  }, [query, onChange]);

  return (
    <>
      <QueryHeader query={query} onChange={onChange} onRunQuery={onRunQuery} datasource={props.datasource} />
      <QuestDBEditorByType {...props} />
    </>
  );
};
