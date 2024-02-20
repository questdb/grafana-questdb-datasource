import { getFields, sqlToStatement } from './ast';
import { toSql } from 'questdb-sql-ast-parser'

describe('ast', () => {
  describe('getFields', () => {
    it('return 1 expression if statement does not have an alias', () => {
      const stm = getFields(`select foo from bar`);
      expect(stm.length).toBe(1);
      expect(stm[0]).toEqual('foo');
    });

    it('return all columns in select list', () => {
      const stm = getFields(`select foo1, foo2, foo3 from bar`);
      expect(stm.length).toBe(3);
      expect(stm).toEqual(['foo1', 'foo2', 'foo3']);
    });

    it('return all columns ignoring expressions', () => {
      const stm = getFields(`select foo1, 1+2, abs(f(x)) from bar`);
      expect(stm.length).toBe(3);
      expect(stm).toEqual(['foo1', '', '']);
    });
  });
  describe('sqlToStatement', () => {
    it('macro parses correctly', () => {
      const sql = 'SELECT count(*) FROM foo where $__timeFilter(tstmp)';
      const stm = sqlToStatement(sql);
      // this is formatted like this to match how pgsql generates its sql
      expect(toSql.statement(stm)).toEqual('SELECT (count (*) )  FROM foo   WHERE ("$__timefilter" (tstmp) )');
    });

    it('sampleByInterval macro parses correctly', () => {
      const sql = 'SELECT count(*) FROM foo sample by $__sampleByInterval';
      const stm = sqlToStatement(sql);
      // this is formatted like this to match how pgsql generates its sql
      expect(toSql.statement(stm)).toEqual('SELECT (count (*) )  FROM foo   SAMPLE BY $__sampleByInterval');
    });

    it('variable parses correctly', () => {
      const sql = 'SELECT count(*) FROM foo where str in $query0';
      const stm = sqlToStatement(sql);
      // this is formatted like this to match how pgsql generates its sql
      expect(toSql.statement(stm)).toEqual('SELECT (count (*) )  FROM foo   WHERE (str IN \"$query0\")');
    });

  });
});
