import { AdHocFilter, AdHocVariableFilter } from './adHocFilter';

describe('AdHocManager', () => {
  it('apply ad hoc filter with no inner query and existing WHERE', () => {
    const ahm = new AdHocFilter();
    ahm.setTargetTableFromQuery('SELECT * FROM foo');
    const val = ahm.apply('SELECT stuff FROM foo WHERE col = test', [
      { key: 'key', operator: '=', value: 'val' },
      { key: 'keyNum', operator: '=', value: '123' },
    ] as AdHocVariableFilter[]);
    expect(val).toEqual(
      `SELECT * FROM (SELECT stuff FROM foo WHERE col = test) WHERE  key = 'val' AND keyNum = 123 `
    );
  });
  it('apply ad hoc filter with no inner query and no existing WHERE', () => {
    const ahm = new AdHocFilter();
    ahm.setTargetTableFromQuery('SELECT * FROM foo');
    const val = ahm.apply('SELECT stuff FROM foo', [
      { key: 'key', operator: '=', value: 'val' },
      { key: 'keyNum', operator: '=', value: '123' },
    ] as AdHocVariableFilter[]);
    expect(val).toEqual(
      `SELECT * FROM (SELECT stuff FROM foo) WHERE  key = 'val' AND keyNum = 123 `
    );
  });
  it('apply ad hoc filter with an inner query without existing WHERE', () => {
    const ahm = new AdHocFilter();
    ahm.setTargetTableFromQuery('SELECT * FROM foo');
    const val = ahm.apply(`SELECT stuff FROM (SELECT * FROM foo) as r , bar GROUP BY s ORDER BY s`, [
      { key: 'key', operator: '=', value: 'val' },
      { key: 'keyNum', operator: '=', value: '123' },
    ] as AdHocVariableFilter[]);
    expect(val).toEqual(
      `SELECT * FROM (SELECT stuff FROM (SELECT * FROM foo) as r , bar GROUP BY s ORDER BY s) WHERE  key = 'val' AND keyNum = 123 `
    );
  });
  it('apply ad hoc filter with an inner from query with existing WHERE', () => {
    const ahm = new AdHocFilter();
    ahm.setTargetTableFromQuery('SELECT * FROM foo');
    const val = ahm.apply(`SELECT stuff FROM (SELECT * FROM foo WHERE col = test) as r GROUP BY s ORDER BY s`, [
      { key: 'key', operator: '=', value: 'val' },
      { key: 'keyNum', operator: '=', value: '123' },
    ] as AdHocVariableFilter[]);
    expect(val).toEqual(
      `SELECT * FROM (SELECT stuff FROM (SELECT * FROM foo WHERE col = test) as r GROUP BY s ORDER BY s) WHERE  key = 'val' AND keyNum = 123 `
    );
  });
  it('apply ad hoc filter with an inner where query with existing WHERE', () => {
    const ahm = new AdHocFilter();
    ahm.setTargetTableFromQuery('SELECT * FROM foo');
    const val = ahm.apply(
      `SELECT * FROM foo WHERE (name = stuff) AND (name IN ( SELECT * FROM foo WHERE (field = 'hello') GROUP BY name ORDER BY count() DESC LIMIT 10 )) GROUP BY name , time ORDER BY time`,
      [{ key: 'key', operator: '=', value: 'val' }] as AdHocVariableFilter[]
    );
    expect(val).toEqual(
      `SELECT * FROM (SELECT * FROM foo WHERE (name = stuff) AND (name IN ( SELECT * FROM foo WHERE (field = 'hello') GROUP BY name ORDER BY count() DESC LIMIT 10 )) GROUP BY name , time ORDER BY time) WHERE  key = 'val' `
    );
  });
  it('does not apply ad hoc filter when the target table is not in the query', () => {
    const ahm = new AdHocFilter();
    ahm.setTargetTableFromQuery('SELECT * FROM bar');
    const val = ahm.apply('select stuff FROM foo', [
      { key: 'key', operator: '=', value: 'val' },
    ] as AdHocVariableFilter[]);
    expect(val).toEqual('select stuff FROM foo');
  });
  it('apply ad hoc filter when the ad hoc options are from a query with a from inline query', () => {
    const ahm = new AdHocFilter();
    ahm.setTargetTableFromQuery('SELECT * FROM (select * FROM foo) bar');
    const val = ahm.apply('select stuff FROM foo', [
      { key: 'key', operator: '=', value: 'val' },
    ] as AdHocVariableFilter[]);
    expect(val).toEqual(`SELECT * FROM (select stuff FROM foo) WHERE  key = 'val' `);
  });
  it('apply ad hoc filter when the ad hoc options are from a query with a where inline query', () => {
    const ahm = new AdHocFilter();
    ahm.setTargetTableFromQuery(
      'SELECT * FROM foo where stuff = stuff and (repo in (select * FROM foo)) order by stuff'
    );
    const val = ahm.apply('select stuff FROM foo', [
      { key: 'key', operator: '=', value: 'val' },
    ] as AdHocVariableFilter[]);
    expect(val).toEqual(`SELECT * FROM (select stuff FROM foo) WHERE  key = 'val' `);
  });
  it('apply ad hoc filter to complex join statement', () => {
    const ahm = new AdHocFilter();
    ahm.setTargetTableFromQuery(
      'SELECT * FROM foo where stuff = stuff and (repo in (select * FROM foo)) order by stuff'
    );
    const val = ahm.apply(
      'SELECT number, letter FROM foo AS x ' +
           'INNER JOIN (SELECT number FROM bar LIMIT 5) AS inner_numbers ON inner_numbers.number = x.number LIMIT 5',
      [{ key: 'key', operator: '=', value: 'val' }] as AdHocVariableFilter[]
    );
    expect(val).toEqual(
      "SELECT * FROM (SELECT number, letter FROM foo AS x " +
              "INNER JOIN (SELECT number FROM bar LIMIT 5) AS inner_numbers ON inner_numbers.number = x.number LIMIT 5) WHERE  key = 'val' "
    );
  });

  it('throws an error when the adhoc filter select cannot be parsed', () => {
    const ahm = new AdHocFilter();
    expect(function () {
      ahm.setTargetTableFromQuery('select not sql');
    }).toThrow(new Error('Failed to get table from adhoc query.'));
  });

  it('apply ad hoc filter and does not include the table reference in the selected fields of the function', () => {
    const ahm = new AdHocFilter();
    ahm.setTargetTableFromQuery('SELECT * FROM foo');
    const val = ahm.apply('SELECT foo.stuff FROM foo', [
      { key: 'foo.key', operator: '=', value: 'val' },
    ] as AdHocVariableFilter[]);
    expect(val).toEqual(`SELECT * FROM (SELECT foo.stuff FROM foo) WHERE  key = 'val' `);
  });

  it('does not apply an adhoc filter without "operator"', () => {
    const ahm = new AdHocFilter();
    ahm.setTargetTableFromQuery('SELECT * FROM foo');
    const val = ahm.apply('SELECT foo.stuff FROM foo', [
      // @ts-expect-error
      { key: 'foo.key', operator: undefined, value: 'val' },
    ]);
    expect(val).toEqual(`SELECT foo.stuff FROM foo`);
  });

  it('does not apply an adhoc filter without "value"', () => {
    const ahm = new AdHocFilter();
    ahm.setTargetTableFromQuery('SELECT * FROM foo');
    const val = ahm.apply('SELECT foo.stuff FROM foo', [
      // @ts-expect-error
      { key: 'foo.key', operator: '=', value: undefined },
    ]);
    expect(val).toEqual(`SELECT foo.stuff FROM foo`);
  });

  it('does not apply an adhoc filter without "key"', () => {
    const ahm = new AdHocFilter();
    ahm.setTargetTableFromQuery('SELECT * FROM foo');
    const val = ahm.apply('SELECT foo.stuff FROM foo', [
      // @ts-expect-error
      { key: undefined, operator: '=', value: 'val' },
    ]);
    expect(val).toEqual(`SELECT foo.stuff FROM foo`);
  });

  it('log a malformed filter', () => {
    const warn = jest.spyOn(console, 'error');
    const value = { key: 'foo.key', operator: '=', value: undefined };
    const ahm = new AdHocFilter();
    ahm.setTargetTableFromQuery('SELECT * FROM foo');
    ahm.apply('SELECT foo.stuff FROM foo', [
      // @ts-expect-error
      value,
    ]);
    expect(warn).toHaveBeenCalledTimes(1);
    expect(warn).toHaveBeenCalledWith('Invalid adhoc filter will be ignored:', value);
  });
});
