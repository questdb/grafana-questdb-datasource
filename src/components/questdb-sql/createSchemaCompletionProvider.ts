/*import * as monaco from "monaco-editor"
import {editor, Position, Range} from "monaco-editor"
import IStandaloneCodeEditor = editor.IStandaloneCodeEditor*/
//import {CompletionList, ProviderResult} from "monaco-editor";
//import {CompletionItem} from "monaco-editor";

import { CompletionItemPriority, InformationSchemaColumn } from "./types"
import { findMatches , getQueryFromCursor, Table, uniq } from "./utils"
import { getTableCompletions } from "./getTableCompletions"
import { getColumnCompletions } from "./getColumnCompletions"
import { getLanguageCompletions } from "./getLanguageCompletions"
import {getMacroAndVarCompletion} from "./getMacroAndVarCompletions";

const trimQuotesFromTableName = (tableName: string) => {
  return tableName.replace(/(^")|("$)/g, "")
}

export interface Position {
  lineNumber: number;
  column: number;
}

export type Thenable<T> = PromiseLike<T>;
export type ProviderResult<T> = T | undefined | null | Thenable<T | undefined | null>;

export interface CompletionList {
  suggestions: any[];
  incomplete?: boolean;
}

export const createSchemaCompletionProvider = (
  editor: any,
  tables: Table[] = [],
  informationSchemaColumns: InformationSchemaColumn[] = [],
) => {

  const completionProvider: any = {
    triggerCharacters:
      'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz\n ."'.split(""),
    provideCompletionItems(model: any, position: Position): ProviderResult<CompletionList> {
      const word = model.getWordUntilPosition(position)
      const queryAtCursor = getQueryFromCursor(editor)

      let tableContext: string[] = []

      if (queryAtCursor) {
        const matches = findMatches(model, queryAtCursor.query)
        if (matches.length > 0) {
          const cursorMatch = matches.find(
            (m) => m.range.startLineNumber === queryAtCursor.row + 1,
          )

          const fromMatch = queryAtCursor.query.match(/(?<=FROM\s)([^ )]+)/gim)
          const joinMatch = queryAtCursor.query.match(/(JOIN)\s+([^ ]+)/i)
          const alterTableMatch = queryAtCursor.query.match(
            /(ALTER TABLE)\s+([^ ]+)/i,
          )
          if (fromMatch) {
            tableContext = uniq(fromMatch)
          } else if (alterTableMatch && alterTableMatch[2]) {
            tableContext.push(alterTableMatch[2])
          }
          if (joinMatch && joinMatch[2]) {
            tableContext.push(joinMatch[2])
          }

          tableContext = tableContext.map(trimQuotesFromTableName)

          const textUntilPosition = model.getValueInRange({
            startLineNumber: cursorMatch?.range.startLineNumber ?? 1,
            startColumn: cursorMatch?.range.startColumn ?? 1,
            endLineNumber: position.lineNumber,
            endColumn: word.startColumn,
          })

          const range = {
            startLineNumber: position.lineNumber,
            endLineNumber: position.lineNumber,
            startColumn: word.startColumn,
            endColumn: word.endColumn,
          }

          const nextChar = model.getValueInRange({
            startLineNumber: position.lineNumber,
            startColumn: word.endColumn,
            endLineNumber: position.lineNumber,
            endColumn: word.endColumn + 1,
          })

          const openQuote = textUntilPosition.substr(-1) === '"'
          const nextCharQuote = nextChar === '"'

          if (
            /(FROM|INTO|(ALTER|BACKUP|DROP|REINDEX|RENAME|TRUNCATE|VACUUM) TABLE|JOIN|UPDATE)\s$/gim.test(
              textUntilPosition,
            ) ||
            (/'$/gim.test(textUntilPosition) &&
              !textUntilPosition.endsWith("= '"))
          ) {
            return {
              suggestions: getTableCompletions({
                tables,
                range,
                priority: CompletionItemPriority.High,
                openQuote,
                nextCharQuote,
              }),
            }
          }

          // get text value in the current line
          const textInLine = model.getValueInRange({
            startLineNumber: position.lineNumber,
            startColumn: 1,
            endLineNumber: position.lineNumber,
            endColumn: position.column,
          })
          // check if `textInLine` contains whitespaces only
          const isWhitespaceOnly = /^\s*$/.test(textInLine)

          if (
            /(?:(SELECT|UPDATE).*?(?:(?:,(?:COLUMN )?)|(?:ALTER COLUMN ))?(?:WHERE )?(?: BY )?(?: ON )?(?: SET )?$|ALTER COLUMN )/gim.test(
              textUntilPosition,
            ) &&
            !isWhitespaceOnly
          ) {
            if (tableContext.length > 0) {
              const withTableName =
                textUntilPosition.match(/\sON\s/gim) !== null
              return {
                suggestions: [
                  ...getColumnCompletions({
                    columns: informationSchemaColumns.filter((item) =>
                      tableContext.includes(item.tableName),
                    ),
                    range,
                    withTableName,
                    priority: CompletionItemPriority.High,
                  }),
                  ...getLanguageCompletions(range),
                  ...getMacroAndVarCompletion(range)
                ],
              }
            } else {
              return {
                suggestions: [
                  ...getColumnCompletions({
                    columns: informationSchemaColumns,
                    range,
                    withTableName: false,
                    priority: CompletionItemPriority.High,
                  }),
                  ...getTableCompletions({
                    tables,
                    range,
                    priority: CompletionItemPriority.MediumHigh,
                    openQuote,
                    nextCharQuote,
                  }),
                  ...getLanguageCompletions(range),
                  ...getMacroAndVarCompletion(range)
                ],
              }
            }
          }

          if (word.word) {
            return {
              suggestions: [
                ...getTableCompletions({
                  tables,
                  range,
                  priority: CompletionItemPriority.High,
                  openQuote,
                  nextCharQuote,
                }),
                ...getLanguageCompletions(range),
                ...getMacroAndVarCompletion(range)
              ],
            }
          }
        }
      }

      return undefined;
    },
  }

  return completionProvider
}
