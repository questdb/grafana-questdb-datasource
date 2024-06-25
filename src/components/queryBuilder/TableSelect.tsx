import React, { useState, useEffect } from 'react';
import { Select } from '@grafana/ui';
import { SelectableValue } from '@grafana/data';
import { Datasource } from '../../data/QuestDbDatasource';
import { selectors } from './../../selectors';
import { EditorField } from '@grafana/plugin-ui';
import { BuilderMode } from '../../types';

export type Props = {
  datasource: Datasource;
  table?: string;
  onTableChange: (value: string, desginatedTimestamp?: string) => void;
  mode: BuilderMode;
};

export const TableSelect = (props: Props) => {
  const { datasource, onTableChange, table } = props;
  const [list, setList] = useState<Array<SelectableValue<string>>>([]);
  const { label, tooltip } = selectors.components.QueryEditor.QueryBuilder.FROM;
  const [map, setMap] = useState<Map<string, string>>(new Map());

  useEffect(() => {
    async function fetchTables() {
      const tables = (await datasource.fetchTables()).sort((a, b) => a.tableName.localeCompare(b.tableName));
      const values = tables.map((t) => ({ label: t.tableName, value: t.tableName }));
      // Add selected value to the list if it does not exist.
      if (table && !tables.find((x) => x.tableName === table) && props.mode !== BuilderMode.Trend) {
        values.push({ label: table!, value: table! });
      }

      const map = new Map<string, string>();
      tables.forEach((t) => {
        map.set(t.tableName, t.designatedTimestamp);
      });
      setMap(map);

      // TODO - can't seem to reset the select to unselected
      values.push({ label: '-- Choose --', value: '' });
      setList(values);
    }
    fetchTables();
  }, [datasource, table, props.mode]);

  const onChange = (table: string, desginatedTimestamp?: string) => {
    onTableChange(table, desginatedTimestamp);
  };

  return (
    <EditorField tooltip={tooltip} label={label}>
      <Select
        onChange={(e) => onChange(e.value ? e.value : '', e.value ? map.get(e.value) : undefined)}
        options={list}
        value={table}
        allowCustomValue={true}
        width={25}
      ></Select>
    </EditorField>
  );
};
