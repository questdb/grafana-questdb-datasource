import {
    BuilderMetricFieldAggregation,
    BuilderMode,
    FilterOperator, FullField,
    OrderByDirection,
    SampleByAlignToMode
} from 'types';
import {getQueryOptionsFromSql, getSQLFromQueryOptions, isDateType, isNumberType, isTimestampType} from './utils';
import {Datasource} from "../../data/QuestDbDatasource";
import {PluginType} from "@grafana/data";

let mockTimeField = "";

const mockDatasource = new Datasource({
    id: 1,
    uid: 'questdb_ds',
    type: 'questdb-grafana-datasource',
    name: 'QuestDB',
    jsonData: {
        server: 'foo.com',
        port: 443,
        username: 'user'
    },
    readOnly: true,
    access: 'direct',
    meta: {
        id: 'questdb-grafana-datasource',
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


mockDatasource.fetchFields = async function(table: string): Promise<FullField[]> {
    if (mockTimeField.length > 0){
        return [{name:mockTimeField, label:mockTimeField, designated: true, type: "timestamp", picklistValues: []}];
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
  it( 'handles a table without a database', test( 'SELECT name FROM "tab"', {
    mode: BuilderMode.List,
    table: 'tab',
    fields: ['name'],
    timeField: "",
  }));

  it('handles a table with a dot', test( 'SELECT name FROM "foo.bar"', {
    mode: BuilderMode.List,
    table: 'foo.bar',
    fields: ['name'],
    timeField: "",
  }));

  it( 'handles 2 fields', test( 'SELECT field1, field2 FROM "tab"', {
    mode: BuilderMode.List,
    table: 'tab',
    fields: ['field1', 'field2'],
    timeField: "",
  }));

  it( 'handles a limit wih upper bound', test( 'SELECT field1, field2 FROM "tab" LIMIT 20', {
    mode: BuilderMode.List,
    table: 'tab',
    fields: ['field1', 'field2'],
    limit: '20',
    timeField: "",
  }));

  it( 'handles a limit with lower and upper bound', test( 'SELECT field1, field2 FROM "tab" LIMIT 10, 20', {
      mode: BuilderMode.List,
      table: 'tab',
      fields: ['field1', 'field2'],
      limit: '10, 20',
      timeField: "",
  }));

  it( 'handles empty orderBy array', test(
    'SELECT field1, field2 FROM "tab" LIMIT 20',
    {
      mode: BuilderMode.List,
      table: 'tab',
      fields: ['field1', 'field2'],
      orderBy: [],
      limit: 20,
      timeField: "",
    },
    false
  ));

  it( 'handles order by', test( 'SELECT field1, field2 FROM "tab" ORDER BY field1 ASC LIMIT 20', {
    mode: BuilderMode.List,
    table: 'tab',
    fields: ['field1', 'field2'],
    orderBy: [{ name: 'field1', dir: OrderByDirection.ASC }],
    limit: '20',
    timeField: "",
  }));

  it( 'handles no select', test(
    'SELECT  FROM "tab"',
    {
      mode: BuilderMode.Aggregate,
      table: 'tab',
      fields: [],
      metrics: [],
      timeField: "",
    },
    false
  ));

  it( 'does not escape * field', test(
    'SELECT * FROM "tab"',
    {
      mode: BuilderMode.Aggregate,
      table: 'tab',
      fields: ['*'],
      metrics: [],
      timeField: "",
    },
    false
  ));

  it( 'handles aggregation function', test( 'SELECT sum(field1) FROM "tab"', {
    mode: BuilderMode.Aggregate,
    table: 'tab',
    fields: [],
    metrics: [{ field: 'field1', aggregation: BuilderMetricFieldAggregation.Sum }],
    timeField: "",
  }));

  it( 'handles aggregation with alias', test( 'SELECT sum(field1) total_records FROM "tab"', {
    mode: BuilderMode.Aggregate,
    table: 'tab',
    fields: [],
    metrics: [{ field: 'field1', aggregation: BuilderMetricFieldAggregation.Sum, alias: 'total_records' }],
    timeField: "",
  }));

  it( 'handles 2 aggregations', test(
    'SELECT sum(field1) total_records, count(field2) total_records2 FROM "tab"',
    {
      mode: BuilderMode.Aggregate,
      table: 'tab',
      fields: [],
      metrics: [
        { field: 'field1', aggregation: BuilderMetricFieldAggregation.Sum, alias: 'total_records' },
        { field: 'field2', aggregation: BuilderMetricFieldAggregation.Count, alias: 'total_records2' },
      ],
      timeField: "",
    }
  ));

  it( 'handles aggregation with groupBy', test(
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
      timeField: "",
    },
    false
  ));

  it( 'handles aggregation with groupBy with fields having group by value', test(
    'SELECT field3, sum(field1) total_records, count(field2) total_records2 FROM "tab" GROUP BY field3',
    {
      mode: BuilderMode.Aggregate,
      table: 'tab',
      fields: ['field3'],
      metrics: [
        { field: 'field1', aggregation: BuilderMetricFieldAggregation.Sum, alias: 'total_records' },
        { field: 'field2', aggregation: BuilderMetricFieldAggregation.Count, alias: 'total_records2' },
      ],
      groupBy: ['field3'],
      timeField: "",
    }
  ));

  it( 'handles aggregation with group by and order by', test(
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
      timeField: "",
    },
    false
  ));

  it( 'handles aggregation with a IN filter', test(
    `SELECT count(id) FROM "tab" WHERE stagename IN ('Deal Won', 'Deal Lost' )`,
    {
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
      timeField: "",
    }
  ));

  it( 'handles aggregation with a NOT IN filter', test(
    `SELECT count(id) FROM "tab" WHERE stagename NOT IN ('Deal Won', 'Deal Lost' )`,
    {
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
      timeField: "",
    }
  ));

  it( 'handles $__fromTime and $__toTime filters', test(
    `SELECT id FROM "tab" WHERE tstmp > $__fromTime AND tstmp < $__toTime`,
    {
        mode: BuilderMode.List,
        table: 'tab',
        fields: ['id'],
        filters: [
            { key: 'tstmp', operator: '>', value: 'GRAFANA_START_TIME', type: 'timestamp', },
            { condition: 'AND', key: 'tstmp', operator: '<', value: 'GRAFANA_END_TIME', type: 'timestamp', },
        ],
        timeField: "",
    }, true
  ));

  it( 'handles aggregation with $__timeFilter', test(
    `SELECT count(id) FROM "tab" WHERE  $__timeFilter(createdon)`,
    {
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
      timeField: "",
    }
  ));

  it( 'handles aggregation with negated $__timeFilter', test(
      `SELECT count(id) FROM "tab" WHERE NOT (  $__timeFilter(closedate) )`,
      {
          mode: BuilderMode.Aggregate,
          table: 'tab',
          fields: [],
          metrics: [{field: 'id', aggregation: BuilderMetricFieldAggregation.Count,}],
          filters: [
              {
                  key: 'closedate',
                  operator: FilterOperator.OutsideGrafanaTimeRange,
                  type: 'timestamp',
              },
          ],
          timeField: "",
      }
  ));

  it( 'handles latest on one column ', test(
    'SELECT sym, value FROM "tab" LATEST ON tstmp PARTITION BY sym',
    {
        mode: BuilderMode.List,
        table: 'tab',
        fields: ['sym', 'value'],
        timeField: "tstmp",
        partitionBy: ['sym'],
        filters: [],
        },
    false
    ));

    it( 'handles latest on two columns ', test(
        'SELECT s1, s2, value FROM "tab" LATEST ON tstmp PARTITION BY s1, s2 ORDER BY time ASC',
        {
            mode: BuilderMode.List,
            table: 'tab',
            fields: ['s1', 's2', 'value'],
            timeField: "tstmp",
            partitionBy: ['s1', 's2'],
            filters: [],
            orderBy: [{name: "time", dir: "ASC"}]
        },
        false
    ));

  it( 'handles sample by align to calendar', test(
    'SELECT tstmp as time,  count(*), first(str) FROM "tab" WHERE   $__timeFilter(tstmp) SAMPLE BY $__sampleByInterval FILL ( null, 10 ) ALIGN TO CALENDAR',
    {
        mode: BuilderMode.Trend,
        table: 'tab',
        fields: ['tstmp'],
        sampleByAlignTo: SampleByAlignToMode.Calendar,
        sampleByFill: ["null", "10"],
        metrics: [ { field: '*', aggregation: BuilderMetricFieldAggregation.Count },
                    { field: 'str', aggregation: BuilderMetricFieldAggregation.First },
                  ],
        filters: [{
            key: 'tstmp',
            operator: FilterOperator.WithInGrafanaTimeRange,
            type: 'timestamp',
         },],
        timeField: "tstmp"
    },
    true, "tstmp"
  ));

  it( 'handles sample by align to calendar time zone', test(
        'SELECT tstmp as time,  count(*), first(str) FROM "tab" WHERE $__timeFilter(tstmp) SAMPLE BY $__sampleByInterval FILL ( null, 10 ) ALIGN TO CALENDAR TIME ZONE \'EST\'',
        {
            mode: BuilderMode.Trend,
            table: 'tab',
            fields: ['time'],
            sampleByAlignTo: SampleByAlignToMode.CalendarTimeZone,
            sampleByAlignToValue: "EST",
            sampleByFill: ["null", "10"],
            metrics: [ { field: '*', aggregation: BuilderMetricFieldAggregation.Count },
                { field: 'str', aggregation: BuilderMetricFieldAggregation.First },
            ],
            filters: [],
            timeField: "tstmp"
        },
        false
  ));

  it( 'handles sample by align to calendar offset', test(
    'SELECT tstmp as time,  count(*), first(str) FROM "tab" WHERE $__timeFilter(tstmp) SAMPLE BY $__sampleByInterval FILL ( null, 10 ) ALIGN TO CALENDAR WITH OFFSET \'01:00\'',
    {
        mode: BuilderMode.Trend,
        table: 'tab',
        fields: ['time'],
        sampleByAlignTo: SampleByAlignToMode.CalendarOffset,
        sampleByAlignToValue: "01:00",
        sampleByFill: ["null", "10"],
        metrics: [ { field: '*', aggregation: BuilderMetricFieldAggregation.Count },
            { field: 'str', aggregation: BuilderMetricFieldAggregation.First },
        ],
        filters: [],
        timeField: "tstmp"
    },
    false
  ));

  it( 'handles sample by align to first observation', test(
        'SELECT tstmp as time,  count(*), first(str) FROM "tab" WHERE $__timeFilter(tstmp) SAMPLE BY $__sampleByInterval FILL ( null, 10 ) ALIGN TO FIRST OBSERVATION',
        {
            mode: BuilderMode.Trend,
            table: 'tab',
            fields: ['time'],
            sampleByAlignTo: SampleByAlignToMode.FirstObservation,
            sampleByFill: ["null", "10"],
            metrics: [ { field: '*', aggregation: BuilderMetricFieldAggregation.Count },
                { field: 'str', aggregation: BuilderMetricFieldAggregation.First },
            ],
            filters: [],
            timeField: "tstmp"
        },
        false
  ));

  it( 'handles __timeFilter macro and sample by', test(
    'SELECT time as time FROM "tab" WHERE $__timeFilter(time) SAMPLE BY $__sampleByInterval ORDER BY time ASC',
    {
      mode: BuilderMode.Trend,
      table: 'tab',
      fields: [],
      timeField: 'time',
      metrics: [],
      filters: [],
      orderBy: [{name: "time", dir: "ASC"}]
    },
    false
  ));

  it( 'handles __timeFilter macro and sample by with filters', test(
    'SELECT time as time FROM "tab" WHERE   $__timeFilter(time) AND base IS NOT NULL AND time IS NOT NULL SAMPLE BY $__sampleByInterval',
    {
      mode: BuilderMode.Trend,
      table: 'tab',
      fields: ['time'],
      timeField: 'time',
      filters: [
        { key: 'time', operator: FilterOperator.WithInGrafanaTimeRange, type: 'timestamp',},
        { condition: 'AND', key: 'base', operator: 'IS NOT NULL'},
        { condition: 'AND', key: 'time', operator: 'IS NOT NULL', type: 'timestamp'},
      ],
    },
    true, "time"
  ));

  it( 'handles function filter', test(
  'SELECT tstmp FROM "tab" WHERE tstmp > dateadd(\'M\', -1, now())',
    {
        mode: BuilderMode.List,
        table: 'tab',
        fields: ["tstmp"],
        timeField: 'tstmp',
        filters: [
            {
                key: 'tstmp',
                operator: '>',
                type: 'timestamp',
                value: 'dateadd(\'M\', -1, now())'
            },
        ],
    },
    true, "tstmp"
  ));

  it( 'handles multiple function filters', test(
    'SELECT tstmp FROM "tab" WHERE tstmp > dateadd(\'M\', -1, now()) AND tstmp = dateadd(\'M\', -1, now())',
    {
        mode: BuilderMode.List,
        table: 'tab',
        fields: ["tstmp"],
        timeField: 'tstmp',
        filters: [
            { key: 'tstmp', operator: '>', type: 'timestamp', value: 'dateadd(\'M\', -1, now())' },
            { condition: 'AND', key: 'tstmp', operator: '=', type: 'timestamp', value: 'dateadd(\'M\', -1, now())' },
        ],
    },
    true, "tstmp"
  ));

  it( 'handles boolean column ref filters', test(
    'SELECT tstmp, bool FROM "tab" WHERE bool = true AND tstmp > cast( \'2020-01-01\' as timestamp )',
    {
        mode: BuilderMode.List,
        table: 'tab',
        fields: ['tstmp', 'bool'],
        timeField: 'tstmp',
        filters: [
            { key: 'bool', operator: '=', type: 'boolean', value: true },
            { condition: 'AND', key: 'tstmp', operator: '>', type: 'timestamp', value: 'cast( \'2020-01-01\' as timestamp )' },
        ],
    },
    true, "tstmp"
  ));

  it( 'handles numeric filters', test(
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
    true, "tstmp"
  ));

  // builder doesn't support nested conditions, so we flatten them
  it( 'flattens condition hierarchy', async () => {
      let options = await getQueryOptionsFromSql('SELECT tstmp, z FROM "tab" WHERE k = 1 AND ( j > 1.2 OR p = \'start\' )', mockDatasource);
      expect( options).toEqual(    {
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

  it( 'handles expressions in select list', async () => {
    let options = await getQueryOptionsFromSql('SELECT tstmp, e::timestamp, f(x), g(a,b) FROM "tab"', mockDatasource);
    expect( options).toEqual(    {
        mode: BuilderMode.List,
        table: 'tab',
        fields: ['tstmp', 'cast(e  as timestamp)', 'f(x)', 'g(a, b)'],
        timeField: '',
    });
  });
});

function test(sql: string, builder: any, testQueryOptionsFromSql = true, timeField?: string) {
    return async () => {
        if (timeField){
            mockTimeField = timeField;
        }
        expect(getSQLFromQueryOptions(builder)).toBe(sql);
        if (testQueryOptionsFromSql) {
            let options = await getQueryOptionsFromSql(sql, mockDatasource);
            expect( options).toEqual(builder);
        }
        mockTimeField = "";
    }
}
