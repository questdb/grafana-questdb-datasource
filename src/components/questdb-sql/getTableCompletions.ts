import {Range, CompletionKind, Table} from "./utils"
import { CompletionItemPriority } from "./types"
//import { /*IRange,*/ languages } from "monaco-editor"

export const getTableCompletions = ({
  tables,
  range,
  priority,
  openQuote,
  nextCharQuote,
}: {
  tables: Table[]
  range: Range
  priority: CompletionItemPriority
  openQuote: boolean
  nextCharQuote: boolean
}) => {
  return tables.map((item) => {
    return {
      label: item.tableName,
      kind: CompletionKind.Class,
      insertText: openQuote
        ? item.tableName + (nextCharQuote ? "" : '"')
        : /^[a-z0-9_]+$/i.test(item.tableName)
        ? item.tableName
        : `"${item.tableName}"`,
      sortText: priority,
      range,
    }
  })
}
