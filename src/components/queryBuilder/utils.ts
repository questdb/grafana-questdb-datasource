import { VariableWithMultiSupport } from '@grafana/data';
import {
  astVisitor,
  Expr,
  ExprBinary,
  ExprBool,
  ExprCall,
  ExprCast,
  ExprInteger,
  ExprList,
  ExprNumeric,
  ExprRef,
  ExprString,
  ExprUnary,
  FromTable,
  IAstVisitor,
  SelectedColumn,
} from '@questdb/sql-ast-parser';
import {
  BooleanFilter,
  BuilderMetricField,
  BuilderMetricFieldAggregation,
  BuilderMode,
  DateFilter,
  DateFilterWithoutValue,
  Filter,
  FilterOperator,
  MultiFilter,
  NullFilter,
  NumberFilter,
  OrderBy,
  SampleByAlignToMode,
  SqlBuilderOptions,
  SqlBuilderOptionsAggregate,
  SqlBuilderOptionsList,
  SqlBuilderOptionsTrend,
} from 'types';
import { sqlToStatement } from 'data/ast';
import { Datasource } from '../../data/QuestDbDatasource';
import { isString } from 'lodash';

export const isBooleanType = (type: string): boolean => {
  return ['boolean'].includes(type?.toLowerCase());
};

export const isGeoHashType = (type: string): boolean => {
  return type?.toLowerCase().startsWith('geohash');
};

export const isNumberType = (type: string): boolean => {
  return ['byte', 'short', 'int', 'long', 'float', 'double'].includes(type?.toLowerCase());
};

export const isNullableNumberType = (type: string): boolean => {
  return ['int', 'long', 'float', 'double'].includes(type?.toLowerCase());
};

export const isDateType = (type: string): boolean => {
  return ['date', 'timestamp'].includes(type?.toLowerCase());
};
export const isTimestampType = (type: string): boolean => {
  return ['timestamp'].includes(type?.toLowerCase());
};

export const isIPv4Type = (type: string): boolean => {
  return 'ipv4' === type?.toLowerCase();
};

export const isStringType = (type: string): boolean => {
  return ['string', 'symbol', 'char'].includes(type?.toLowerCase());
};

export const isNullFilter = (filter: Filter): filter is NullFilter => {
  return [FilterOperator.IsNull, FilterOperator.IsNotNull].includes(filter.operator);
};
export const isBooleanFilter = (filter: Filter): filter is BooleanFilter => {
  return isBooleanType(filter.type);
};
export const isNumberFilter = (filter: Filter): filter is NumberFilter => {
  return isNumberType(filter.type);
};
export const isDateFilterWithOutValue = (filter: Filter): filter is DateFilterWithoutValue => {
  return (
    isDateType(filter.type) &&
    [FilterOperator.WithInGrafanaTimeRange, FilterOperator.OutsideGrafanaTimeRange].includes(filter.operator)
  );
};
export const isDateFilter = (filter: Filter): filter is DateFilter => {
  return isDateType(filter.type);
};

export const isMultiFilter = (filter: Filter): filter is MultiFilter => {
  return FilterOperator.In === filter.operator || FilterOperator.NotIn === filter.operator;
};

export const isSetFilter = (filter: Filter): filter is MultiFilter => {
  return [FilterOperator.In, FilterOperator.NotIn].includes(filter.operator);
};

const getListQuery = (table = '', fields: string[] = []): string => {
  fields = fields && fields.length > 0 ? fields : [''];
  return `SELECT ${fields.join(', ')} FROM ${escaped(table)}`;
};

const getLatestOn = (timeField = '', partitionBy: string[] = []): string => {
  if (timeField.length === 0 || partitionBy.length === 0) {
    return '';
  }

  return ` LATEST ON ${timeField} PARTITION BY ${partitionBy.join(', ')}`;
};

const getAggregationQuery = (
  table = '',
  fields: string[] = [],
  metrics: BuilderMetricField[] = [],
  groupBy: string[] = []
): string => {
  let selected = fields.length > 0 ? fields.join(', ') : '';
  let metricsQuery = metrics
    .map((m) => {
      const alias = m.alias ? ` ${m.alias.replace(/ /g, '_')}` : '';
      return `${m.aggregation}(${m.field})${alias}`;
    })
    .join(', ');
  const groupByQuery = groupBy
    .filter((x) => !fields.some((y) => y === x)) // not adding field if its already is selected
    .join(', ');
  return `SELECT ${selected}${selected && (groupByQuery || metricsQuery) ? ', ' : ''}${groupByQuery}${
    metricsQuery && groupByQuery ? ', ' : ''
  }${metricsQuery} FROM ${escaped(table)}`;
};

const getSampleByQuery = (
  table = '',
  fields: string[] = [],
  metrics: BuilderMetricField[] = [],
  groupBy: string[] = [],
  timeField = ''
): string => {
  metrics = metrics && metrics.length > 0 ? metrics : [];

  let metricsQuery = metrics
    .map((m) => {
      const alias = m.alias ? ` ` + m.alias.replace(/ /g, '_') : '';
      return `${m.aggregation}(${m.field})${alias}`;
    })
    .join(', ');
  const time = `${timeField} as time`;

  if (metricsQuery !== '') {
    const group = groupBy.length > 0 ? `${groupBy.join(', ')},` : '';
    metricsQuery = `${time}, ${group} ${metricsQuery}`;
  } else if (groupBy.length > 0) {
    metricsQuery = `${time}, ${groupBy.join(', ')}`;
  } else {
    metricsQuery = `${time}`;
  }

  return `SELECT ${metricsQuery} FROM ${escaped(table)}`;
};

const getFilters = (
  filters: Filter[],
  templateVars: VariableWithMultiSupport[]
): { filters: string; hasTimeFilter: boolean } => {
  let hasTsFilter = false;

  let combinedFilters = filters.reduce((previousValue, currentFilter, currentIndex) => {
    const prefixCondition = currentIndex === 0 ? '' : currentFilter.condition;
    let filter;
    let field = currentFilter.key;
    let operator;
    let notOperator = false;

    if (currentFilter.operator === FilterOperator.NotLike) {
      operator = 'LIKE';
      notOperator = true;
    } else if (currentFilter.operator === FilterOperator.OutsideGrafanaTimeRange) {
      operator = '';
      notOperator = true;
      field = ` \$__timeFilter(${currentFilter.key})`;
      hasTsFilter = true;
    } else if (FilterOperator.WithInGrafanaTimeRange === currentFilter.operator) {
      operator = '';
      field = ` \$__timeFilter(${currentFilter.key})`;
      hasTsFilter = true;
    } else {
      operator = currentFilter.operator;
    }
    if (operator.length > 0) {
      filter = ` ${field} ${operator}`;
    } else {
      filter = ` ${field}`;
    }

    if (isNullFilter(currentFilter)) {
      // don't add anything
    } else if (isMultiFilter(currentFilter)) {
      let values = currentFilter.value;
      if (isNumberType(currentFilter.type)) {
        filter += ` (${values?.map((v) => v.trim()).join(', ')} )`;
      } else {
        filter += ` (${values
          ?.map((v) =>
            formatStringValue(
              v,
              templateVars,
              currentFilter.operator === FilterOperator.In || currentFilter.operator === FilterOperator.NotIn
            ).trim()
          )
          .join(', ')} )`;
      }
    } else if (isBooleanFilter(currentFilter)) {
      filter += ` ${currentFilter.value}`;
    } else if (isNumberFilter(currentFilter)) {
      filter += ` ${currentFilter.value || '0'}`;
    } else if (isDateFilter(currentFilter)) {
      if (!isDateFilterWithOutValue(currentFilter)) {
        switch (currentFilter.value) {
          case 'GRAFANA_START_TIME':
            filter += ` \$__fromTime`;
            break;
          case 'GRAFANA_END_TIME':
            filter += ` \$__toTime`;
            break;
          default:
            filter += ` ${currentFilter.value || 'TODAY'}`;
        }
      }
    } else {
      filter += formatStringValue(currentFilter.value || '', templateVars);
    }

    if (notOperator) {
      filter = ` NOT (${filter} )`;
    }

    if (!filter) {
      return previousValue;
    }

    if (previousValue.length > 0) {
      return `${previousValue} ${prefixCondition}${filter}`;
    } else {
      return filter;
    }
  }, '');

  return { filters: removeQuotesForMultiVariables(combinedFilters, templateVars), hasTimeFilter: hasTsFilter };
};

const getSampleBy = (sampleByMode: SampleByAlignToMode, sampleByValue?: string, sampleByFill?: string[]): string => {
  let fills = '';
  if (sampleByFill !== undefined && sampleByFill.length > 0) {
    // remove suffixes
    fills = ` FILL ( ${sampleByFill.map((s) => s.replace(/_[0-9]+$/, '')).join(', ')} )`;
  }
  let mode = '';
  if (sampleByMode !== undefined) {
    mode = ` ALIGN TO ${sampleByMode}`;
  }
  let offsetOrTz = '';
  if (
    (sampleByMode === SampleByAlignToMode.CalendarOffset || sampleByMode === SampleByAlignToMode.CalendarTimeZone) &&
    sampleByValue !== undefined &&
    sampleByValue.length > 0
  ) {
    offsetOrTz = ` '${sampleByValue}'`;
  }

  return ` SAMPLE BY \$__sampleByInterval${fills}${mode}${offsetOrTz}`;
};

const getGroupBy = (groupBy: string[] = [], timeField?: string): string => {
  const clause = groupBy.length > 0 ? ` GROUP BY ${groupBy.join(', ')}` : '';
  if (groupBy.length === 0) {
    return '';
  }
  if (timeField === undefined) {
    return clause;
  }
  return `${clause}, ${timeField}`;
};

const getOrderBy = (orderBy?: OrderBy[], prefix = true): string => {
  const pfx = prefix ? ' ORDER BY ' : '';
  return orderBy && orderBy.filter((o) => o.name).length > 0
    ? pfx +
        orderBy
          .filter((o) => o.name)
          .map((o) => {
            return `${o.name} ${o.dir}`;
          })
          .join(', ')
    : '';
};

const getLimit = (limit?: string): string => {
  return ` LIMIT ` + (limit || '100');
};

const escapeFields = (fields: string[]): string[] => {
  return fields.map((f) => {
    return f.match(/(^\d|[^a-zA-Z_])/im) ? `"${f}"` : f;
  });
};

export const getSQLFromQueryOptions = (
  options: SqlBuilderOptions,
  templateVars: VariableWithMultiSupport[]
): string => {
  const limit = options.limit ? getLimit(options.limit) : '';
  const fields = escapeFields(options.fields || []);
  let query = ``;
  switch (options.mode) {
    case BuilderMode.Aggregate:
      query += getAggregationQuery(options.table, fields, options.metrics, options.groupBy);
      const aggregateFilters = getFilters(options.filters || [], templateVars);
      if (aggregateFilters.filters) {
        query += ` WHERE${aggregateFilters.filters}`;
      }
      query += getGroupBy(options.groupBy);
      break;
    case BuilderMode.Trend:
      query += getSampleByQuery(options.table, fields, options.metrics, options.groupBy, options.timeField);
      const sampleByFilters = getFilters(options.filters || [], templateVars);
      if (options.timeField || sampleByFilters.filters.length > 0) {
        query += ' WHERE';

        if (options.timeField && !sampleByFilters.hasTimeFilter) {
          query += ` $__timeFilter(${options.timeField})`;
          if (sampleByFilters.filters.length > 0) {
            query += ' AND';
          }
        }
        if (sampleByFilters.filters.length > 0) {
          query += ` ${sampleByFilters.filters}`;
        }
      }
      query += getSampleBy(options.sampleByAlignTo, options.sampleByAlignToValue, options.sampleByFill);
      break;
    case BuilderMode.List:
    default:
      query += getListQuery(options.table, fields);
      const filters = getFilters(options.filters || [], templateVars);
      if (filters.filters) {
        query += ` WHERE${filters.filters}`;
      }
      query += getLatestOn(options.timeField, options.partitionBy);
  }

  query += getOrderBy(options.orderBy);
  query += limit;
  return query;
};

export async function getQueryOptionsFromSql(
  sql: string,
  datasource?: Datasource
): Promise<SqlBuilderOptions | string> {
  const ast = sqlToStatement(sql);
  if (!ast || ast.type !== 'select') {
    return "The query can't be parsed.";
  }
  if (!ast.from || ast.from.length !== 1) {
    return `The query has too many 'FROM' clauses.`;
  }
  if (ast.from[0].type !== 'table') {
    return `The 'FROM' clause is not a table.`;
  }
  const fromTable = ast.from[0] as FromTable;

  let timeField;
  let fieldsToTypes = new Map<string, string>();

  if (fromTable?.name?.name.length > 0 && datasource) {
    const dbFields = await datasource.fetchFields(fromTable?.name?.name);
    dbFields.forEach((f) => {
      fieldsToTypes.set(f.name, f.type);
    });
    timeField = dbFields.find((f) => f.designated)?.name;
  }

  if (timeField === undefined) {
    timeField = '';
  }

  const fieldsAndMetrics = getMetricsFromAst(ast.columns ? ast.columns : null);

  let builder = {
    mode: BuilderMode.List,
    table: fromTable.name.name,
    timeField: timeField,
  } as SqlBuilderOptions;

  if (fieldsAndMetrics.fields) {
    builder.fields = fieldsAndMetrics.fields;
  }

  if (fieldsAndMetrics.metrics.length > 0) {
    builder.mode = BuilderMode.Aggregate;
    (builder as SqlBuilderOptionsAggregate).metrics = fieldsAndMetrics.metrics;
  }

  if (ast.where) {
    builder.filters = getFiltersFromAst(ast.where, fieldsToTypes);
  }

  const orderBy = ast.orderBy
    ?.map<OrderBy>((ob) => {
      if (ob.by.type !== 'ref' || ob.by.name === 'time') {
        return {} as OrderBy;
      }
      return { name: ob.by.name, dir: ob.order } as OrderBy;
    })
    .filter((x) => x.name);

  if (orderBy && orderBy.length > 0) {
    (builder as SqlBuilderOptionsAggregate).orderBy = orderBy!;
  }

  builder.limit = undefined;
  if (ast.limit) {
    if (ast.limit.upperBound && ast.limit.upperBound.type === 'integer') {
      if (ast.limit.lowerBound && ast.limit.lowerBound.type === 'integer') {
        builder.limit = `${ast.limit.lowerBound.value}, ${ast.limit.upperBound.value}`;
      } else {
        builder.limit = `${ast.limit.upperBound.value}`;
      }
    }
  }

  if (ast.sampleBy) {
    builder.mode = BuilderMode.Trend;
    if (ast.sampleByAlignTo) {
      (builder as SqlBuilderOptionsTrend).sampleByAlignTo = ast.sampleByAlignTo.alignTo as SampleByAlignToMode;
    }
    if (ast.sampleByFill) {
      (builder as SqlBuilderOptionsTrend).sampleByFill = ast.sampleByFill.map((f) => {
        if (f.type === 'sampleByKeyword') {
          return f.keyword;
        } else if (f.type === 'null') {
          return 'null';
        } else {
          return f.value.toString();
        }
      });
    }
    if (ast.sampleByAlignTo?.alignValue) {
      (builder as SqlBuilderOptionsTrend).sampleByAlignToValue = ast.sampleByAlignTo?.alignValue;
    }
  }

  if (ast.latestOn) {
    builder.mode = BuilderMode.List;
    if (ast.partitionBy) {
      (builder as SqlBuilderOptionsList).partitionBy = ast.partitionBy.map((p) => {
        if (p.table) {
          return p.table.name + '.' + p.name;
        } else {
          return p.name;
        }
      });
    }
  }

  const groupBy = ast.groupBy
    ?.map((gb) => {
      if (gb.type !== 'ref' || gb.name === 'time') {
        return '';
      }
      return gb.name;
    })
    .filter((x) => x !== '');

  if (groupBy && groupBy.length > 0) {
    (builder as SqlBuilderOptionsAggregate).groupBy = groupBy;
  }
  return builder;
}

type MapperState = {
  currentFilter: Filter | null;
  filters: Filter[];
  notFlag: boolean;
  condition: 'AND' | 'OR' | null;
};

function getFiltersFromAst(expr: Expr, fieldsToTypes: Map<string, string>): Filter[] {
  let state: MapperState = { currentFilter: null, filters: [], notFlag: false, condition: null } as MapperState;

  const visitor = astVisitor((mapper) => ({
    expr: (e) => {
      switch (e?.type) {
        case 'binary':
          getBinaryFilter(mapper, e, state);
          break;
        case 'boolean':
          getBooleanFilter(e, state);
          break;
        case 'call':
          getCallFilter(e, state);
          break;
        case 'cast':
          getCastFilter(e, state);
          break;
        case 'integer':
          getIntFilter(e, state);
          break;
        case 'list':
          getListFilter(e, state);
          break;
        case 'numeric':
          getNumericFilter(e, state);
          break;
        case 'ref':
          getRefFilter(e, state, fieldsToTypes);
          break;
        case 'string':
          getStringFilter(e, state);
          break;
        case 'unary':
          getUnaryFilter(mapper, e, state);
          break;
        default:
          console.error(`${e?.type} is not supported. This is likely a bug.`);
          break;
      }
    },
  }));

  try {
    // don't break conversion
    visitor.expr(expr);
  } catch (error) {
    console.error(error);
  }

  return state.filters;
}

function getRefFilter(e: ExprRef, state: MapperState, fieldsToTypes: Map<string, string>) {
  let doAdd = false;
  if (state.currentFilter === null) {
    state.currentFilter = {} as Filter;
    doAdd = true;
  }

  if (e.name?.toLowerCase() === '$__fromtime') {
    state.currentFilter = { ...state.currentFilter, value: 'GRAFANA_START_TIME', type: 'timestamp' } as Filter;
    return;
  }

  if (e.name?.toLowerCase() === '$__totime') {
    state.currentFilter = { ...state.currentFilter, value: 'GRAFANA_END_TIME', type: 'timestamp' } as Filter;
    return;
  }

  let type = fieldsToTypes.get(e.name);
  if (!state.currentFilter.key) {
    state.currentFilter = { ...state.currentFilter, key: e.name };
    if (type) {
      state.currentFilter.type = type;
    }
  } else {
    state.currentFilter = { ...state.currentFilter, value: [e.name], type: type || 'string' } as Filter;
  }

  if (doAdd) {
    state.filters.push(state.currentFilter);
    state.currentFilter = null;
  }
}

function getListFilter(e: ExprList, state: MapperState) {
  state.currentFilter = {
    ...state.currentFilter,
    value: e.expressions.map((x) => {
      const k = x as ExprString;
      return k.value;
    }),
    type: 'string',
  } as Filter;
}

function getCallString(e: ExprCall) {
  let args: string = e.args
    .map((x) => {
      switch (x.type) {
        case 'string':
          return `'${x.value}'`;
        case 'boolean':
        case 'numeric':
        case 'integer':
          return x.value;
        case 'ref':
          return x.name;
        case 'null':
          return 'null';
        case 'call':
          return getCallString(x);
        default:
          return '';
      }
    })
    .join(', ');

  return `${e.function.name}(${args})`;
}

function toString(x: Expr) {
  switch (x.type) {
    case 'string':
      return `'${x.value}'`;
    case 'boolean':
    case 'numeric':
    case 'integer':
      return x.value;
    case 'ref':
      return x.name;
    case 'null':
      return 'null';
    case 'call':
      return getCallString(x);
    default:
      return '';
  }
}

function getCallFilter(e: ExprCall, state: MapperState) {
  let doAdd = false;
  if (!state.currentFilter) {
    // map f(x) to true = f(x) so it can be displayed in builder
    state.currentFilter = { key: 'true', type: 'boolean' } as Filter;
    doAdd = true;
  }

  let args = e.args
    .map((x) => {
      return toString(x);
    })
    .join(', ');
  const val = `${e.function.name}(${args})`;

  if (val.startsWith('$__timefilter(')) {
    state.currentFilter = {
      ...state.currentFilter,
      key: (e.args[0] as ExprRef).name,
      operator: state.notFlag ? FilterOperator.OutsideGrafanaTimeRange : FilterOperator.WithInGrafanaTimeRange,
      type: 'timestamp',
    } as Filter;
  } else {
    state.currentFilter = { ...state.currentFilter, value: val } as Filter;
  }

  if (doAdd) {
    if (state.condition) {
      state.currentFilter.condition = state.condition;
      state.condition = null;
    }

    state.filters.push(state.currentFilter);
    state.currentFilter = null;
  }
}

function getUnaryFilter(mapper: IAstVisitor, e: ExprUnary, state: MapperState) {
  if (e.op === 'NOT') {
    state.notFlag = true;
    mapper.super().expr(e);
    state.notFlag = false;
    return;
  }

  state.currentFilter = { operator: e.op as FilterOperator } as Filter;
  if (state.condition) {
    state.currentFilter.condition = state.condition;
    state.condition = null;
  }
  mapper.super().expr(e);
  state.filters.push(state.currentFilter);
  state.currentFilter = null;
}

function getStringFilter(e: ExprString, state: MapperState) {
  if (state.currentFilter != null && !state.currentFilter.key) {
    state.currentFilter = { ...state.currentFilter, key: e.value } as Filter;
    return;
  }
  state.currentFilter = {
    ...state.currentFilter,
    value: e.value,
    type: state.currentFilter?.type || 'string',
  } as Filter;
}

function getNumericFilter(e: ExprNumeric, state: MapperState) {
  if (state.currentFilter != null && !state.currentFilter.key) {
    state.currentFilter = { ...state.currentFilter, key: e.value.toString() } as Filter;
    return;
  }
  state.currentFilter = { ...state.currentFilter, value: e.value, type: 'double' } as Filter;
}

function getIntFilter(e: ExprInteger, state: MapperState) {
  if (state.currentFilter != null && !state.currentFilter.key) {
    state.currentFilter = { ...state.currentFilter, key: e.value.toString() } as Filter;
    return;
  }
  state.currentFilter = { ...state.currentFilter, value: e.value, type: 'int' } as Filter;
}

function getCastFilter(e: ExprCast, state: MapperState) {
  let val = `cast( ${toString(e.operand)} as ${e.to.kind === undefined ? e.to.name : ''} )`;

  if (state.currentFilter != null && !state.currentFilter.key) {
    state.currentFilter = { ...state.currentFilter, key: val } as Filter;
    return;
  } else {
    state.currentFilter = { ...state.currentFilter, value: val, type: state.currentFilter?.type || 'int' } as Filter;
  }
}

function getBooleanFilter(e: ExprBool, state: MapperState) {
  state.currentFilter = { ...state.currentFilter, value: e.value, type: 'boolean' } as Filter;
}

function getBinaryFilter(mapper: IAstVisitor, e: ExprBinary, state: MapperState) {
  if (e.op === 'AND' || e.op === 'OR') {
    mapper.expr(e.left);
    state.condition = e.op;
    mapper.expr(e.right);
    state.condition = null;
  } else if (Object.values(FilterOperator).find((x) => e.op === x)) {
    state.currentFilter = {} as Filter;
    state.currentFilter.operator = e.op as FilterOperator;
    if (state.condition) {
      state.currentFilter.condition = state.condition;
      state.condition = null;
    }
    if (state.notFlag && state.currentFilter.operator === FilterOperator.Like) {
      state.currentFilter.operator = FilterOperator.NotLike;
      state.notFlag = false;
    }
    mapper.expr(e.left);
    mapper.expr(e.right);
    state.filters.push(state.currentFilter);
    state.currentFilter = null;
  }
}

function selectCallFunc(s: SelectedColumn): BuilderMetricField | string {
  if (s.expr.type !== 'call') {
    return {} as BuilderMetricField;
  }
  let fields = s.expr.args.map((x) => {
    if (x.type !== 'ref') {
      return '';
    }
    return x.name;
  });
  if (
    Object.values(BuilderMetricFieldAggregation).includes(
      s.expr.function.name.toLowerCase() as BuilderMetricFieldAggregation
    )
  ) {
    return {
      aggregation: s.expr.function.name as BuilderMetricFieldAggregation,
      field: fields[0],
      alias: s.alias?.name,
    } as BuilderMetricField;
  }
  return toString(s.expr).toString();
}

function getMetricsFromAst(selectClauses: SelectedColumn[] | null): {
  metrics: BuilderMetricField[];
  fields: string[];
} {
  if (!selectClauses) {
    return { metrics: [], fields: [] };
  }
  const metrics: BuilderMetricField[] = [];
  const fields: string[] = [];

  for (let s of selectClauses) {
    switch (s.expr.type) {
      case 'ref':
        fields.push(s.expr.name);
        break;
      case 'call':
        const f = selectCallFunc(s);
        if (!f) {
          break;
        }
        if (isString(f)) {
          fields.push(f);
        } else {
          metrics.push(f);
        }
        break;
      case 'string':
        fields.push(`'${s.expr.value}'`);
        break;
      case 'numeric':
      case 'boolean':
      case 'integer':
        fields.push(`${s.expr.value}`);
        break;
      case 'cast':
        fields.push(`cast(${toString(s.expr.operand)}  as ${s.expr.to.kind === undefined ? s.expr.to?.name : ''})`);
        break;
      default:
        break;
    }
  }
  return { metrics, fields };
}

function formatStringValue(
  currentFilter: string,
  templateVars: VariableWithMultiSupport[],
  multipleValue?: boolean
): string {
  const filter = Array.isArray(currentFilter) ? currentFilter[0] : currentFilter;
  const varConfigForFilter = templateVars.find((tv) => tv.name === filter.substring(1));
  return filter.startsWith('$') && (multipleValue || varConfigForFilter?.current.value.length === 1)
    ? ` ${filter || ''}`
    : ` '${filter || ''}'`;
}

function escaped(object: string) {
  return object === '' ? '' : `"${object}"`;
}

export const operMap = new Map<string, FilterOperator>([
  ['equals', FilterOperator.Equals],
  ['contains', FilterOperator.Like],
]);

export function getOper(v: string): FilterOperator {
  return operMap.get(v) || FilterOperator.Equals;
}

function removeQuotesForMultiVariables(val: string, templateVars: VariableWithMultiSupport[]): string {
  console.log(val);
  const multiVariableInWhereString = (tv: VariableWithMultiSupport) =>
    tv.multi && (val.includes(`\${${tv.name}}`) || val.includes(`$${tv.name}`));

  if (templateVars.some((tv) => multiVariableInWhereString(tv))) {
    val = val.replace(/'\)/g, ')');
    val = val.replace(/\('\)/g, '(');
  }
  return val;
}
