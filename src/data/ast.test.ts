import { getFields, getTable, sqlToStatement, deepReplaceStrings, ReplaceFuncItem } from './ast';
import { toSql } from '@questdb/sql-parser';

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

    it('returns field with alias', () => {
      const stm = getFields(`SELECT foo as bar FROM t`);
      expect(stm).toEqual(['foo as bar']);
    });

    it('returns multiple fields with mixed aliases', () => {
      const stm = getFields(`SELECT a, b as c, d FROM t`);
      expect(stm).toEqual(['a', 'b as c', 'd']);
    });

    it('returns multiple fields with multiple aliases', () => {
      const stm = getFields(`SELECT x as x1, y as y1 FROM t`);
      expect(stm).toEqual(['x as x1', 'y as y1']);
    });

    it('returns empty array for empty SQL', () => {
      const stm = getFields('');
      expect(stm).toEqual([]);
    });

    it('returns empty array for unparseable SQL', () => {
      const stm = getFields('NOT VALID SQL AT ALL');
      expect(stm).toEqual([]);
    });

    it('returns empty string for function call in select', () => {
      const stm = getFields(`SELECT count(*) FROM t`);
      expect(stm).toEqual(['']);
    });

    it('returns empty string for numeric literal in select', () => {
      const stm = getFields(`SELECT 42 FROM t`);
      expect(stm).toEqual(['']);
    });

    it('returns empty string for string literal in select', () => {
      const stm = getFields(`SELECT 'hello' FROM t`);
      expect(stm).toEqual(['']);
    });

    it('handles mixed fields and expressions', () => {
      const stm = getFields(`SELECT a, count(*), b as c FROM t`);
      expect(stm).toEqual(['a', '', 'b as c']);
    });

    it('handles star select', () => {
      const stm = getFields(`SELECT * FROM t`);
      // parser represents * as ExprRef with name '*'
      expect(stm).toEqual(['*']);
    });
  });

  describe('getTable', () => {
    it('returns simple table name', () => {
      expect(getTable('SELECT * FROM foo')).toBe('foo');
    });

    it('returns quoted table name', () => {
      expect(getTable('SELECT * FROM "my_table"')).toBe('my_table');
    });

    it('returns table with dot in quoted name', () => {
      expect(getTable('SELECT * FROM "foo.bar"')).toBe('foo.bar');
    });

    it('returns inner table from subquery', () => {
      expect(getTable('SELECT * FROM (SELECT * FROM inner_t) alias')).toBe('inner_t');
    });

    it('returns deepest table from nested subquery', () => {
      expect(getTable('SELECT * FROM (SELECT * FROM (SELECT * FROM deep) a) b')).toBe('deep');
    });

    it('returns empty string for invalid SQL', () => {
      expect(getTable('NOT VALID SQL')).toBe('');
    });

    it('returns empty string for empty string', () => {
      expect(getTable('')).toBe('');
    });

    it('returns table name when query has $variable in WHERE', () => {
      expect(getTable('SELECT * FROM foo WHERE col = $var')).toBe('foo');
    });

    it('returns table name when query has $__timeFilter', () => {
      expect(getTable('SELECT * FROM foo WHERE $__timeFilter(ts)')).toBe('foo');
    });

    it('returns table name for simple select with fields', () => {
      expect(getTable('SELECT a, b FROM my_table')).toBe('my_table');
    });

    it('returns table name when query has WHERE clause', () => {
      expect(getTable('SELECT a FROM my_table WHERE x = 1')).toBe('my_table');
    });

    it('returns table name when query has ORDER BY', () => {
      expect(getTable('SELECT a FROM my_table ORDER BY a ASC')).toBe('my_table');
    });

    it('returns table name when query has LIMIT', () => {
      expect(getTable('SELECT a FROM my_table LIMIT 10')).toBe('my_table');
    });

    it('returns table name when query has SAMPLE BY', () => {
      expect(getTable('SELECT count(*) FROM my_table SAMPLE BY $__sampleByInterval')).toBe('my_table');
    });

    it('returns table name with $__sampleByInterval and FILL', () => {
      expect(getTable('SELECT count(*) FROM my_table SAMPLE BY $__sampleByInterval FILL(NONE)')).toBe('my_table');
    });
  });

  describe('sqlToStatement', () => {
    it('macro parses correctly', () => {
      const sql = 'SELECT count(*) FROM foo where $__timeFilter(tstmp)';
      const stm = sqlToStatement(sql);
      const output = toSql(stm);
      expect(output).toContain('count(*)');
      expect(output).toContain('foo');
      expect(output.toLowerCase()).toContain('$__timefilter');
      expect(output).toContain('tstmp');
    });

    it('sampleByInterval macro parses correctly', () => {
      const sql = 'SELECT count(*) FROM foo sample by $__sampleByInterval';
      const stm = sqlToStatement(sql);
      const output = toSql(stm);
      expect(output).toContain('count(*)');
      expect(output).toContain('foo');
      expect(output).toContain('$__sampleByInterval');
    });

    it('variable parses correctly', () => {
      const sql = 'SELECT count(*) FROM foo where str in $query0';
      const stm = sqlToStatement(sql);
      const output = toSql(stm);
      expect(output).toContain('count(*)');
      expect(output).toContain('foo');
      expect(output).toContain('$query0');
    });

    it('returns empty object for invalid SQL', () => {
      const stm = sqlToStatement('NOT VALID SQL AT ALL');
      expect(stm).toEqual({});
    });

    it('returns empty object for empty string', () => {
      const stm = sqlToStatement('');
      expect(stm).toEqual({});
    });

    it('parses plain SQL without variables correctly', () => {
      const sql = 'SELECT a, b FROM foo WHERE x = 1';
      const stm = sqlToStatement(sql);
      expect(stm.type).toBe('select');
    });

    it('preserves $__fromTime variable', () => {
      const sql = 'SELECT * FROM foo WHERE ts > $__fromTime';
      const stm = sqlToStatement(sql);
      const output = toSql(stm);
      expect(output.toLowerCase()).toContain('$__fromtime');
    });

    it('preserves $__toTime variable', () => {
      const sql = 'SELECT * FROM foo WHERE ts < $__toTime';
      const stm = sqlToStatement(sql);
      const output = toSql(stm);
      expect(output.toLowerCase()).toContain('$__totime');
    });

    it('handles multiple $ variables in one query', () => {
      const sql = 'SELECT $col FROM foo WHERE col = $var';
      const stm = sqlToStatement(sql);
      const output = toSql(stm);
      expect(output).toContain('$col');
      expect(output).toContain('$var');
    });

    it('handles $ variable in WHERE value', () => {
      const sql = 'SELECT * FROM foo WHERE col = $var';
      const stm = sqlToStatement(sql);
      const output = toSql(stm);
      expect(output).toContain('$var');
    });

    it('preserves $__sampleByInterval with FILL', () => {
      const sql = 'SELECT count(*) FROM foo SAMPLE BY $__sampleByInterval FILL(NONE)';
      const stm = sqlToStatement(sql);
      const output = toSql(stm);
      expect(output).toContain('$__sampleByInterval');
    });

    it('parsed AST has correct type for SELECT', () => {
      const sql = 'SELECT a FROM foo';
      const stm = sqlToStatement(sql);
      expect(stm.type).toBe('select');
    });

    it('parsed AST has FROM populated', () => {
      const sql = 'SELECT a FROM foo';
      const stm = sqlToStatement(sql) as any;
      expect(stm.from).toBeDefined();
      expect(stm.from.length).toBe(1);
    });

    it('parsed AST has WHERE populated when filter exists', () => {
      const sql = 'SELECT a FROM foo WHERE x = 1';
      const stm = sqlToStatement(sql) as any;
      expect(stm.where).toBeDefined();
    });

    it('parsed AST has columns populated', () => {
      const sql = 'SELECT a, b FROM foo';
      const stm = sqlToStatement(sql) as any;
      expect(stm.columns).toBeDefined();
      expect(stm.columns.length).toBe(2);
    });

    it('parsed AST has SAMPLE BY populated', () => {
      const sql = 'SELECT count(*) FROM foo SAMPLE BY $__sampleByInterval';
      const stm = sqlToStatement(sql) as any;
      expect(stm.sampleBy).toBeDefined();
    });

    it('handles $__timeFilter in combination with other variables', () => {
      const sql = 'SELECT count(*) FROM foo WHERE $__timeFilter(ts) AND col = $var';
      const stm = sqlToStatement(sql);
      const output = toSql(stm);
      expect(output.toLowerCase()).toContain('$__timefilter');
      expect(output).toContain('$var');
    });
  });

  describe('deepReplaceStrings', () => {
    const replaceFuncs: ReplaceFuncItem[] = [
      { startIndex: 0, name: '$__timeFilter', replacementName: 'qdbvar0' },
      { startIndex: 0, name: '$var', replacementName: 'qdbvar1' },
    ];

    it('replaces a matching string', () => {
      expect(deepReplaceStrings('qdbvar0', replaceFuncs)).toBe('$__timeFilter');
    });

    it('replaces second replacement in string', () => {
      expect(deepReplaceStrings('qdbvar1', replaceFuncs)).toBe('$var');
    });

    it('returns non-matching string unchanged', () => {
      expect(deepReplaceStrings('hello', replaceFuncs)).toBe('hello');
    });

    it('returns number unchanged', () => {
      expect(deepReplaceStrings(42, replaceFuncs)).toBe(42);
    });

    it('returns boolean unchanged', () => {
      expect(deepReplaceStrings(true, replaceFuncs)).toBe(true);
    });

    it('returns null unchanged', () => {
      expect(deepReplaceStrings(null, replaceFuncs)).toBe(null);
    });

    it('returns undefined unchanged', () => {
      expect(deepReplaceStrings(undefined, replaceFuncs)).toBe(undefined);
    });

    it('replaces strings inside a flat array', () => {
      expect(deepReplaceStrings(['qdbvar0', 'qdbvar1', 'other'], replaceFuncs))
        .toEqual(['$__timeFilter', '$var', 'other']);
    });

    it('replaces strings inside object values', () => {
      expect(deepReplaceStrings({ a: 'qdbvar0', b: 'qdbvar1', c: 'keep' }, replaceFuncs))
        .toEqual({ a: '$__timeFilter', b: '$var', c: 'keep' });
    });

    it('replaces strings in deeply nested objects', () => {
      const input = { level1: { level2: { level3: 'qdbvar0' } } };
      expect(deepReplaceStrings(input, replaceFuncs))
        .toEqual({ level1: { level2: { level3: '$__timeFilter' } } });
    });

    it('replaces strings in arrays nested inside objects', () => {
      const input = { items: ['qdbvar0', 'qdbvar1'] };
      expect(deepReplaceStrings(input, replaceFuncs))
        .toEqual({ items: ['$__timeFilter', '$var'] });
    });

    it('replaces strings in objects nested inside arrays', () => {
      const input = [{ name: 'qdbvar0' }, { name: 'qdbvar1' }];
      expect(deepReplaceStrings(input, replaceFuncs))
        .toEqual([{ name: '$__timeFilter' }, { name: '$var' }]);
    });

    it('leaves non-string values in mixed objects unchanged', () => {
      const input = { a: 'qdbvar0', b: 42, c: true, d: null };
      expect(deepReplaceStrings(input, replaceFuncs))
        .toEqual({ a: '$__timeFilter', b: 42, c: true, d: null });
    });

    it('replaces partial match within a string', () => {
      expect(deepReplaceStrings('prefix_qdbvar0_suffix', replaceFuncs)).toBe('prefix_$__timeFilter_suffix');
    });

    it('returns empty object unchanged', () => {
      expect(deepReplaceStrings({}, replaceFuncs)).toEqual({});
    });

    it('returns empty array unchanged', () => {
      expect(deepReplaceStrings([], replaceFuncs)).toEqual([]);
    });

    it('does nothing with empty replaceFuncs', () => {
      const input = { a: 'qdbvar0', b: [1, 'qdbvar1'] };
      expect(deepReplaceStrings(input, [])).toEqual(input);
    });
  });
});
