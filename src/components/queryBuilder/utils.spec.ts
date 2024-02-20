import {BuilderMetricFieldAggregation, BuilderMode, FilterOperator, OrderByDirection} from 'types';
import {getQueryOptionsFromSql, getSQLFromQueryOptions, isDateType, isNumberType, isTimestampType} from './utils';

describe('isDateType', () => {
  it('returns true for Date type', () => {
    expect(isDateType('Date')).toBe(true);
    expect(isDateType('date')).toBe(true);
    expect(isDateType('Timestamp')).toBe(true);
    expect(isDateType('timestamp')).toBe(true);
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
  testCondition('handles a table without a database', 'SELECT "name" FROM "foo"', {
    mode: BuilderMode.List,
    table: 'foo',
    fields: ['name'],
  });

  testCondition('handles a table with a dot', 'SELECT "name" FROM "foo.bar"', {
    mode: BuilderMode.List,
    table: 'foo.bar',
    fields: ['name'],
  });

  testCondition('handles 2 fields', 'SELECT "field1", "field2" FROM "foo"', {
    mode: BuilderMode.List,
    table: 'foo',
    fields: ['field1', 'field2'],
  });

  testCondition('handles a limit wih upper bound', 'SELECT "field1", "field2" FROM "foo" LIMIT 20', {
    mode: BuilderMode.List,
    table: 'foo',
    fields: ['field1', 'field2'],
    limit: '20',
  });

  testCondition('handles a limit with lower and upper bound', 'SELECT "field1", "field2" FROM "foo" LIMIT 10, 20', {
      mode: BuilderMode.List,
      table: 'foo',
      fields: ['field1', 'field2'],
      limit: '10, 20',
  });

  testCondition( 'handles empty orderBy array',
    'SELECT "field1", "field2" FROM "foo" LIMIT 20',
    {
      mode: BuilderMode.List,
      table: 'foo',
      fields: ['field1', 'field2'],
      orderBy: [],
      limit: 20,
    },
    false
  );

  testCondition('handles order by', 'SELECT "field1", "field2" FROM "foo" ORDER BY field1 ASC LIMIT 20', {
    mode: BuilderMode.List,
    table: 'foo',
    fields: ['field1', 'field2'],
    orderBy: [{ name: 'field1', dir: OrderByDirection.ASC }],
    limit: '20',
  });

  testCondition( 'handles no select',
    'SELECT  FROM "tab"',
    {
      mode: BuilderMode.Aggregate,
      table: 'tab',
      fields: [],
      metrics: [],
    },
    false
  );

  testCondition( 'does not escape * field',
    'SELECT * FROM "tab"',
    {
      mode: BuilderMode.Aggregate,
      table: 'tab',
      fields: ['*'],
      metrics: [],
    },
    false
  );

  testCondition('handles aggregation function', 'SELECT sum(field1) FROM "foo"', {
    mode: BuilderMode.Aggregate,
    table: 'foo',
    fields: [],
    metrics: [{ field: 'field1', aggregation: BuilderMetricFieldAggregation.Sum }],
  });

  testCondition('handles aggregation with alias', 'SELECT sum(field1) total_records FROM "foo"', {
    mode: BuilderMode.Aggregate,
    table: 'foo',
    fields: [],
    metrics: [{ field: 'field1', aggregation: BuilderMetricFieldAggregation.Sum, alias: 'total_records' }],
  });

  testCondition(
    'handles 2 aggregations',
    'SELECT sum(field1) total_records, count(field2) total_records2 FROM "foo"',
    {
      mode: BuilderMode.Aggregate,
      table: 'foo',
      fields: [],
      metrics: [
        { field: 'field1', aggregation: BuilderMetricFieldAggregation.Sum, alias: 'total_records' },
        { field: 'field2', aggregation: BuilderMetricFieldAggregation.Count, alias: 'total_records2' },
      ],
    }
  );

  testCondition(
    'handles aggregation with groupBy',
    'SELECT field3, sum(field1) total_records, count(field2) total_records2 FROM "foo" GROUP BY field3',
    {
      mode: BuilderMode.Aggregate,
      table: 'foo',
      database: 'db',
      fields: [],
      metrics: [
        { field: 'field1', aggregation: BuilderMetricFieldAggregation.Sum, alias: 'total_records' },
        { field: 'field2', aggregation: BuilderMetricFieldAggregation.Count, alias: 'total_records2' },
      ],
      groupBy: ['field3'],
    },
    false
  );

  testCondition(
    'handles aggregation with groupBy with fields having group by value',
    'SELECT field3, sum(field1) total_records, count(field2) total_records2 FROM "foo" GROUP BY field3',
    {
      mode: BuilderMode.Aggregate,
      table: 'foo',
      fields: ['field3'],
      metrics: [
        { field: 'field1', aggregation: BuilderMetricFieldAggregation.Sum, alias: 'total_records' },
        { field: 'field2', aggregation: BuilderMetricFieldAggregation.Count, alias: 'total_records2' },
      ],
      groupBy: ['field3'],
    }
  );

  testCondition(
    'handles aggregation with group by and order by',
    'SELECT StageName, Type, count(Id) count_of, sum(Amount) FROM "foo" GROUP BY StageName, Type ORDER BY count(Id) DESC, StageName ASC',
    {
      mode: BuilderMode.Aggregate,
      table: 'foo',
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
    },
    false
  );

  testCondition(
    'handles aggregation with a IN filter',
    `SELECT count(id) FROM "foo" WHERE   ( stagename IN ('Deal Won', 'Deal Lost' ) )`,
    {
      mode: BuilderMode.Aggregate,
      table: 'foo',
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
    }
  );

  testCondition(
    'handles aggregation with a NOT IN filter',
    `SELECT count(id) FROM "foo" WHERE   ( stagename NOT IN ('Deal Won', 'Deal Lost' ) )`,
    {
      mode: BuilderMode.Aggregate,
      table: 'foo',
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
    }
  );

  testCondition(
    'handles aggregation with timestamp filter',
    `SELECT count(id) FROM "foo" WHERE   ( createdon  >= $__fromTime AND createdon <= $__toTime )`,
    {
      mode: BuilderMode.Aggregate,
      table: 'foo',
      fields: [],
      metrics: [{ field: 'id', aggregation: BuilderMetricFieldAggregation.Count }],
      filters: [
        {
          key: 'createdon',
          operator: FilterOperator.WithInGrafanaTimeRange,
          type: 'timestamp',
        },
      ],
    }
  );

  testCondition(
    'handles aggregation with date filter',
    `SELECT count(id) FROM "foo" WHERE   (  NOT ( closedate  >= $__fromTime AND closedate <= $__toTime ) )`,
    {
      mode: BuilderMode.Aggregate,
      table: 'foo',
      fields: [],
      metrics: [{ field: 'id', aggregation: BuilderMetricFieldAggregation.Count }],
      filters: [
        {
          key: 'closedate',
          operator: FilterOperator.OutsideGrafanaTimeRange,
          type: 'timestamp',
        },
      ],
    }
  );

  testCondition(
    'handles timeseries function',
    'SELECT time as time FROM "foo" WHERE $__timeFilter(time) SAMPLE BY $__sampleByInterval ORDER BY time ASC',
    {
      mode: BuilderMode.Trend,
      table: 'foo',
      fields: [],
      timeField: 'time',
      metrics: [],
      filters: [],
      orderBy: [{name: "time", dir: "ASC"}]
    },
    false
  );

  testCondition(
    'handles timeseries function with a filter',
    'SELECT time as time FROM "foo" WHERE $__timeFilter(time) AND   ( base IS NOT NULL ) SAMPLE BY $__sampleByInterval',
    {
      mode: BuilderMode.Trend,
      table: 'foo',
      fields: [],
      timeField: 'time',
      metrics: [],
      filters: [
        {
          condition: 'AND',
          filterType: 'custom',
          key: 'base',
          operator: 'IS NOT NULL',
          type: 'string',
        },
      ],
    },
    false
  );
});

function testCondition(name: string, sql: string, builder: any, testQueryOptionsFromSql = true) {
  it(name, () => {
    expect(getSQLFromQueryOptions(builder)).toBe(sql);
    if (testQueryOptionsFromSql) {
      expect(getQueryOptionsFromSql(sql)).toEqual(builder);
    }
  });
}
