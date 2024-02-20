import React, { useState, useEffect } from 'react';
import { MultiSelect } from '@grafana/ui';
import { SelectableValue } from '@grafana/data';
import { FullField } from './../../types';
import { selectors } from './../../selectors';
import {EditorField, EditorRow} from '@grafana/experimental';

interface PartitionByEditorProps {
    fieldsList: FullField[];
    fields: string[];
    onFieldsChange: (fields: string[]) => void;
    timeField: string;
    isDisabled: boolean
}
export const PartitionByEditor = (props: PartitionByEditorProps) => {
    //const timestamp = props.timeField;
    const columns = (props.fieldsList || []).map((f) => ({ label: f.label, value: f.name }));
    const [isOpen, setIsOpen] = useState(false);
    const [fields, setFields] = useState<string[]>(props.fields || []);
    const { label, tooltip } = selectors.components.QueryEditor.QueryBuilder.PARTITION_BY;

    useEffect(() => {
        if (props.fieldsList.length === 0) {
            return;
        }
        setFields(props.fields);
    }, [props.fieldsList, props.fields]);

    const onFieldsChange = (fields: string[]) => {
        setFields(fields);
    };

    const onUpdateField = () => {
        props.onFieldsChange(fields);
    };

    const onChange = (e: Array<SelectableValue<string>>): void => {
        setIsOpen(false);
        onFieldsChange(e.map((v) => v.value!));
    };
    return (
        <EditorRow>
            <EditorField tooltip={tooltip} label={label} data-testid={'query-builder-fields-multi-select-container'}>
                <MultiSelect<string>
                    placeholder={props.isDisabled ? 'Table is missing designated timestamp' : 'Choose'}
                    options={[...columns]}
                    value={fields && fields.length > 0 ? fields : []}
                    isOpen={isOpen}
                    onOpenMenu={() => setIsOpen(true)}
                    onCloseMenu={() => setIsOpen(false)}
                    onChange={onChange}
                    onBlur={onUpdateField}
                    allowCustomValue={false}
                    width={50}
                    disabled={props.isDisabled}
                />
            </EditorField>
        </EditorRow>
    );
};
