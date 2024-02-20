import React from 'react';
import { SelectableValue } from '@grafana/data';
import {Input, Select} from '@grafana/ui';
import { selectors } from './../../selectors';
import {FullField, modeRequiresValue, SampleByAlignToMode} from 'types';
import {EditorField, EditorFieldGroup} from '@grafana/experimental';

interface SampleByAlignmentProps {
  fieldsList: FullField[];
  timeField: string | null;
  sampleByAlignToMode: SampleByAlignToMode;
  sampleByAlignToValue: string | null;
  onSampleByAlignToModeChange: (mode: string) => void;
  onSampleByAlignToValueChange: (value: string) => void;
}

const alignToModes: SelectableValue[] = [
  { value: SampleByAlignToMode.FirstObservation , label: 'FIRST OBSERVATION' },
  { value: SampleByAlignToMode.Calendar , label: 'CALENDAR' },
  { value: SampleByAlignToMode.CalendarOffset , label: 'CALENDAR OFFSET' },
  { value: SampleByAlignToMode.CalendarTimeZone , label: 'CALENDAR TIME ZONE' },
];

export const SampleByAlignEditor = (props: SampleByAlignmentProps) => {
  return (
      <EditorFieldGroup>
        <EditorField tooltip={selectors.components.QueryEditor.QueryBuilder.ALIGN_TO.tooltip} label={selectors.components.QueryEditor.QueryBuilder.ALIGN_TO.label}>
          <Select
              options={alignToModes}
              width={25}
              onChange={(e) => props.onSampleByAlignToModeChange(e.value)}
              value={props.sampleByAlignToMode}
          />
        </EditorField>
        <EditorField tooltip={selectors.components.QueryEditor.QueryBuilder.CALENDAR_OFF_TZ.tooltip} label={selectors.components.QueryEditor.QueryBuilder.CALENDAR_OFF_TZ.label}>
          <Input width={25} type="text"
                 disabled={!modeRequiresValue(props.sampleByAlignToMode)}
                 defaultValue={props.sampleByAlignToValue || ''}
                 onBlur={(e) => props.onSampleByAlignToValueChange(e.currentTarget.value)} />
        </EditorField>
      </EditorFieldGroup>
  );
};
