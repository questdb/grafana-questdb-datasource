import React from 'react';
import {render, waitFor} from '@testing-library/react';
import {TableSelect} from './TableSelect';
import {Datasource} from '../../data/QuestDbDatasource';
import {BuilderMode} from "../../types";

describe('TableSelect', () => {
  it('renders correctly', async () => {
    const setState = jest.fn();
    const mockDs = {} as Datasource;
    mockDs.fetchTables = jest.fn((tsOnly?: boolean) => Promise.resolve([]));
    const useStateMock: any = (initState: any) => [initState, setState];
    jest.spyOn(React, 'useState').mockImplementation(useStateMock);
    const result = await waitFor(() => render(<TableSelect table="" onTableChange={() => {}} datasource={mockDs} mode={BuilderMode.Trend} />));
    expect(result.container.firstChild).not.toBeNull();
  });
});
