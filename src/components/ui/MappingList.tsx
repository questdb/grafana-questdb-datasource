import React from 'react';
import { Button, IconButton, Input } from '@grafana/ui';

export interface MappingColumn<T> {
  /** Key on the row object this column edits. */
  field: keyof T & string;
  placeholder: string;
  /** Accessible-name prefix; the 1-based row number is appended (e.g. "Service account 1"). */
  ariaLabel: string;
}

export interface MappingListProps<T> {
  items: T[];
  columns: Array<MappingColumn<T>>;
  /** Factory for a blank row, used by the add button. */
  newRow: () => T;
  onChange: (next: T[]) => void;
  addLabel: string;
  /** Accessible-name prefix for each row's remove button; the 1-based row number is appended. */
  removeLabel: string;
}

/**
 * MappingList renders an editable list of fixed-shape, all-string rows (e.g. user → service
 * account or group → service account mappings) with add / inline-edit / remove. Rows carry
 * stable client-side keys so editing or removing a middle row reconciles inputs by identity
 * rather than by position — otherwise React would move focus/caret to the wrong row.
 */
export function MappingList<T>({ items, columns, newRow, onChange, addLabel, removeLabel }: MappingListProps<T>) {
  // Stable, client-only ids (never persisted): one per row, assigned lazily as rows appear.
  // remove() drops the id at the removed index so survivors keep their identity; the
  // render-time grow/shrink covers initial load and any external replacement of items.
  const idsRef = React.useRef<number[]>([]);
  const nextIdRef = React.useRef(0);
  while (idsRef.current.length < items.length) {
    idsRef.current.push(nextIdRef.current++);
  }
  if (idsRef.current.length > items.length) {
    idsRef.current = idsRef.current.slice(0, items.length);
  }

  const update = (index: number, field: keyof T & string, value: string) =>
    onChange(items.map((row, i) => (i === index ? ({ ...row, [field]: value } as T) : row)));
  const remove = (index: number) => {
    idsRef.current = idsRef.current.filter((_, i) => i !== index);
    onChange(items.filter((_, i) => i !== index));
  };
  const add = () => onChange([...items, newRow()]);

  return (
    <div>
      {items.map((row, i) => (
        <div key={idsRef.current[i]} style={{ display: 'flex', gap: '8px', marginBottom: '8px', alignItems: 'center' }}>
          {columns.map((col) => (
            <Input
              key={col.field}
              width={30}
              value={String(row[col.field] ?? '')}
              placeholder={col.placeholder}
              aria-label={`${col.ariaLabel} ${i + 1}`}
              onChange={(e) => update(i, col.field, e.currentTarget.value)}
            />
          ))}
          <IconButton
            name="trash-alt"
            aria-label={`${removeLabel} ${i + 1}`}
            tooltip={`${removeLabel} ${i + 1}`}
            onClick={() => remove(i)}
          />
        </div>
      ))}
      <Button variant="secondary" icon="plus" onClick={add}>
        {addLabel}
      </Button>
    </div>
  );
}
