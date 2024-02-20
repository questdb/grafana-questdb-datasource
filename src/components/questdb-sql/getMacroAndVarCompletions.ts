import {getTemplateSrv} from "@grafana/runtime";
//import {IRange, languages} from "monaco-editor";
import {Range, CompletionKind} from "./utils";

const macros = [
    {
        label: '$__fromTime',
        documentation: 'Will be replaced by the starting time of the range of the panel cast to timestamp. Example: cast(1706263425598000 as timestamp)',
    },
    {
        label: '$__toTime',
        documentation: 'Will be replaced by the ending time of the range of the panel cast to timestamp. Example: cast(1706263425598000 as timestamp)',
    },
    {
        label: '$__timeFilter(timestampColumn)',
        documentation: 'Will be replaced by a conditional that filters the data (using the provided column) based on the time range of the panel. ' +
            'Example: timestampColumn >= cast(1706263425598000 as timestamp) AND timestampColumn <= cast(1706285057560000 as timestamp)',
    },
    {
        label: '$__sampleByInterval',
        documentation: 'Will be replaced by the interval, followed by unit: d, h, s or T (millisecond). Example: 1d, 5h, 20s, 1T',
    },
];

export function getMacroAndVarCompletion(range: Range) {
    const templateSrv = getTemplateSrv();
    if (!templateSrv) {
        return [];
    }

    let variables = templateSrv.getVariables().map((variable) => {
        const label = `\${${variable.name}}`;
        const val = templateSrv.replace(label);
        return {
            label,
            detail: `(Template Variable) ${val}`,
            kind: CompletionKind.Variable,
            insertText: `{${variable.name}}`,
            range,
        };
    });
    for (const macro of  macros) {
        variables.push(
            {
                ...macro,
                detail: '(Macro) ' + macro.label,
                kind: CompletionKind.Variable,
                insertText: macro.label,
                range
            }
        );
    }

    return variables;
}
