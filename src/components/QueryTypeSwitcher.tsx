import React, { useState } from 'react';
import { SelectableValue, VariableWithMultiSupport } from '@grafana/data';
import { RadioButtonGroup, ConfirmModal } from '@grafana/ui';
import { getQueryOptionsFromSql, getSQLFromQueryOptions } from './queryBuilder/utils';
import { selectors } from './../selectors';
import { QuestDBQuery, QueryType, defaultBuilderQuery, SqlBuilderOptions, QuestDBSQLQuery } from 'types';
import { isString } from 'lodash';
import { Datasource } from '../data/QuestDbDatasource';
import { getTemplateSrv } from '@grafana/runtime';

interface QueryTypeSwitcherProps {
  query: QuestDBQuery;
  onChange: (query: QuestDBQuery) => void;
  datasource?: Datasource;
}

export const QueryTypeSwitcher = ({ query, onChange, datasource }: QueryTypeSwitcherProps) => {
  const { options: queryTypeLabels, switcher, cannotConvert } = selectors.components.QueryEditor.Types;
  let queryType: QueryType =
    query.queryType ||
    ((query as QuestDBSQLQuery).rawSql && !(query as QuestDBQuery).queryType ? QueryType.SQL : QueryType.Builder);
  const [editor, setEditor] = useState<QueryType>(queryType);
  const [confirmModalState, setConfirmModalState] = useState<boolean>(false);
  const [cannotConvertModalState, setCannotConvertModalState] = useState<boolean>(false);
  const options: Array<SelectableValue<QueryType>> = [
    { label: queryTypeLabels.SQLEditor, value: QueryType.SQL },
    { label: queryTypeLabels.QueryBuilder, value: QueryType.Builder },
  ];
  const [errorMessage, setErrorMessage] = useState<string>('');
  const templateVars = getTemplateSrv().getVariables() as VariableWithMultiSupport[];

  async function onQueryTypeChange(queryType: QueryType, confirm = false) {
    if (query.queryType === QueryType.SQL && queryType === QueryType.Builder && !confirm) {
      const queryOptionsFromSql = await getQueryOptionsFromSql(query.rawSql);
      if (isString(queryOptionsFromSql)) {
        setCannotConvertModalState(true);
        setErrorMessage(queryOptionsFromSql);
      } else {
        setConfirmModalState(true);
      }
    } else {
      setEditor(queryType);
      let builderOptions: SqlBuilderOptions;
      switch (query.queryType) {
        case QueryType.Builder:
          builderOptions = query.builderOptions;
          break;
        case QueryType.SQL:
          builderOptions =
            ((await getQueryOptionsFromSql(query.rawSql, datasource)) as SqlBuilderOptions) ||
            defaultBuilderQuery.builderOptions;
          break;
        default:
          builderOptions = defaultBuilderQuery.builderOptions;
          break;
      }
      if (queryType === QueryType.SQL) {
        onChange({
          ...query,
          queryType,
          rawSql: getSQLFromQueryOptions(builderOptions, templateVars),
          meta: { builderOptions },
          format: query.format,
          selectedFormat: query.selectedFormat,
        });
      } else if (queryType === QueryType.Builder) {
        onChange({ ...query, queryType, rawSql: getSQLFromQueryOptions(builderOptions, templateVars), builderOptions });
      }
    }
  }
  async function onConfirmQueryTypeChange() {
    await onQueryTypeChange(QueryType.Builder, true);
    setConfirmModalState(false);
    setCannotConvertModalState(false);
  }
  return (
    <>
      <RadioButtonGroup size="sm" options={options} value={editor} onChange={(e) => onQueryTypeChange(e!)} />
      <ConfirmModal
        isOpen={confirmModalState}
        title={switcher.title}
        body={switcher.body}
        confirmText={switcher.confirmText}
        dismissText={switcher.dismissText}
        icon="exclamation-triangle"
        onConfirm={onConfirmQueryTypeChange}
        onDismiss={() => setConfirmModalState(false)}
      />
      <ConfirmModal
        title={cannotConvert.title}
        body={`${errorMessage} \nDo you want to delete your current query and use the query builder?`}
        isOpen={cannotConvertModalState}
        icon="exclamation-triangle"
        onConfirm={onConfirmQueryTypeChange}
        confirmText={switcher.confirmText}
        onDismiss={() => setCannotConvertModalState(false)}
      />
    </>
  );
};
