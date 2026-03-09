import React from 'react';
import { render } from '@testing-library/react';
import { QueryBuilder } from './QueryBuilder';
import { Datasource } from '../../data/QuestDbDatasource';
import { BuilderMode, Format } from 'types';
import { CoreApp } from '@grafana/data';

describe('QueryBuilder', () => {
  it('renders correctly', async () => {
    const mockDs = {
      settings: { jsonData: {} },
      fetchTables: jest.fn(() => Promise.resolve([])),
      fetchFields: jest.fn(() => Promise.resolve([])),
    } as unknown as Datasource;

    const { container } = render(
      <QueryBuilder
        builderOptions={{
          mode: BuilderMode.List,
          table: 'foo',
          fields: [],
          filters: [],
          timeField: '',
        }}
        onBuilderOptionsChange={() => {}}
        datasource={mockDs}
        format={Format.AUTO}
        app={CoreApp.PanelEditor}
      />
    );
    expect(container.firstChild).not.toBeNull();
  });
});
