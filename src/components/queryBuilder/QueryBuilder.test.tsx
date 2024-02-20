import React from 'react';
import { render, waitFor } from '@testing-library/react';
import { QueryBuilder } from './QueryBuilder';
import { Datasource } from '../../data/QuestDbDatasource';
import { BuilderMode, Format } from 'types';
import { CoreApp } from '@grafana/data';

describe('QueryBuilder', () => {
  it('renders correctly', async () => {
    const setState = jest.fn();
    const mockDs = { settings: { jsonData: {} } } as Datasource;
    mockDs.fetchTables = jest.fn((timeSeriesOnly?: boolean) => Promise.resolve([]));
    mockDs.fetchFields = jest.fn(() => {
      setState();
      return Promise.resolve([]);
    });
    const useStateMock: any = (initState: any) => [initState, setState];
    jest.spyOn(React, 'useState').mockImplementation(useStateMock);
    const result = await waitFor(() =>
      render(
        <QueryBuilder
          builderOptions={{
            mode: BuilderMode.List,
            table: 'foo',
            fields: [],
            filters: [],
            timeField: ''
          }}
          onBuilderOptionsChange={() => {}}
          datasource={mockDs}
          format={Format.AUTO}
          app={CoreApp.PanelEditor}
        />
      )
    );
    expect(result.container.firstChild).not.toBeNull();
  });
});
