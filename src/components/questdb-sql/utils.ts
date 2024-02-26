export const uniq = <T = unknown>(list: T[]) => Array.from(new Set(list))

// monaco.languages.CompletionItemKind
export enum CompletionKind {
    Function = 1, // monaco.languages.CompletionItemKind.Function
    Field = 3, // monaco.languages.CompletionItemKind.Field,
    Variable = 4, // monaco.languages.CompletionItemKind.Variable,
    Class = 5, // monaco.languages.CompletionItemKind.Class,
    Module = 8, // monaco.languages.CompletionItemKind.Module,
    Operator = 11, // monaco.languages.CompletionItemKind.Operator,
    Enum = 15, // monaco.languages.CompletionItemKind.Enum,
    Keyword = 17, // monaco.languages.CompletionItemKind.Keyword,
}

export interface Range {
    startLineNumber: number;
    endLineNumber: number;
    startColumn: number;
    endColumn: number;
}

export type Table = {
    tableName: string
    partitionBy: string
    designatedTimestamp: string
    walEnabled: boolean
    dedup: boolean
}

export const QuestDBLanguageName = "questdb-sql"

export type Request = Readonly<{
    query: string
    row: number
    column: number
}>

export const getQueryFromCursor = (
    editor: any,
): Request | undefined => {
    const text = editor
        .getValue({ preserveBOM: false, lineEnding: "\n" })
        .replace(/"[^"]*"|'[^']*'|`[^`]*`|(--\s?.*$)/gm, (match: string, group: string) => {
            return group ? "" : match
        })
    const position = editor.getPosition()

    let row = 0

    let column = 0

    const sqlTextStack = []
    let startRow = 0
    let startCol = 0
    let startPos = -1
    let sql = null
    let inQuote = false

    if (!position) {
        return
    }

    for (let i = 0; i < text.length; i++) {
        if (sql !== null) {
            break
        }

        const char = text[i]

        switch (char) {
            case ";": {
                if (inQuote) {
                    column++
                    continue
                }

                if (
                    row < position.lineNumber - 1 ||
                    (row === position.lineNumber - 1 && column < position.column - 1)
                ) {
                    sqlTextStack.push({
                        row: startRow,
                        col: startCol,
                        position: startPos,
                        limit: i,
                    })
                    startRow = row
                    startCol = column
                    startPos = i + 1
                    column++
                } else {
                    // empty queries, aka ;; , make sql.length === 0
                    sql = text.substring(startPos === -1 ? 0 : startPos, i)
                }
                break
            }

            case " ": {
                // ignore leading space
                if (startPos === i) {
                    startRow = row
                    startCol = column
                    startPos = i + 1
                }

                column++
                break
            }

            case "\n": {
                row++
                column = 0

                if (startPos === i) {
                    startRow = row
                    startCol = column
                    startPos = i + 1
                    column++
                }
                break
            }

            case "'": {
                inQuote = !inQuote
                column++
                break
            }

            default: {
                column++
                break
            }
        }
    }

    if (sql === null) {
        sql = startPos === -1 ? text : text.substring(startPos)
    }

    if (sql.length === 0) {
        const prev = sqlTextStack.pop()

        if (prev) {
            return {
                column: prev.col,
                query: text.substring(prev.position, prev.limit),
                row: prev.row,
            }
        }

        return
    }

    return {
        column: startCol,
        query: sql,
        row: startRow,
    }
}

export interface FindMatch {
    range: Range;
    matches: string[] | null;
}

export const findMatches = (model: any, needle: string): FindMatch[] =>
    model.findMatches(
    needle /* searchString */,
    true /* searchOnlyEditableRange */,
    false /* isRegex */,
    true /* matchCase */,
    null /* wordSeparators */,
    true /* captureMatches */,
) ?? null
