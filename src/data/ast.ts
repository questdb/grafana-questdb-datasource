import { parseOne, toSql, type Statement, type SelectStatement, type ColumnRef, type QualifiedName, type ExpressionSelectItem } from '@questdb/sql-parser';

export type ReplaceFuncItem = {
  startIndex: number;
  name: string;
  replacementName: string;
};

export function deepReplaceStrings(obj: any, replaceFuncs: ReplaceFuncItem[]): any {
  if (typeof obj === 'string') {
    for (const rf of replaceFuncs) {
      if (obj.includes(rf.replacementName)) {
        return obj.replace(rf.replacementName, rf.name);
      }
    }
    return obj;
  }
  if (Array.isArray(obj)) {
    return obj.map((item) => deepReplaceStrings(item, replaceFuncs));
  }
  if (obj && typeof obj === 'object') {
    const result: any = {};
    for (const key of Object.keys(obj)) {
      result[key] = deepReplaceStrings(obj[key], replaceFuncs);
    }
    return result;
  }
  return obj;
}

export function sqlToStatement(sql: string): Statement {
  const replaceFuncs: ReplaceFuncItem[] = [];
  // questdb parser accepts only number followed by letter as sample by argument, so we've to use specific id
  const re = /(\$__sampleByInterval|\$__|\$)/gi;
  let regExpArray: RegExpExecArray | null;
  while ((regExpArray = re.exec(sql)) !== null) {
    replaceFuncs.push({ startIndex: regExpArray.index, name: regExpArray[0], replacementName: '' });
  }

  const interval = "$__sampleByInterval";
  //need to process in reverse so starting positions aren't effected by replacing other things
  for (let i = replaceFuncs.length - 1; i >= 0; i--) {
    const si = replaceFuncs[i].startIndex;

    if (replaceFuncs[i].name !== interval){
      const replacementName = 'f' + (Math.random() + 1).toString(36).substring(7);
      replaceFuncs[i].replacementName = replacementName;
      sql = sql.substring(0, si) + replacementName + sql.substring(si + replaceFuncs[i].name.length);
    } else {
      const replacementName =  (Math.random() + 1).toString(10).substring(7) + 'd';
      replaceFuncs[i].replacementName = replacementName;
      sql = sql.substring(0, si) + replacementName + sql.substring(si + replaceFuncs[i].name.length);
    }
  }

  let ast: Statement;
  try {
    ast = parseOne(sql);
  } catch (err) {
    //console.debug(`Failed to parse SQL statement into an AST: ${err}`);
    return {} as Statement;
  }

  return deepReplaceStrings(ast, replaceFuncs);
}

export function getTable(sql: string): string {
  const stm = sqlToStatement(sql) as SelectStatement;
  if (stm.type !== 'select' || !stm.from?.length || stm.from?.length <= 0) {
    return '';
  }
  const tableRef = stm.from![0];
  switch (tableRef.table.type) {
    case 'qualifiedName': {
      const parts = (tableRef.table as QualifiedName).parts;
      return parts[parts.length - 1];
    }
    case 'select': {
      return getTable(toSql(tableRef.table as SelectStatement));
    }
  }
  return '';
}

export function getFields(sql: string): string[] {
  const stm = sqlToStatement(sql) as SelectStatement;
  if (stm.type !== 'select' || !stm.columns?.length || stm.columns?.length <= 0) {
    return [];
  }

  return stm.columns.map((x) => {
    if (x.type === 'star') {
      return '*';
    }
    if (x.type !== 'selectItem') {
      return '';
    }
    const item = x as ExpressionSelectItem;
    if (item.expression.type !== 'column') {
      return '';
    }
    const colRef = item.expression as ColumnRef;
    const colName = colRef.name.parts[colRef.name.parts.length - 1];
    if (!colName) {
      return '';
    }
    if (item.alias !== undefined) {
      return `${colName} as ${item.alias}`;
    } else {
      return `${colName}`;
    }
  });
}
