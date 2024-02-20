import React, { useState } from 'react';
import { Input } from '@grafana/ui';
import { selectors } from './../../selectors';
import { EditorField } from '@grafana/experimental';

interface LimitEditorProps {
  limit: string;
  onLimitChange: (limit: string) => void;
}
export const LimitEditor = (props: LimitEditorProps) => {
  const [limit, setLimit] = useState(props.limit || '100');
  const { label, tooltip } = selectors.components.QueryEditor.QueryBuilder.LIMIT;

  return (
    <EditorField tooltip={tooltip} label={label}>
      <Input
        width={10}
        value={limit}
        onChange={(e) => setLimit(e.currentTarget.value.replace(/[^0-9 ,-]/, ''))}
        onBlur={() => props.onLimitChange(limit)}
      />
    </EditorField>
  );
};
