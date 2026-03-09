import React from 'react';
import { render } from '@testing-library/react';
import { TableSelect } from './TableSelect';
import { Datasource } from '../../data/QuestDbDatasource';
import { BuilderMode } from '../../types';

describe('TableSelect', () => {
  it('renders correctly', async () => {
    const mockDs = {
      fetchTables: jest.fn(() => Promise.resolve([])),
    } as unknown as Datasource;

    const { container } = render(
      <TableSelect table="" onTableChange={() => {}} datasource={mockDs} mode={BuilderMode.Trend} />
    );
    expect(container.firstChild).not.toBeNull();
  });
});
