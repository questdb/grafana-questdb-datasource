import {CompletionKind, Range, uniq} from "./utils"
import { CompletionItemPriority, InformationSchemaColumn } from "./types"
//import { IRange, languages } from "monaco-editor"

export const getColumnCompletions = ({
  columns,
  range,
  withTableName,
  priority,
}: {
  columns: InformationSchemaColumn[]
  range: Range
  withTableName: boolean
  priority: CompletionItemPriority
}) => {
  // For JOIN ON ... completions, return `table.column` text
  if (withTableName) {
    return columns.map((item) => ({
      label: {
        label: `${item.tableName}.${item.columnName}`,
        detail: "",
        description: item.dataType,
      },
      kind: CompletionKind.Enum,
      insertText: `${item.tableName}.${item.columnName}`,
      sortText: priority,
      range,
    }))
    // For everything else, return a list of unique column names.
  } else {
    return uniq(columns.map((item) => item.columnName)).map((columnName) => {
      const tableNames = columns
        .filter((item) => item.columnName === columnName)
        .map((item) => item.tableName)
      return {
        label: {
          label: columnName,
          detail: ` (${tableNames.sort().join(", ")})`,
          // If the column is present in multiple tables, show their list here, otherwise return the column type.
          description:
            tableNames.length > 1
              ? ""
              : columns.find((item) => item.columnName === columnName)
                  ?.dataType,
        },
        kind: CompletionKind.Enum,
        insertText: columnName,
        sortText: priority,
        range,
      }
    })
  }
}
