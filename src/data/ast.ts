import { parseFirst, Statement, SelectFromStatement, astMapper, toSql, ExprRef } from 'questdb-sql-ast-parser';

export function sqlToStatement(sql: string): Statement {
  const replaceFuncs: Array<{
    startIndex: number;
    name: string;
    replacementName: string;
  }> = [];
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
    ast = parseFirst(sql);
  } catch (err) {
    //console.debug(`Failed to parse SQL statement into an AST: ${err}`);
    return {} as Statement;
  }

  const mapper = astMapper((map) => ({
    constant: (c) => {
      if (c.type === 'sampleByUnit') {
        const rf = replaceFuncs.find((x) => c.value.startsWith(x.replacementName));
        if (rf) {
          return {...c, value: c.value.replace(rf.replacementName, rf.name)};
        }
      }
      return map.super().constant(c);
    },
    tableRef: (t) => {
      const rfs = replaceFuncs.find((x) => x.replacementName === t.schema);
      if (rfs) {
        return { ...t, schema: t.schema?.replace(rfs.replacementName, rfs.name) };
      }
      const rft = replaceFuncs.find((x) => x.replacementName === t.name);
      if (rft) {
        return { ...t, name: t.name.replace(rft.replacementName, rft.name) };
      }
      return map.super().tableRef(t);
    },
    ref: (r) => {
      const rf = replaceFuncs.find((x) => r.name.startsWith(x.replacementName));
      if (rf) {
        const d = r.name.replace(rf.replacementName, rf.name);
        return { ...r, name: d };
      }
      return map.super().ref(r);
    },
    call: (c) => {
      const rf = replaceFuncs.find((x) => c.function.name.startsWith(x.replacementName));
      if (rf) {
        return { ...c, function: { ...c.function, name: c.function.name.replace(rf.replacementName, rf.name) } };
      }
      return map.super().call(c);
    },
  }));
  return mapper.statement(ast)!;
}

export function getTable(sql: string): string {
  const stm = sqlToStatement(sql);
  if (stm.type !== 'select' || !stm.from?.length || stm.from?.length <= 0) {
    return '';
  }
  switch (stm.from![0].type) {
    case 'table': {
      const table = stm.from![0];
      return `${table.name.name}`;
    }
    case 'statement': {
      const table = stm.from![0];
      return getTable(toSql.statement(table.statement));
    }
  }
  return '';
}

export function getFields(sql: string): string[] {
  const stm = sqlToStatement(sql) as SelectFromStatement;
  if (stm.type !== 'select' || !stm.columns?.length || stm.columns?.length <= 0) {
    return [];
  }

  return stm.columns.map((x) => {
    const exprName = (x.expr as ExprRef).name;
    if (!exprName){
      return '';
    }
    if (x.alias !== undefined) {
      return `${exprName} as ${x.alias?.name}`;
    } else {
      return `${exprName}`;
    }
  });
}
