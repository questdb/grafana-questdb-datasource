import {
  BuilderMetricFieldAggregation,
  BuilderMode,
  FilterOperator,
  FullField,
  OrderByDirection,
  SampleByAlignToMode,
} from 'types';
import { getQueryOptionsFromSql, getSQLFromQueryOptions, isDateType, isNumberType, isTimestampType } from './utils';
import { Datasource } from '../../data/QuestDbDatasource';
import { PluginType } from '@grafana/data';

let mockTimeField = '';

const mockDatasource = new Datasource({
  id: 1,
  uid: 'questdb_ds',
  type: 'questdb-questdb-datasource',
  name: 'QuestDB',
  jsonData: {
    server: 'foo.com',
    port: 443,
    username: 'user',
  },
  readOnly: true,
  access: 'direct',
  meta: {
    id: 'questdb-questdb-datasource',
    name: 'QuestDB',
    type: PluginType.datasource,
    module: '',
    baseUrl: '',
    info: {
      description: '',
      screenshots: [],
      updated: '',
      version: '',
      logos: {
        small: '',
        large: '',
      },
      author: {
        name: '',
      },
      links: [],
    },
  },
});

mockDatasource.fetchFields = async function (table: string): Promise<FullField[]> {
  if (mockTimeField.length > 0) {
    return [{ name: mockTimeField, label: mockTimeField, designated: true, type: 'timestamp', picklistValues: [] }];
  } else {
    return [];
  }
};

describe('isDateType', () => {
  it('returns true for Date type', () => {
    expect(isDateType('Date')).toBe(true);
    expect(isDateType('date')).toBe(true);
    expect(isDateType('Timestamp')).toBe(true);
    expect(isDateType('timestamp')).toBe(true);
  });

  it('returns true for timestamp_ns type', () => {
    expect(isDateType('timestamp_ns')).toBe(true);
    expect(isDateType('Timestamp_ns')).toBe(true);
    expect(isDateType('TIMESTAMP_NS')).toBe(true);
  });

  it('returns false for other types', () => {
    expect(isDateType('boolean')).toBe(false);
    expect(isDateType('Boolean')).toBe(false);
  });
});

describe('isTimestampType', () => {
  it('returns true for timestamp type', () => {
    expect(isTimestampType('Timestamp')).toBe(true);
    expect(isTimestampType('timestamp')).toBe(true);
  });
  it('returns true for timestamp_ns type', () => {
    expect(isTimestampType('timestamp_ns')).toBe(true);
    expect(isTimestampType('Timestamp_ns')).toBe(true);
    expect(isTimestampType('TIMESTAMP_NS')).toBe(true);
  });
  it('returns false for Date type', () => {
    expect(isTimestampType('Date')).toBe(false);
    expect(isTimestampType('date')).toBe(false);
  });
  it('returns false for other types', () => {
    expect(isTimestampType('boolean')).toBe(false);
    expect(isTimestampType('String')).toBe(false);
  });
});

describe('isNumberType', () => {
  it('returns true for integer and float types', () => {
    expect(isNumberType('byte')).toBe(true);
    expect(isNumberType('short')).toBe(true);
    expect(isNumberType('int')).toBe(true);
    expect(isNumberType('long')).toBe(true);
    expect(isNumberType('float')).toBe(true);
    expect(isNumberType('double')).toBe(true);
  });

  it('returns false for other types', () => {
    expect(isNumberType('boolean')).toBe(false);
    expect(isNumberType('date')).toBe(false);
    expect(isNumberType('timestamp')).toBe(false);
    expect(isNumberType('string')).toBe(false);
    expect(isNumberType('symbol')).toBe(false);
  });
});

describe('Utils: getSQLFromQueryOptions and getQueryOptionsFromSql', () => {
  it(
    'handles a table without a database',
    test('SELECT name FROM "tab"', {
      mode: BuilderMode.List,
      table: 'tab',
      fields: ['name'],
      timeField: '',
    })
  );

  it(
    'handles a table with a dot',
    test('SELECT name FROM "foo.bar"', {
      mode: BuilderMode.List,
      table: 'foo.bar',
      fields: ['name'],
      timeField: '',
    })
  );

  it(
    'handles 2 fields',
    test('SELECT field1, field2 FROM "tab"', {
      mode: BuilderMode.List,
      table: 'tab',
      fields: ['field1', 'field2'],
      timeField: '',
    })
  );

  it(
    'handles a limit wih upper bound',
    test('SELECT field1, field2 FROM "tab" LIMIT 20', {
      mode: BuilderMode.List,
      table: 'tab',
      fields: ['field1', 'field2'],
      limit: '20',
      timeField: '',
    })
  );

  it(
    'handles a limit with lower and upper bound',
    test('SELECT field1, field2 FROM "tab" LIMIT 10, 20', {
      mode: BuilderMode.List,
      table: 'tab',
      fields: ['field1', 'field2'],
      limit: '10, 20',
      timeField: '',
    })
  );

  it(
    'handles empty orderBy array',
    test(
      'SELECT field1, field2 FROM "tab" LIMIT 20',
      {
        mode: BuilderMode.List,
        table: 'tab',
        fields: ['field1', 'field2'],
        orderBy: [],
        limit: 20,
        timeField: '',
      },
      false
    )
  );

  it(
    'handles order by',
    test('SELECT field1, field2 FROM "tab" ORDER BY field1 ASC LIMIT 20', {
      mode: BuilderMode.List,
      table: 'tab',
      fields: ['field1', 'field2'],
      orderBy: [{ name: 'field1', dir: OrderByDirection.ASC }],
      limit: '20',
      timeField: '',
    })
  );

  it(
    'handles no select',
    test(
      'SELECT  FROM "tab"',
      {
        mode: BuilderMode.Aggregate,
        table: 'tab',
        fields: [],
        metrics: [],
        timeField: '',
      },
      false
    )
  );

  it(
    'does not escape * field',
    test(
      'SELECT * FROM "tab"',
      {
        mode: BuilderMode.Aggregate,
        table: 'tab',
        fields: ['*'],
        metrics: [],
        timeField: '',
      },
      false
    )
  );

  it(
    'handles aggregation function',
    test('SELECT sum(field1) FROM "tab"', {
      mode: BuilderMode.Aggregate,
      table: 'tab',
      fields: [],
      metrics: [{ field: 'field1', aggregation: BuilderMetricFieldAggregation.Sum }],
      timeField: '',
    })
  );

  it(
    'handles aggregation with alias',
    test('SELECT sum(field1) total_records FROM "tab"', {
      mode: BuilderMode.Aggregate,
      table: 'tab',
      fields: [],
      metrics: [{ field: 'field1', aggregation: BuilderMetricFieldAggregation.Sum, alias: 'total_records' }],
      timeField: '',
    })
  );

  it(
    'handles 2 aggregations',
    test('SELECT sum(field1) total_records, count(field2) total_records2 FROM "tab"', {
      mode: BuilderMode.Aggregate,
      table: 'tab',
      fields: [],
      metrics: [
        { field: 'field1', aggregation: BuilderMetricFieldAggregation.Sum, alias: 'total_records' },
        { field: 'field2', aggregation: BuilderMetricFieldAggregation.Count, alias: 'total_records2' },
      ],
      timeField: '',
    })
  );

  it(
    'handles aggregation with groupBy',
    test(
      'SELECT field3, sum(field1) total_records, count(field2) total_records2 FROM "tab" GROUP BY field3',
      {
        mode: BuilderMode.Aggregate,
        table: 'tab',
        database: 'db',
        fields: [],
        metrics: [
          { field: 'field1', aggregation: BuilderMetricFieldAggregation.Sum, alias: 'total_records' },
          { field: 'field2', aggregation: BuilderMetricFieldAggregation.Count, alias: 'total_records2' },
        ],
        groupBy: ['field3'],
        timeField: '',
      },
      false
    )
  );

  it(
    'handles aggregation with groupBy with fields having group by value',
    test('SELECT field3, sum(field1) total_records, count(field2) total_records2 FROM "tab" GROUP BY field3', {
      mode: BuilderMode.Aggregate,
      table: 'tab',
      fields: ['field3'],
      metrics: [
        { field: 'field1', aggregation: BuilderMetricFieldAggregation.Sum, alias: 'total_records' },
        { field: 'field2', aggregation: BuilderMetricFieldAggregation.Count, alias: 'total_records2' },
      ],
      groupBy: ['field3'],
      timeField: '',
    })
  );

  it(
    'handles aggregation with group by and order by',
    test(
      'SELECT StageName, Type, count(Id) count_of, sum(Amount) FROM "tab" GROUP BY StageName, Type ORDER BY count(Id) DESC, StageName ASC',
      {
        mode: BuilderMode.Aggregate,
        table: 'tab',
        fields: [],
        metrics: [
          { field: 'Id', aggregation: BuilderMetricFieldAggregation.Count, alias: 'count_of' },
          { field: 'Amount', aggregation: BuilderMetricFieldAggregation.Sum },
        ],
        groupBy: ['StageName', 'Type'],
        orderBy: [
          { name: 'count(Id)', dir: OrderByDirection.DESC },
          { name: 'StageName', dir: OrderByDirection.ASC },
        ],
        timeField: '',
      },
      false
    )
  );

  it(
    'handles aggregation with a IN filter',
    test(`SELECT count(id) FROM "tab" WHERE stagename IN ('Deal Won', 'Deal Lost' )`, {
      mode: BuilderMode.Aggregate,
      table: 'tab',
      fields: [],
      metrics: [{ field: 'id', aggregation: BuilderMetricFieldAggregation.Count }],
      filters: [
        {
          key: 'stagename',
          operator: FilterOperator.In,
          value: ['Deal Won', 'Deal Lost'],
          type: 'string',
        },
      ],
      timeField: '',
    })
  );

  it(
    'handles aggregation with a NOT IN filter',
    test(`SELECT count(id) FROM "tab" WHERE stagename NOT IN ('Deal Won', 'Deal Lost' )`, {
      mode: BuilderMode.Aggregate,
      table: 'tab',
      fields: [],
      metrics: [{ field: 'id', aggregation: BuilderMetricFieldAggregation.Count }],
      filters: [
        {
          key: 'stagename',
          operator: FilterOperator.NotIn,
          value: ['Deal Won', 'Deal Lost'],
          type: 'string',
        },
      ],
      timeField: '',
    })
  );

  it(
    'handles $__fromTime and $__toTime filters',
    test(
      `SELECT id FROM "tab" WHERE tstmp > $__fromTime AND tstmp < $__toTime`,
      {
        mode: BuilderMode.List,
        table: 'tab',
        fields: ['id'],
        filters: [
          { key: 'tstmp', operator: '>', value: 'GRAFANA_START_TIME', type: 'timestamp' },
          { condition: 'AND', key: 'tstmp', operator: '<', value: 'GRAFANA_END_TIME', type: 'timestamp' },
        ],
        timeField: '',
      },
      true
    )
  );

  it(
    'handles aggregation with $__timeFilter',
    test(`SELECT count(id) FROM "tab" WHERE  $__timeFilter(createdon)`, {
      mode: BuilderMode.Aggregate,
      table: 'tab',
      fields: [],
      metrics: [{ field: 'id', aggregation: BuilderMetricFieldAggregation.Count }],
      filters: [
        {
          key: 'createdon',
          operator: FilterOperator.WithInGrafanaTimeRange,
          type: 'timestamp',
        },
      ],
      timeField: '',
    })
  );

  it(
    'handles aggregation with negated $__timeFilter',
    test(`SELECT count(id) FROM "tab" WHERE NOT (  $__timeFilter(closedate) )`, {
      mode: BuilderMode.Aggregate,
      table: 'tab',
      fields: [],
      metrics: [{ field: 'id', aggregation: BuilderMetricFieldAggregation.Count }],
      filters: [
        {
          key: 'closedate',
          operator: FilterOperator.OutsideGrafanaTimeRange,
          type: 'timestamp',
        },
      ],
      timeField: '',
    })
  );

  it(
    'handles latest on one column ',
    test(
      'SELECT sym, value FROM "tab" LATEST ON tstmp PARTITION BY sym',
      {
        mode: BuilderMode.List,
        table: 'tab',
        fields: ['sym', 'value'],
        timeField: 'tstmp',
        partitionBy: ['sym'],
        filters: [],
      },
      false
    )
  );

  it(
    'handles latest on two columns ',
    test(
      'SELECT s1, s2, value FROM "tab" LATEST ON tstmp PARTITION BY s1, s2 ORDER BY time ASC',
      {
        mode: BuilderMode.List,
        table: 'tab',
        fields: ['s1', 's2', 'value'],
        timeField: 'tstmp',
        partitionBy: ['s1', 's2'],
        filters: [],
        orderBy: [{ name: 'time', dir: 'ASC' }],
      },
      false
    )
  );

  it(
    'handles sample by align to calendar',
    test(
      'SELECT tstmp as time,  count(*), first(str) FROM "tab" WHERE   $__timeFilter(tstmp) SAMPLE BY $__sampleByInterval FILL ( NULL, 10 ) ALIGN TO CALENDAR',
      {
        mode: BuilderMode.Trend,
        table: 'tab',
        fields: ['tstmp'],
        sampleByAlignTo: SampleByAlignToMode.Calendar,
        sampleByFill: ['NULL', '10'],
        metrics: [
          { field: '*', aggregation: BuilderMetricFieldAggregation.Count },
          { field: 'str', aggregation: BuilderMetricFieldAggregation.First },
        ],
        filters: [
          {
            key: 'tstmp',
            operator: FilterOperator.WithInGrafanaTimeRange,
            type: 'timestamp',
          },
        ],
        timeField: 'tstmp',
      },
      true,
      'tstmp'
    )
  );

  it(
    'handles sample by align to calendar time zone',
    test(
      'SELECT tstmp as time,  count(*), first(str) FROM "tab" WHERE $__timeFilter(tstmp) SAMPLE BY $__sampleByInterval FILL ( NULL, 10 ) ALIGN TO CALENDAR TIME ZONE \'EST\'',
      {
        mode: BuilderMode.Trend,
        table: 'tab',
        fields: ['time'],
        sampleByAlignTo: SampleByAlignToMode.CalendarTimeZone,
        sampleByAlignToValue: 'EST',
        sampleByFill: ['NULL', '10'],
        metrics: [
          { field: '*', aggregation: BuilderMetricFieldAggregation.Count },
          { field: 'str', aggregation: BuilderMetricFieldAggregation.First },
        ],
        filters: [],
        timeField: 'tstmp',
      },
      false
    )
  );

  it(
    'handles sample by align to calendar offset',
    test(
      'SELECT tstmp as time,  count(*), first(str) FROM "tab" WHERE $__timeFilter(tstmp) SAMPLE BY $__sampleByInterval FILL ( NULL, 10 ) ALIGN TO CALENDAR WITH OFFSET \'01:00\'',
      {
        mode: BuilderMode.Trend,
        table: 'tab',
        fields: ['time'],
        sampleByAlignTo: SampleByAlignToMode.CalendarOffset,
        sampleByAlignToValue: '01:00',
        sampleByFill: ['NULL', '10'],
        metrics: [
          { field: '*', aggregation: BuilderMetricFieldAggregation.Count },
          { field: 'str', aggregation: BuilderMetricFieldAggregation.First },
        ],
        filters: [],
        timeField: 'tstmp',
      },
      false
    )
  );

  it(
    'handles sample by align to first observation',
    test(
      'SELECT tstmp as time,  count(*), first(str) FROM "tab" WHERE $__timeFilter(tstmp) SAMPLE BY $__sampleByInterval FILL ( NULL, 10 ) ALIGN TO FIRST OBSERVATION',
      {
        mode: BuilderMode.Trend,
        table: 'tab',
        fields: ['time'],
        sampleByAlignTo: SampleByAlignToMode.FirstObservation,
        sampleByFill: ['NULL', '10'],
        metrics: [
          { field: '*', aggregation: BuilderMetricFieldAggregation.Count },
          { field: 'str', aggregation: BuilderMetricFieldAggregation.First },
        ],
        filters: [],
        timeField: 'tstmp',
      },
      false
    )
  );

  it(
    'handles __timeFilter macro and sample by',
    test(
      'SELECT time as time FROM "tab" WHERE $__timeFilter(time) SAMPLE BY $__sampleByInterval ORDER BY time ASC',
      {
        mode: BuilderMode.Trend,
        table: 'tab',
        fields: [],
        timeField: 'time',
        metrics: [],
        filters: [],
        orderBy: [{ name: 'time', dir: 'ASC' }],
      },
      false
    )
  );

  it(
    'handles __timeFilter macro and sample by with filters',
    test(
      'SELECT time as time FROM "tab" WHERE   $__timeFilter(time) AND base IS NOT NULL AND time IS NOT NULL SAMPLE BY $__sampleByInterval',
      {
        mode: BuilderMode.Trend,
        table: 'tab',
        fields: ['time'],
        timeField: 'time',
        filters: [
          { key: 'time', operator: FilterOperator.WithInGrafanaTimeRange, type: 'timestamp' },
          { condition: 'AND', key: 'base', operator: 'IS NOT NULL' },
          { condition: 'AND', key: 'time', operator: 'IS NOT NULL', type: 'timestamp' },
        ],
      },
      true,
      'time'
    )
  );

  it(
    'handles function filter',
    test(
      'SELECT tstmp FROM "tab" WHERE tstmp > dateadd(\'M\', -1, now())',
      {
        mode: BuilderMode.List,
        table: 'tab',
        fields: ['tstmp'],
        timeField: 'tstmp',
        filters: [
          {
            key: 'tstmp',
            operator: '>',
            type: 'timestamp',
            value: "dateadd('M', -1, now())",
          },
        ],
      },
      true,
      'tstmp'
    )
  );

  it(
    'handles multiple function filters',
    test(
      "SELECT tstmp FROM \"tab\" WHERE tstmp > dateadd('M', -1, now()) AND tstmp = dateadd('M', -1, now())",
      {
        mode: BuilderMode.List,
        table: 'tab',
        fields: ['tstmp'],
        timeField: 'tstmp',
        filters: [
          { key: 'tstmp', operator: '>', type: 'timestamp', value: "dateadd('M', -1, now())" },
          { condition: 'AND', key: 'tstmp', operator: '=', type: 'timestamp', value: "dateadd('M', -1, now())" },
        ],
      },
      true,
      'tstmp'
    )
  );

  it(
    'handles boolean column ref filters',
    test(
      'SELECT tstmp, bool FROM "tab" WHERE bool = true AND tstmp > cast( \'2020-01-01\' as timestamp )',
      {
        mode: BuilderMode.List,
        table: 'tab',
        fields: ['tstmp', 'bool'],
        timeField: 'tstmp',
        filters: [
          { key: 'bool', operator: '=', type: 'boolean', value: true },
          {
            condition: 'AND',
            key: 'tstmp',
            operator: '>',
            type: 'timestamp',
            value: "cast( '2020-01-01' as timestamp )",
          },
        ],
      },
      true,
      'tstmp'
    )
  );

  it(
    'handles numeric filters',
    test(
      'SELECT tstmp, z FROM "tab" WHERE k = 1 AND j > 1.2',
      {
        mode: BuilderMode.List,
        table: 'tab',
        fields: ['tstmp', 'z'],
        timeField: 'tstmp',
        filters: [
          { key: 'k', operator: '=', type: 'int', value: 1 },
          { condition: 'AND', key: 'j', operator: '>', type: 'double', value: 1.2 },
        ],
      },
      true,
      'tstmp'
    )
  );

  // builder doesn't support nested conditions, so we flatten them
  it('flattens condition hierarchy', async () => {
    let options = await getQueryOptionsFromSql(
      'SELECT tstmp, z FROM "tab" WHERE k = 1 AND ( j > 1.2 OR p = \'start\' )',
      mockDatasource
    );
    expect(options).toEqual({
      mode: BuilderMode.List,
      table: 'tab',
      fields: ['tstmp', 'z'],
      timeField: '',
      filters: [
        { key: 'k', operator: '=', type: 'int', value: 1 },
        { condition: 'AND', key: 'j', operator: '>', type: 'double', value: 1.2 },
        { condition: 'OR', key: 'p', operator: '=', type: 'string', value: 'start' },
      ],
    });
  });

  it('handles expressions in select list', async () => {
    let options = await getQueryOptionsFromSql('SELECT tstmp, e::timestamp, f(x), g(a,b) FROM "tab"', mockDatasource);
    expect(options).toEqual({
      mode: BuilderMode.List,
      table: 'tab',
      fields: ['tstmp', 'cast(e  as timestamp)', 'f(x)', 'g(a, b)'],
      timeField: '',
    });
  });
});

describe('getQueryOptionsFromSql: error paths', () => {
  it('returns error for unparseable SQL', async () => {
    const result = await getQueryOptionsFromSql('NOT VALID SQL AT ALL', mockDatasource);
    expect(result).toBe("The query can't be parsed.");
  });

  it('returns error for empty string', async () => {
    const result = await getQueryOptionsFromSql('', mockDatasource);
    expect(result).toBe("The query can't be parsed.");
  });

  it('returns error for multiple FROM tables', async () => {
    const result = await getQueryOptionsFromSql('SELECT * FROM a, b', mockDatasource);
    expect(result).toBe("The query has too many 'FROM' clauses.");
  });

  it('returns error for FROM subquery', async () => {
    const result = await getQueryOptionsFromSql('SELECT * FROM (SELECT * FROM t) alias', mockDatasource);
    expect(result).toBe("The 'FROM' clause is not a table.");
  });
});

describe('getQueryOptionsFromSql: filter operators', () => {
  it('handles LIKE filter', async () => {
    const result = await getQueryOptionsFromSql(`SELECT a FROM "t" WHERE name LIKE '%foo%'`, mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ['a'],
      timeField: '',
      filters: [
        { key: 'name', operator: FilterOperator.Like, value: '%foo%', type: 'string' },
      ],
    });
  });

  it('handles NOT LIKE filter', async () => {
    const result = await getQueryOptionsFromSql(`SELECT a FROM "t" WHERE NOT ( name LIKE '%foo%' )`, mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ['a'],
      timeField: '',
      filters: [
        { key: 'name', operator: FilterOperator.NotLike, value: '%foo%', type: 'string' },
      ],
    });
  });

  it('handles != filter', async () => {
    const result = await getQueryOptionsFromSql(`SELECT a FROM "t" WHERE col != 'val'`, mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ['a'],
      timeField: '',
      filters: [
        { key: 'col', operator: FilterOperator.NotEquals, value: 'val', type: 'string' },
      ],
    });
  });

  it('handles >= filter', async () => {
    const result = await getQueryOptionsFromSql('SELECT a FROM "t" WHERE num >= 10', mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ['a'],
      timeField: '',
      filters: [
        { key: 'num', operator: FilterOperator.GreaterThanOrEqual, value: 10, type: 'int' },
      ],
    });
  });

  it('handles <= filter', async () => {
    const result = await getQueryOptionsFromSql('SELECT a FROM "t" WHERE num <= 10', mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ['a'],
      timeField: '',
      filters: [
        { key: 'num', operator: FilterOperator.LessThanOrEqual, value: 10, type: 'int' },
      ],
    });
  });

  it('handles < filter', async () => {
    const result = await getQueryOptionsFromSql('SELECT a FROM "t" WHERE num < 5', mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ['a'],
      timeField: '',
      filters: [
        { key: 'num', operator: FilterOperator.LessThan, value: 5, type: 'int' },
      ],
    });
  });

  it('handles IS NULL filter', async () => {
    const result = await getQueryOptionsFromSql('SELECT a FROM "t" WHERE col IS NULL', mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ['a'],
      timeField: '',
      filters: [
        { key: 'col', operator: FilterOperator.IsNull },
      ],
    });
  });

  it('handles IS NOT NULL filter', async () => {
    const result = await getQueryOptionsFromSql('SELECT a FROM "t" WHERE col IS NOT NULL', mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ['a'],
      timeField: '',
      filters: [
        { key: 'col', operator: FilterOperator.IsNotNull },
      ],
    });
  });

  it('handles boolean false filter', async () => {
    const result = await getQueryOptionsFromSql('SELECT a FROM "t" WHERE active = false', mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ['a'],
      timeField: '',
      filters: [
        { key: 'active', operator: FilterOperator.Equals, value: false, type: 'boolean' },
      ],
    });
  });

  it('handles string equality filter', async () => {
    const result = await getQueryOptionsFromSql(`SELECT a FROM "t" WHERE name = 'hello'`, mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ['a'],
      timeField: '',
      filters: [
        { key: 'name', operator: FilterOperator.Equals, value: 'hello', type: 'string' },
      ],
    });
  });

  it('handles numeric equality filter with float', async () => {
    const result = await getQueryOptionsFromSql('SELECT a FROM "t" WHERE price = 9.99', mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ['a'],
      timeField: '',
      filters: [
        { key: 'price', operator: FilterOperator.Equals, value: 9.99, type: 'double' },
      ],
    });
  });
});

describe('getQueryOptionsFromSql: complex filter conditions', () => {
  it('handles multiple AND conditions', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE x = 1 AND y = 2 AND z = 3',
      mockDatasource
    );
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ['a'],
      timeField: '',
      filters: [
        { key: 'x', operator: FilterOperator.Equals, value: 1, type: 'int' },
        { condition: 'AND', key: 'y', operator: FilterOperator.Equals, value: 2, type: 'int' },
        { condition: 'AND', key: 'z', operator: FilterOperator.Equals, value: 3, type: 'int' },
      ],
    });
  });

  it('handles OR condition', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE x = 1 OR y = 2',
      mockDatasource
    );
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ['a'],
      timeField: '',
      filters: [
        { key: 'x', operator: FilterOperator.Equals, value: 1, type: 'int' },
        { condition: 'OR', key: 'y', operator: FilterOperator.Equals, value: 2, type: 'int' },
      ],
    });
  });

  it('handles mixed AND/OR conditions', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE x = 1 AND y = 2 OR z = 3',
      mockDatasource
    );
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ['a'],
      timeField: '',
      filters: [
        { key: 'x', operator: FilterOperator.Equals, value: 1, type: 'int' },
        { condition: 'AND', key: 'y', operator: FilterOperator.Equals, value: 2, type: 'int' },
        { condition: 'OR', key: 'z', operator: FilterOperator.Equals, value: 3, type: 'int' },
      ],
    });
  });

  it('handles AND with IS NULL', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE x = 1 AND y IS NULL',
      mockDatasource
    );
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ['a'],
      timeField: '',
      filters: [
        { key: 'x', operator: FilterOperator.Equals, value: 1, type: 'int' },
        { condition: 'AND', key: 'y', operator: FilterOperator.IsNull },
      ],
    });
  });

  it('handles filter with cast value', async () => {
    mockTimeField = 'ts';
    const result = await getQueryOptionsFromSql(
      `SELECT ts FROM "t" WHERE ts > cast( '2020-01-01' as timestamp )`,
      mockDatasource
    );
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ['ts'],
      timeField: 'ts',
      filters: [
        {
          key: 'ts',
          operator: FilterOperator.GreaterThan,
          type: 'timestamp',
          value: "cast( '2020-01-01' as timestamp )",
        },
      ],
    });
    mockTimeField = '';
  });
});

describe('getQueryOptionsFromSql: aggregation functions', () => {
  it('handles avg aggregation', async () => {
    const result = await getQueryOptionsFromSql('SELECT avg(field1) FROM "t"', mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.Aggregate,
      table: 't',
      fields: [],
      metrics: [{ field: 'field1', aggregation: BuilderMetricFieldAggregation.Average }],
      timeField: '',
    });
  });

  it('handles min aggregation', async () => {
    const result = await getQueryOptionsFromSql('SELECT min(field1) FROM "t"', mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.Aggregate,
      table: 't',
      fields: [],
      metrics: [{ field: 'field1', aggregation: BuilderMetricFieldAggregation.Min }],
      timeField: '',
    });
  });

  it('handles max aggregation', async () => {
    const result = await getQueryOptionsFromSql('SELECT max(field1) FROM "t"', mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.Aggregate,
      table: 't',
      fields: [],
      metrics: [{ field: 'field1', aggregation: BuilderMetricFieldAggregation.Max }],
      timeField: '',
    });
  });

  it('handles first aggregation', async () => {
    const result = await getQueryOptionsFromSql('SELECT first(field1) FROM "t"', mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.Aggregate,
      table: 't',
      fields: [],
      metrics: [{ field: 'field1', aggregation: BuilderMetricFieldAggregation.First }],
      timeField: '',
    });
  });

  it('handles last aggregation', async () => {
    const result = await getQueryOptionsFromSql('SELECT last(field1) FROM "t"', mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.Aggregate,
      table: 't',
      fields: [],
      metrics: [{ field: 'field1', aggregation: BuilderMetricFieldAggregation.Last }],
      timeField: '',
    });
  });

  it('handles count_distinct aggregation', async () => {
    const result = await getQueryOptionsFromSql('SELECT count_distinct(field1) FROM "t"', mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.Aggregate,
      table: 't',
      fields: [],
      metrics: [{ field: 'field1', aggregation: BuilderMetricFieldAggregation.Count_Distinct }],
      timeField: '',
    });
  });

  it('handles ksum aggregation', async () => {
    const result = await getQueryOptionsFromSql('SELECT ksum(field1) FROM "t"', mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.Aggregate,
      table: 't',
      fields: [],
      metrics: [{ field: 'field1', aggregation: BuilderMetricFieldAggregation.KSum }],
      timeField: '',
    });
  });

  it('handles nsum aggregation', async () => {
    const result = await getQueryOptionsFromSql('SELECT nsum(field1) FROM "t"', mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.Aggregate,
      table: 't',
      fields: [],
      metrics: [{ field: 'field1', aggregation: BuilderMetricFieldAggregation.NSum }],
      timeField: '',
    });
  });

  it('handles count(*) special case', async () => {
    const result = await getQueryOptionsFromSql('SELECT count(*) FROM "t"', mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.Aggregate,
      table: 't',
      fields: [],
      metrics: [{ field: '*', aggregation: BuilderMetricFieldAggregation.Count }],
      timeField: '',
    });
  });

  it('handles non-aggregation function as field', async () => {
    const result = await getQueryOptionsFromSql('SELECT abs(field1) FROM "t"', mockDatasource);
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ['abs(field1)'],
      timeField: '',
    });
  });

  it('handles mix of fields, metrics, and literals', async () => {
    const result = await getQueryOptionsFromSql(
      `SELECT field1, sum(field2), 42, 'hello', true FROM "t"`,
      mockDatasource
    );
    expect(result).toEqual({
      mode: BuilderMode.Aggregate,
      table: 't',
      fields: ['field1', '42', "'hello'", 'true'],
      metrics: [{ field: 'field2', aggregation: BuilderMetricFieldAggregation.Sum }],
      timeField: '',
    });
  });
});

describe('getQueryOptionsFromSql: SAMPLE BY variations', () => {
  it('handles SAMPLE BY without FILL', async () => {
    mockTimeField = 'ts';
    const result = await getQueryOptionsFromSql(
      'SELECT ts as time, count(*) FROM "t" WHERE $__timeFilter(ts) SAMPLE BY $__sampleByInterval',
      mockDatasource
    );
    expect(result).toMatchObject({
      mode: BuilderMode.Trend,
      table: 't',
      timeField: 'ts',
      metrics: [{ field: '*', aggregation: BuilderMetricFieldAggregation.Count }],
    });
    // sampleByFill should not be set
    expect((result as any).sampleByFill).toBeUndefined();
    mockTimeField = '';
  });

  it('handles SAMPLE BY with FILL(NONE) only', async () => {
    mockTimeField = 'ts';
    const result = await getQueryOptionsFromSql(
      'SELECT ts as time, count(*) FROM "t" WHERE $__timeFilter(ts) SAMPLE BY $__sampleByInterval FILL(NONE)',
      mockDatasource
    );
    expect(result).toMatchObject({
      mode: BuilderMode.Trend,
      table: 't',
      timeField: 'ts',
    });
    expect((result as any).sampleByFill).toEqual(['NONE']);
    // sampleByAlignTo should not be set
    expect((result as any).sampleByAlignTo).toBeUndefined();
    mockTimeField = '';
  });

  it('handles SAMPLE BY with FILL(PREV)', async () => {
    mockTimeField = 'ts';
    const result = await getQueryOptionsFromSql(
      'SELECT ts as time, count(*) FROM "t" WHERE $__timeFilter(ts) SAMPLE BY $__sampleByInterval FILL(PREV)',
      mockDatasource
    );
    expect((result as any).sampleByFill).toEqual(['PREV']);
    mockTimeField = '';
  });

  it('handles SAMPLE BY with FILL(LINEAR)', async () => {
    mockTimeField = 'ts';
    const result = await getQueryOptionsFromSql(
      'SELECT ts as time, count(*) FROM "t" WHERE $__timeFilter(ts) SAMPLE BY $__sampleByInterval FILL(LINEAR)',
      mockDatasource
    );
    expect((result as any).sampleByFill).toEqual(['LINEAR']);
    mockTimeField = '';
  });

  it('handles SAMPLE BY with multiple FILL values', async () => {
    mockTimeField = 'ts';
    const result = await getQueryOptionsFromSql(
      'SELECT ts as time, count(*), sum(val) FROM "t" WHERE $__timeFilter(ts) SAMPLE BY $__sampleByInterval FILL(NONE, PREV)',
      mockDatasource
    );
    expect((result as any).sampleByFill).toEqual(['NONE', 'PREV']);
    mockTimeField = '';
  });
});

describe('getQueryOptionsFromSql: ORDER BY variations', () => {
  it('handles ORDER BY DESC', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" ORDER BY a DESC',
      mockDatasource
    );
    expect(result).toMatchObject({
      orderBy: [{ name: 'a', dir: OrderByDirection.DESC }],
    });
  });

  it('handles multiple ORDER BY clauses', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a, b FROM "t" ORDER BY a ASC, b DESC',
      mockDatasource
    );
    expect(result).toMatchObject({
      orderBy: [
        { name: 'a', dir: OrderByDirection.ASC },
        { name: 'b', dir: OrderByDirection.DESC },
      ],
    });
  });

  it('filters out ORDER BY time', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" ORDER BY time ASC',
      mockDatasource
    );
    // 'time' should be filtered out, so no orderBy
    expect((result as any).orderBy).toBeUndefined();
  });

  it('handles no ORDER BY', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t"',
      mockDatasource
    );
    expect((result as any).orderBy).toBeUndefined();
  });
});

describe('getQueryOptionsFromSql: LIMIT variations', () => {
  it('handles no LIMIT', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t"',
      mockDatasource
    );
    expect((result as any).limit).toBeUndefined();
  });

  it('handles simple LIMIT', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" LIMIT 50',
      mockDatasource
    );
    expect((result as any).limit).toBe('50');
  });

  it('handles LIMIT with offset', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" LIMIT 5, 50',
      mockDatasource
    );
    expect((result as any).limit).toBe('5, 50');
  });
});

describe('getQueryOptionsFromSql: GROUP BY variations', () => {
  it('handles multiple GROUP BY', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a, b, count(*) FROM "t" GROUP BY a, b',
      mockDatasource
    );
    expect(result).toMatchObject({
      groupBy: ['a', 'b'],
    });
  });

  it('filters out GROUP BY time', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT time, count(*) FROM "t" GROUP BY time',
      mockDatasource
    );
    // 'time' should be filtered out
    expect((result as any).groupBy).toBeUndefined();
  });

  it('handles GROUP BY with aggregation', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT category, sum(amount) FROM "t" GROUP BY category',
      mockDatasource
    );
    expect(result).toEqual({
      mode: BuilderMode.Aggregate,
      table: 't',
      fields: ['category'],
      metrics: [{ field: 'amount', aggregation: BuilderMetricFieldAggregation.Sum }],
      groupBy: ['category'],
      timeField: '',
    });
  });
});

describe('getQueryOptionsFromSql: select list edge cases', () => {
  it('handles cast expression in select', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT col::timestamp FROM "t"',
      mockDatasource
    );
    expect(result).toMatchObject({
      mode: BuilderMode.List,
      table: 't',
    });
    expect((result as any).fields[0]).toContain('cast');
    expect((result as any).fields[0]).toContain('timestamp');
  });

  it('handles string literal in select', async () => {
    const result = await getQueryOptionsFromSql(
      `SELECT 'hello' FROM "t"`,
      mockDatasource
    );
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ["'hello'"],
      timeField: '',
    });
  });

  it('handles numeric literal in select', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT 42 FROM "t"',
      mockDatasource
    );
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ['42'],
      timeField: '',
    });
  });

  it('handles boolean literal in select', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT true FROM "t"',
      mockDatasource
    );
    expect(result).toEqual({
      mode: BuilderMode.List,
      table: 't',
      fields: ['true'],
      timeField: '',
    });
  });

  it('handles aggregation with alias', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT avg(price) avg_price FROM "t"',
      mockDatasource
    );
    expect(result).toEqual({
      mode: BuilderMode.Aggregate,
      table: 't',
      fields: [],
      metrics: [{ field: 'price', aggregation: BuilderMetricFieldAggregation.Average, alias: 'avg_price' }],
      timeField: '',
    });
  });
});

describe('getQueryOptionsFromSql: LATEST ON', () => {
  it('handles LATEST ON parsing with single partition', async () => {
    mockTimeField = 'ts';
    const result = await getQueryOptionsFromSql(
      'SELECT sym, value FROM "t" LATEST ON ts PARTITION BY sym',
      mockDatasource
    );
    expect(result).toMatchObject({
      mode: BuilderMode.List,
      table: 't',
      fields: ['sym', 'value'],
      timeField: 'ts',
      partitionBy: ['sym'],
    });
    mockTimeField = '';
  });

  it('handles LATEST ON parsing with multiple partitions', async () => {
    mockTimeField = 'ts';
    const result = await getQueryOptionsFromSql(
      'SELECT s1, s2, value FROM "t" LATEST ON ts PARTITION BY s1, s2',
      mockDatasource
    );
    expect(result).toMatchObject({
      mode: BuilderMode.List,
      partitionBy: ['s1', 's2'],
    });
    mockTimeField = '';
  });
});

describe('getQueryOptionsFromSql: SAMPLE BY ALIGN TO with value', () => {
  it('handles ALIGN TO CALENDAR TIME ZONE with value', async () => {
    mockTimeField = 'ts';
    const result = await getQueryOptionsFromSql(
      "SELECT ts as time, count(*) FROM \"t\" WHERE $__timeFilter(ts) SAMPLE BY $__sampleByInterval FILL(NONE) ALIGN TO CALENDAR TIME ZONE 'EST'",
      mockDatasource
    );
    expect(result).toMatchObject({
      mode: BuilderMode.Trend,
      sampleByAlignTo: SampleByAlignToMode.CalendarTimeZone,
      sampleByAlignToValue: 'EST',
    });
    mockTimeField = '';
  });

  it('handles ALIGN TO CALENDAR WITH OFFSET with value', async () => {
    mockTimeField = 'ts';
    const result = await getQueryOptionsFromSql(
      "SELECT ts as time, count(*) FROM \"t\" WHERE $__timeFilter(ts) SAMPLE BY $__sampleByInterval ALIGN TO CALENDAR WITH OFFSET '01:00'",
      mockDatasource
    );
    expect(result).toMatchObject({
      mode: BuilderMode.Trend,
      sampleByAlignTo: SampleByAlignToMode.CalendarOffset,
      sampleByAlignToValue: '01:00',
    });
    mockTimeField = '';
  });

  it('handles ALIGN TO FIRST OBSERVATION (no value)', async () => {
    mockTimeField = 'ts';
    const result = await getQueryOptionsFromSql(
      'SELECT ts as time, count(*) FROM "t" WHERE $__timeFilter(ts) SAMPLE BY $__sampleByInterval ALIGN TO FIRST OBSERVATION',
      mockDatasource
    );
    expect(result).toMatchObject({
      mode: BuilderMode.Trend,
      sampleByAlignTo: SampleByAlignToMode.FirstObservation,
    });
    expect((result as any).sampleByAlignToValue).toBeUndefined();
    mockTimeField = '';
  });

  it('handles SAMPLE BY with FILL(NULL)', async () => {
    mockTimeField = 'ts';
    const result = await getQueryOptionsFromSql(
      'SELECT ts as time, count(*) FROM "t" WHERE $__timeFilter(ts) SAMPLE BY $__sampleByInterval FILL(NULL)',
      mockDatasource
    );
    expect((result as any).sampleByFill).toEqual(['NULL']);
    mockTimeField = '';
  });
});

describe('getQueryOptionsFromSql: WHERE with function calls', () => {
  it('handles standalone function call in WHERE', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE $__timeFilter(ts)',
      mockDatasource
    );
    expect(result).toMatchObject({
      filters: [
        {
          key: 'ts',
          operator: FilterOperator.WithInGrafanaTimeRange,
          type: 'timestamp',
        },
      ],
    });
  });

  it('handles negated $__timeFilter in WHERE', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE NOT ($__timeFilter(ts))',
      mockDatasource
    );
    expect(result).toMatchObject({
      filters: [
        {
          key: 'ts',
          operator: FilterOperator.OutsideGrafanaTimeRange,
          type: 'timestamp',
        },
      ],
    });
  });

  it('handles function call after AND in WHERE', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE col = 1 AND $__timeFilter(ts)',
      mockDatasource
    );
    expect(result).toMatchObject({
      filters: [
        { key: 'col', operator: FilterOperator.Equals },
        {
          key: 'ts',
          operator: FilterOperator.WithInGrafanaTimeRange,
          type: 'timestamp',
          condition: 'AND',
        },
      ],
    });
  });
});

describe('getQueryOptionsFromSql: date filter values', () => {
  it('handles filter with $__fromTime value', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE ts > $__fromTime',
      mockDatasource
    );
    expect(result).toMatchObject({
      filters: [
        {
          key: 'ts',
          operator: FilterOperator.GreaterThan,
          value: 'GRAFANA_START_TIME',
          type: 'timestamp',
        },
      ],
    });
  });

  it('handles filter with $__toTime value', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE ts < $__toTime',
      mockDatasource
    );
    expect(result).toMatchObject({
      filters: [
        {
          key: 'ts',
          operator: FilterOperator.LessThan,
          value: 'GRAFANA_END_TIME',
          type: 'timestamp',
        },
      ],
    });
  });
});

describe('getQueryOptionsFromSql: GROUP BY with timeField', () => {
  it('handles GROUP BY with timeField and aggregation in Trend mode', async () => {
    mockTimeField = 'ts';
    const result = await getQueryOptionsFromSql(
      'SELECT ts as time, sym, count(*) FROM "t" WHERE $__timeFilter(ts) SAMPLE BY $__sampleByInterval',
      mockDatasource
    );
    expect(result).toMatchObject({
      mode: BuilderMode.Trend,
      table: 't',
    });
    mockTimeField = '';
  });
});

describe('getQueryOptionsFromSql: function calls with various arg types in WHERE', () => {
  it('handles function call with string argument in WHERE', async () => {
    const result = await getQueryOptionsFromSql(
      "SELECT a FROM \"t\" WHERE col = func('hello')",
      mockDatasource
    );
    expect(result).toMatchObject({
      filters: [
        {
          key: 'col',
          operator: FilterOperator.Equals,
          value: "func('hello')",
        },
      ],
    });
  });

  it('handles function call with numeric argument in WHERE', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE col = func(42)',
      mockDatasource
    );
    expect(result).toMatchObject({
      filters: [
        {
          key: 'col',
          operator: FilterOperator.Equals,
          value: 'func(42)',
        },
      ],
    });
  });

  it('handles function call with boolean argument in WHERE', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE col = func(true)',
      mockDatasource
    );
    expect(result).toMatchObject({
      filters: [
        {
          key: 'col',
          operator: FilterOperator.Equals,
          value: 'func(true)',
        },
      ],
    });
  });

  it('handles function call with NULL argument in WHERE', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE col = func(NULL)',
      mockDatasource
    );
    expect(result).toMatchObject({
      filters: [
        {
          key: 'col',
          operator: FilterOperator.Equals,
          value: 'func(null)',
        },
      ],
    });
  });

  it('handles function call with multiple mixed args in WHERE', async () => {
    const result = await getQueryOptionsFromSql(
      "SELECT a FROM \"t\" WHERE col = func('hello', 42, true)",
      mockDatasource
    );
    expect(result).toMatchObject({
      filters: [
        {
          key: 'col',
          operator: FilterOperator.Equals,
          value: "func('hello', 42, true)",
        },
      ],
    });
  });

  it('handles standalone non-timeFilter function in WHERE', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE myfunc(x, y)',
      mockDatasource
    );
    expect(result).toMatchObject({
      filters: [
        {
          key: 'true',
          type: 'boolean',
          value: 'myfunc(x, y)',
        },
      ],
    });
  });
});

describe('getQueryOptionsFromSql: LATEST ON with qualified partition', () => {
  it('handles partition column with table prefix', async () => {
    mockTimeField = 'ts';
    const result = await getQueryOptionsFromSql(
      'SELECT sym, value FROM "t" LATEST ON ts PARTITION BY t.sym',
      mockDatasource
    );
    expect(result).toMatchObject({
      mode: BuilderMode.List,
      partitionBy: ['t.sym'],
    });
    mockTimeField = '';
  });
});

describe('getQueryOptionsFromSql: integer filter value', () => {
  it('handles integer value in WHERE', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE col = 42',
      mockDatasource
    );
    expect(result).toMatchObject({
      filters: [
        {
          key: 'col',
          operator: FilterOperator.Equals,
          value: 42,
          type: 'int',
        },
      ],
    });
  });
});

describe('getQueryOptionsFromSql: non-aggregation functions in SELECT', () => {
  it('handles non-agg function with string arg in SELECT', async () => {
    const result = await getQueryOptionsFromSql(
      "SELECT to_str(ts, 'yyyy') FROM \"t\"",
      mockDatasource
    );
    expect(result).toMatchObject({
      fields: ["to_str(ts, 'yyyy')"],
    });
  });

  it('handles non-agg function with numeric arg in SELECT', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT round(price, 2) FROM "t"',
      mockDatasource
    );
    expect(result).toMatchObject({
      fields: ['round(price, 2)'],
    });
  });

  it('handles non-agg function with no args in SELECT', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT now() FROM "t"',
      mockDatasource
    );
    expect(result).toMatchObject({
      fields: ['now()'],
    });
  });
});

// ============================================================================
// Migration safety tests: covering gaps found during comprehensive code audit
// These test specific parser-dependent behaviors that could silently break
// when migrating from @questdb/sql-ast-parser to @questdb/sql-parser
// ============================================================================

describe('getQueryOptionsFromSql: ref-to-ref comparison in WHERE', () => {
  it('handles WHERE col1 = col2 (ref on both sides of binary)', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE col1 = col2',
      mockDatasource
    );
    // getRefFilter stores the RHS ref as value: [e.name] (array, not string)
    expect(result).toMatchObject({
      filters: [
        {
          key: 'col1',
          operator: FilterOperator.Equals,
          value: ['col2'],
          type: 'string',
        },
      ],
    });
  });

  it('handles WHERE col1 != col2', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE col1 != col2',
      mockDatasource
    );
    expect(result).toMatchObject({
      filters: [
        {
          key: 'col1',
          operator: FilterOperator.NotEquals,
          value: ['col2'],
          type: 'string',
        },
      ],
    });
  });
});

describe('getQueryOptionsFromSql: cast on LHS of WHERE', () => {
  it('handles WHERE cast(col AS int) = value', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE cast(col AS int) = 1',
      mockDatasource
    );
    // getCastFilter with no key yet -> cast expression becomes the key
    expect(result).toMatchObject({
      filters: [
        {
          key: 'cast( col as int )',
          operator: FilterOperator.Equals,
          value: 1,
          type: 'int',
        },
      ],
    });
  });
});

describe('getQueryOptionsFromSql: FILL with numeric value', () => {
  it('handles FILL(0) numeric fill value', async () => {
    mockTimeField = 'ts';
    const result = await getQueryOptionsFromSql(
      'SELECT ts as time, count(*) FROM "t" WHERE $__timeFilter(ts) SAMPLE BY $__sampleByInterval FILL(0)',
      mockDatasource
    );
    // sampleByFill with type 'integer' uses f.value.toString()
    expect((result as any).sampleByFill).toEqual(['0']);
    mockTimeField = '';
  });

  it('handles FILL with mixed keyword and numeric', async () => {
    mockTimeField = 'ts';
    const result = await getQueryOptionsFromSql(
      'SELECT ts as time, count(*), sum(val) FROM "t" WHERE $__timeFilter(ts) SAMPLE BY $__sampleByInterval FILL(NONE, 0)',
      mockDatasource
    );
    expect((result as any).sampleByFill).toEqual(['NONE', '0']);
    mockTimeField = '';
  });
});

describe('getQueryOptionsFromSql: aggregation case sensitivity', () => {
  it('parser normalizes SUM to lowercase sum', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT SUM(field1) FROM "t"',
      mockDatasource
    );
    // old parser normalizes function names to lowercase
    // if new parser preserves case, this test will catch it
    expect(result).toMatchObject({
      metrics: [{ field: 'field1', aggregation: 'sum' }],
    });
  });

  it('parser normalizes COUNT to lowercase count', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT COUNT(*) FROM "t"',
      mockDatasource
    );
    expect(result).toMatchObject({
      metrics: [{ field: '*', aggregation: 'count' }],
    });
  });

  it('parser normalizes AVG to lowercase avg', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT AVG(price) FROM "t"',
      mockDatasource
    );
    expect(result).toMatchObject({
      metrics: [{ field: 'price', aggregation: 'avg' }],
    });
  });
});

describe('getQueryOptionsFromSql: > with plain numeric value', () => {
  it('handles WHERE col > 10 (integer)', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE col > 10',
      mockDatasource
    );
    expect(result).toMatchObject({
      filters: [
        {
          key: 'col',
          operator: FilterOperator.GreaterThan,
          value: 10,
          type: 'int',
        },
      ],
    });
  });

  it('handles WHERE col > 3.14 (numeric/float)', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE col > 3.14',
      mockDatasource
    );
    expect(result).toMatchObject({
      filters: [
        {
          key: 'col',
          operator: FilterOperator.GreaterThan,
          value: 3.14,
          type: 'double',
        },
      ],
    });
  });
});

describe('getQueryOptionsFromSql: IN with numeric list', () => {
  it('handles WHERE col IN (1, 2, 3) - integers in list', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT a FROM "t" WHERE col IN (1, 2, 3)',
      mockDatasource
    );
    // getListFilter casts all items to ExprString and reads .value
    // For ExprInteger items, .value is a number, so the list contains numbers
    expect(result).toMatchObject({
      filters: [
        {
          key: 'col',
          operator: FilterOperator.In,
          value: [1, 2, 3],
          type: 'string',
        },
      ],
    });
  });
});

describe('getQueryOptionsFromSql: star select', () => {
  it('handles SELECT * as a field named *', async () => {
    const result = await getQueryOptionsFromSql(
      'SELECT * FROM "t"',
      mockDatasource
    );
    // parser represents * as ExprRef with name '*'
    expect(result).toMatchObject({
      fields: ['*'],
    });
  });
});

describe('getQueryOptionsFromSql: string filter as value (not key)', () => {
  it('handles WHERE col = \'hello\' - string on RHS', async () => {
    const result = await getQueryOptionsFromSql(
      "SELECT a FROM \"t\" WHERE col = 'hello'",
      mockDatasource
    );
    expect(result).toMatchObject({
      filters: [
        {
          key: 'col',
          operator: FilterOperator.Equals,
          value: 'hello',
          type: 'string',
        },
      ],
    });
  });
});

function test(sql: string, builder: any, testQueryOptionsFromSql = true, timeField?: string) {
  return async () => {
    if (timeField) {
      mockTimeField = timeField;
    }
    expect(getSQLFromQueryOptions(builder, [])).toBe(sql);
    if (testQueryOptionsFromSql) {
      let options = await getQueryOptionsFromSql(sql, mockDatasource);
      expect(options).toEqual(builder);
    }
    mockTimeField = '';
  };
}
