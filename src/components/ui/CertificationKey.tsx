import React, { ChangeEvent, MouseEvent, FC } from 'react';
import { Input, Button, TextArea, Field } from '@grafana/ui';

interface Props {
  label: string;
  tooltip?: string;
  hasCert: boolean;
  placeholder: string;
  onChange: (event: ChangeEvent<HTMLTextAreaElement>) => void;
  onClick: (event: MouseEvent<HTMLButtonElement>) => void;
}

export const CertificationKey: FC<Props> = ({ hasCert, label, tooltip, onChange, onClick, placeholder }) => {
  return (
    <Field label={label} description={tooltip}>
      {hasCert ? (
        <>
          <Input type="text" disabled value="configured" width={24} />
          <Button variant="secondary" onClick={onClick} style={{ marginLeft: 4 }}>
            Reset
          </Button>
        </>
      ) : (
        <TextArea rows={7} onChange={onChange} placeholder={placeholder} required />
      )}
    </Field>
  );
};
