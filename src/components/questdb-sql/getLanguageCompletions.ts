import { operators } from "./operators"
import { dataTypes, functions, keywords } from "@questdb/sql-grammar"
import {CompletionKind, Range} from "./utils";

export const getLanguageCompletions = (range: Range) => [
  ...functions.map((qdbFunction) => {
    return {
      label: qdbFunction,
      kind: CompletionKind.Function,
      insertText: qdbFunction,
      range,
    }
  }),
  ...dataTypes.map((item) => {
    return {
      label: item,
      kind: CompletionKind.Keyword,
      insertText: item,
      range,
    }
  }),
  ...keywords.map((item) => {
    const keyword = item.toUpperCase()
    return {
      label: keyword,
      kind: CompletionKind.Keyword,
      insertText: keyword,
      range,
    }
  }),
  ...operators.map((item) => {
    const operator = item.toUpperCase()
    return {
      label: operator,
      kind: CompletionKind.Operator,
      insertText: operator.toUpperCase(),
      range,
    }
  }),
]
