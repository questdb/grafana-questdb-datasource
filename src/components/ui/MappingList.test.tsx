import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MappingColumn, MappingList } from './MappingList';

interface Row {
  user: string;
  sa: string;
}

const columns: Array<MappingColumn<Row>> = [
  { field: 'user', placeholder: 'user', ariaLabel: 'User' },
  { field: 'sa', placeholder: 'sa', ariaLabel: 'Service account' },
];

// A controlled wrapper that actually applies onChange back into state, so MappingList
// re-renders with the mutated rows. This is what exercises the stable-key reconciliation:
// the ConfigEditor tests stub onChange with a bare jest.fn(), so the list never re-renders
// and a broken key scheme (e.g. key={i}) would pass them unnoticed. The discriminating
// signal is focus / DOM-node identity — the controlled `value` is always correct regardless
// of key, but only stable keys keep the caret on the same physical input across a removal.
function Harness({ initial }: { initial: Row[] }) {
  const [items, setItems] = React.useState<Row[]>(initial);
  return (
    <MappingList<Row>
      items={items}
      columns={columns}
      newRow={() => ({ user: '', sa: '' })}
      onChange={setItems}
      addLabel="Add mapping"
      removeLabel="Remove"
    />
  );
}

const threeRows: Row[] = [
  { user: 'a', sa: 'sa_a' },
  { user: 'b', sa: 'sa_b' },
  { user: 'c', sa: 'sa_c' },
];

describe('MappingList stable-key reconciliation', () => {
  it('keeps focus and node identity on a later row when an earlier row is removed', () => {
    render(<Harness initial={threeRows} />);

    // Track the last row's input by reference, then put the caret in it.
    const cInput = screen.getByDisplayValue('c');
    cInput.focus();
    expect(cInput).toHaveFocus();

    // Remove the FIRST row — the maximal index shift. With index-based keys React would
    // reconcile by position, unmount the focused node, and move 'c' onto a different element.
    fireEvent.click(screen.getByRole('button', { name: 'Remove 1' }));

    expect(screen.queryByDisplayValue('a')).not.toBeInTheDocument();
    // Same physical node, still focused — proves reconciliation kept identity by key, not index.
    expect(screen.getByDisplayValue('c')).toBe(cInput);
    expect(cInput).toHaveFocus();
  });

  it('keeps focus and node identity on the last row when a middle row is removed', () => {
    render(<Harness initial={threeRows} />);

    const cInput = screen.getByDisplayValue('c');
    cInput.focus();

    fireEvent.click(screen.getByRole('button', { name: 'Remove 2' }));

    expect(screen.queryByDisplayValue('b')).not.toBeInTheDocument();
    expect(screen.getByDisplayValue('c')).toBe(cInput);
    expect(cInput).toHaveFocus();
  });
});
