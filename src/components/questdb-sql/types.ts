export type InformationSchemaColumn = {
  tableName: string
  ordinalPosition: number
  columnName: string
  dataType: string
}

export enum CompletionItemPriority {
  High = "1",
  MediumHigh = "2",
  Medium = "3",
  MediumLow = "4",
  Low = "5",
}
