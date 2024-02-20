import React, { useState, useEffect } from 'react';
import { MultiSelect } from '@grafana/ui';
import { SelectableValue } from '@grafana/data';
import {SampleByFillMode} from './../../types';
import { selectors } from './../../selectors';
import { EditorField } from '@grafana/experimental';
import {GroupBase, OptionsOrGroups} from "react-select";

interface FillEditorProps {
    fills: string[];
    onFillsChange: (fills: string[]) => void;
}

const fillModes: SelectableValue[] = [];

// work around MultiSelect limitations by multiplicating values with different suffixes that are removed during sql text generation
fillModes.push({ value: SampleByFillMode.None /*+ suffix*/ , label: 'NONE' });
fillModes.push({ value: SampleByFillMode.Null /*+ suffix*/ , label: 'NULL' });
fillModes.push({ value: SampleByFillMode.Prev /*+ suffix*/ , label: 'PREV' });
fillModes.push({ value: SampleByFillMode.Linear /*+ suffix*/ , label: 'LINEAR' });

function getCustomFields(fields: string[]) {
    const customFields: Array<SelectableValue<string>> = [];
    fields.forEach((f, i) => {
        // add _index prefix to all values to allow user to use the same constant more than once value
        if (!f.match(/.*_[0-9]+$/)) {
            let value = f + '_' + i;
            fields[i] = value;
            customFields.push({label: f, value: value});
        } else {
            let label = f.replace(/_[0-9]+$/, '');
            customFields.push({label: label, value: f});
        }
    })
    return customFields;
}

export const SampleByFillEditor = (props: FillEditorProps) => {
    const [ custom, setCustom] = useState<Array<SelectableValue<string>>>([]);
    const [ isOpen, setIsOpen] = useState(false);
    const [ fills, setFills] = useState<string[]>(props.fills || []);
    const { label, tooltip } = selectors.components.QueryEditor.QueryBuilder.FILL;

    useEffect(() => {
        setFills(props.fills);
        const customFields = getCustomFields(props.fills);
        setCustom(customFields);
    }, [props.fills]);

    const onFieldsChange = (fields: string[]) => {
        const customFields = getCustomFields(fields);
        setCustom(customFields);
        setFills(fields);
    };

    const onUpdateField = () => {
        props.onFillsChange(fills);
    };

    const onChange = (e: Array<SelectableValue<string>>): void => {
        setIsOpen(false);
        onFieldsChange(e.map((v) => v.value!));
    };

    const isValidNewOption = (inputValue: string, value: SelectableValue<string> | null, options: OptionsOrGroups<SelectableValue<string>, GroupBase<SelectableValue<string>>>) => {
        return inputValue.trim().length > 0;
    }

    return (
        <EditorField tooltip={tooltip} label={label} data-testid={'query-builder-fields-multi-select-container'}>
            <MultiSelect<string>
                options={[...fillModes, ...custom]}
                value={fills && fills.length > 0 ? fills : []}
                isOpen={isOpen}
                onOpenMenu={() => setIsOpen(true)}
                onCloseMenu={() => setIsOpen(false)}
                onChange={onChange}
                onBlur={onUpdateField}
                allowCustomValue={true}
                isValidNewOption={isValidNewOption}
                width={50}
                isClearable={true}
                hideSelectedOptions={true}
            />
        </EditorField>
    );
};

