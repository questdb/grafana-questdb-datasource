import { VariableWithMultiSupport } from '@grafana/data';
import type {
  Expression,
  BinaryExpression,
  Literal,
  FunctionCall,
  CastExpression,
  ColumnRef,
  UnaryExpression,
  InExpression,
  IsNullExpression,
  QualifiedName,
  SelectStatement,
  ExpressionSelectItem,
  SelectItem,
} from '@questdb/sql-parser';
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
  return ['date', 'timestamp', 'timestamp_ns'].includes(type?.toLowerCase());
};
export const isTimestampType = (type: string): boolean => {
  return ['timestamp', 'timestamp_ns'].includes(type?.toLowerCase());
};

export const isIPv4Type = (type: string): boolean => {
  return 'ipv4' === type?.toLowerCase();
};

export const isStringType = (type: string): boolean => {
  return ['string', 'symbol', 'char', 'varchar'].includes(type?.toLowerCase());
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
    return f !== '*' && f.match(/(^\d|[^a-zA-Z_0-9])/im) ? `"${f}"` : f;
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
  const ast = sqlToStatement(sql) as SelectStatement;
  if (!ast || ast.type !== 'select') {
    return "The query can't be parsed.";
  }
  if (!ast.from || ast.from.length !== 1) {
    return `The query has too many 'FROM' clauses.`;
  }
  if (ast.from[0].table.type !== 'qualifiedName') {
    return `The 'FROM' clause is not a table.`;
  }
  const tableName = (ast.from[0].table as QualifiedName).parts;
  const tableNameStr = tableName[tableName.length - 1];

  let timeField;
  let fieldsToTypes = new Map<string, string>();

  if (tableNameStr.length > 0 && datasource) {
    const dbFields = await datasource.fetchFields(tableNameStr);
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
    table: tableNameStr,
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
      if (ob.expression.type !== 'column') {
        return {} as OrderBy;
      }
      const colName = (ob.expression as ColumnRef).name.parts;
      const name = colName[colName.length - 1];
      if (name === 'time') {
        return {} as OrderBy;
      }
      return { name, dir: (ob.direction || 'asc').toUpperCase() } as OrderBy;
    })
    .filter((x) => x.name);

  if (orderBy && orderBy.length > 0) {
    (builder as SqlBuilderOptionsAggregate).orderBy = orderBy!;
  }

  builder.limit = undefined;
  if (ast.limit) {
    if (ast.limit.upperBound && ast.limit.upperBound.type === 'literal') {
      // LIMIT offset, count
      if (ast.limit.lowerBound && ast.limit.lowerBound.type === 'literal') {
        builder.limit = `${(ast.limit.lowerBound as Literal).value}, ${(ast.limit.upperBound as Literal).value}`;
      }
    } else if (ast.limit.lowerBound && ast.limit.lowerBound.type === 'literal') {
      // LIMIT count
      builder.limit = `${(ast.limit.lowerBound as Literal).value}`;
    }
  }

  if (ast.sampleBy) {
    builder.mode = BuilderMode.Trend;
    if (ast.sampleBy.alignTo) {
      const alignTo = ast.sampleBy.alignTo;
      if (alignTo.mode === 'firstObservation') {
        (builder as SqlBuilderOptionsTrend).sampleByAlignTo = SampleByAlignToMode.FirstObservation;
      } else if (alignTo.mode === 'calendar') {
        if (alignTo.timeZone) {
          (builder as SqlBuilderOptionsTrend).sampleByAlignTo = SampleByAlignToMode.CalendarTimeZone;
        } else if (alignTo.offset) {
          (builder as SqlBuilderOptionsTrend).sampleByAlignTo = SampleByAlignToMode.CalendarOffset;
        } else {
          (builder as SqlBuilderOptionsTrend).sampleByAlignTo = SampleByAlignToMode.Calendar;
        }
      }
    }
    if (ast.sampleBy.fill) {
      (builder as SqlBuilderOptionsTrend).sampleByFill = ast.sampleBy.fill;
    }
    if (ast.sampleBy.alignTo) {
      const alignToValue = ast.sampleBy.alignTo.timeZone || ast.sampleBy.alignTo.offset;
      if (alignToValue) {
        (builder as SqlBuilderOptionsTrend).sampleByAlignToValue = alignToValue;
      }
    }
  }

  if (ast.latestOn) {
    builder.mode = BuilderMode.List;
    if (ast.latestOn.partitionBy) {
      (builder as SqlBuilderOptionsList).partitionBy = ast.latestOn.partitionBy.map((p) => {
        return p.parts.join('.');
      });
    }
  }

  const groupBy = ast.groupBy
    ?.map((gb) => {
      if (gb.type !== 'column') {
        return '';
      }
      const colName = (gb as ColumnRef).name.parts;
      const name = colName[colName.length - 1];
      if (name === 'time') {
        return '';
      }
      return name;
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

function getFiltersFromAst(expr: Expression, fieldsToTypes: Map<string, string>): Filter[] {
  let state: MapperState = { currentFilter: null, filters: [], notFlag: false, condition: null } as MapperState;

  try {
    visitExpr(expr, state, fieldsToTypes);
  } catch (error) {
    console.error(error);
  }

  return state.filters;
}

function visitExpr(e: Expression | undefined | null, state: MapperState, fieldsToTypes: Map<string, string>) {
  if (!e) {
    return;
  }
  switch (e.type) {
    case 'binary':
      getBinaryFilter(e as BinaryExpression, state, fieldsToTypes);
      break;
    case 'literal':
      handleLiteral(e as Literal, state, fieldsToTypes);
      break;
    case 'function':
      getCallFilter(e as FunctionCall, state);
      break;
    case 'cast':
      getCastFilter(e as CastExpression, state);
      break;
    case 'column':
      getRefFilter(e as ColumnRef, state, fieldsToTypes);
      break;
    case 'unary':
      getUnaryFilter(e as UnaryExpression, state, fieldsToTypes);
      break;
    case 'in':
      getInFilter(e as InExpression, state, fieldsToTypes);
      break;
    case 'isNull':
      getIsNullFilter(e as IsNullExpression, state, fieldsToTypes);
      break;
    case 'paren': {
      const paren = e as any;
      visitExpr(paren.expression, state, fieldsToTypes);
      break;
    }
    default:
      console.error(`${e?.type} is not supported. This is likely a bug.`);
      break;
  }
}

function handleLiteral(e: Literal, state: MapperState, fieldsToTypes: Map<string, string>) {
  switch (e.literalType) {
    case 'boolean':
      getBooleanFilter(e, state);
      break;
    case 'string':
      getStringFilter(e, state);
      break;
    case 'number':
      if (Number.isInteger(e.value)) {
        getIntFilter(e, state);
      } else {
        getNumericFilter(e, state);
      }
      break;
    case 'null':
      // null literals handled elsewhere (IS NULL expressions)
      break;
    default:
      break;
  }
}

function getColumnName(e: ColumnRef): string {
  return e.name.parts[e.name.parts.length - 1];
}

function getRefFilter(e: ColumnRef, state: MapperState, fieldsToTypes: Map<string, string>) {
  const name = getColumnName(e);
  let doAdd = false;
  if (state.currentFilter === null) {
    state.currentFilter = {} as Filter;
    doAdd = true;
  }

  if (name?.toLowerCase() === '$__fromtime') {
    state.currentFilter = { ...state.currentFilter, value: 'GRAFANA_START_TIME', type: 'timestamp' } as Filter;
    return;
  }

  if (name?.toLowerCase() === '$__totime') {
    state.currentFilter = { ...state.currentFilter, value: 'GRAFANA_END_TIME', type: 'timestamp' } as Filter;
    return;
  }

  let type = fieldsToTypes.get(name);
  if (!state.currentFilter.key) {
    state.currentFilter = { ...state.currentFilter, key: name };
    if (type) {
      state.currentFilter.type = type;
    }
  } else {
    state.currentFilter = { ...state.currentFilter, value: [name], type: type || 'string' } as Filter;
  }

  if (doAdd) {
    state.filters.push(state.currentFilter);
    state.currentFilter = null;
  }
}

function getInFilter(e: InExpression, state: MapperState, fieldsToTypes: Map<string, string>) {
  state.currentFilter = {} as Filter;
  state.currentFilter.operator = e.not ? FilterOperator.NotIn : FilterOperator.In;
  if (state.condition) {
    state.currentFilter.condition = state.condition;
    state.condition = null;
  }
  if (state.notFlag) {
    if (state.currentFilter.operator === FilterOperator.In) {
      state.currentFilter.operator = FilterOperator.NotIn;
    }
    state.notFlag = false;
  }

  // Extract key from expression (column ref)
  visitExpr(e.expression, state, fieldsToTypes);

  // Extract values
  state.currentFilter = {
    ...state.currentFilter,
    value: e.values.map((x) => {
      return (x as Literal).value;
    }),
    type: state.currentFilter?.type || 'string',
  } as Filter;

  state.filters.push(state.currentFilter);
  state.currentFilter = null;
}

function getIsNullFilter(e: IsNullExpression, state: MapperState, fieldsToTypes: Map<string, string>) {
  state.currentFilter = {
    operator: e.not ? FilterOperator.IsNotNull : FilterOperator.IsNull,
  } as Filter;
  if (state.condition) {
    state.currentFilter.condition = state.condition;
    state.condition = null;
  }
  // Visit the expression to extract the column name (key)
  visitExpr(e.expression, state, fieldsToTypes);
  state.filters.push(state.currentFilter);
  state.currentFilter = null;
}

function getCallString(e: FunctionCall): string {
  let args: string = e.args
    .map((x) => {
      return toString(x);
    })
    .join(', ');

  if (e.star) {
    args = '*';
  }

  return `${e.name}(${args})`;
}

function toString(x: Expression): string | number | boolean {
  switch (x.type) {
    case 'literal': {
      const lit = x as Literal;
      if (lit.literalType === 'string') {
        return `'${lit.value}'`;
      }
      if (lit.literalType === 'null') {
        return 'null';
      }
      return lit.value as number | boolean;
    }
    case 'column':
      return getColumnName(x as ColumnRef);
    case 'function':
      return getCallString(x as FunctionCall);
    case 'unary': {
      const unary = x as UnaryExpression;
      return `${unary.operator}${toString(unary.operand)}`;
    }
    default:
      return '';
  }
}

function getCallFilter(e: FunctionCall, state: MapperState) {
  let doAdd = false;
  if (!state.currentFilter) {
    // map f(x) to true = f(x) so it can be displayed in builder
    state.currentFilter = { key: 'true', type: 'boolean' } as Filter;
    doAdd = true;
  }

  let args: string;
  if (e.star) {
    args = '*';
  } else {
    args = e.args
      .map((x) => {
        return toString(x);
      })
      .join(', ');
  }
  const val = `${e.name}(${args})`;

  if (val.toLowerCase().startsWith('$__timefilter(')) {
    const firstArg = e.args[0];
    const argName = firstArg && firstArg.type === 'column' ? getColumnName(firstArg as ColumnRef) : '';
    state.currentFilter = {
      ...state.currentFilter,
      key: argName,
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

function getUnaryFilter(e: UnaryExpression, state: MapperState, fieldsToTypes: Map<string, string>) {
  if (e.operator === 'NOT') {
    state.notFlag = true;
    visitExpr(e.operand, state, fieldsToTypes);
    state.notFlag = false;
    return;
  }

  // Other unary operators (shouldn't happen often with new parser, IS NULL/NOT handled separately)
  state.currentFilter = { operator: e.operator as FilterOperator } as Filter;
  if (state.condition) {
    state.currentFilter.condition = state.condition;
    state.condition = null;
  }
  visitExpr(e.operand, state, fieldsToTypes);
  state.filters.push(state.currentFilter);
  state.currentFilter = null;
}

function getStringFilter(e: Literal, state: MapperState) {
  if (state.currentFilter != null && !state.currentFilter.key) {
    state.currentFilter = { ...state.currentFilter, key: e.value as string } as Filter;
    return;
  }
  state.currentFilter = {
    ...state.currentFilter,
    value: e.value,
    type: state.currentFilter?.type || 'string',
  } as Filter;
}

function getNumericFilter(e: Literal, state: MapperState) {
  if (state.currentFilter != null && !state.currentFilter.key) {
    state.currentFilter = { ...state.currentFilter, key: (e.value as number).toString() } as Filter;
    return;
  }
  state.currentFilter = { ...state.currentFilter, value: e.value, type: 'double' } as Filter;
}

function getIntFilter(e: Literal, state: MapperState) {
  if (state.currentFilter != null && !state.currentFilter.key) {
    state.currentFilter = { ...state.currentFilter, key: (e.value as number).toString() } as Filter;
    return;
  }
  state.currentFilter = { ...state.currentFilter, value: e.value, type: 'int' } as Filter;
}

function getCastFilter(e: CastExpression, state: MapperState) {
  let val = `cast( ${toString(e.expression)} as ${e.dataType.toLowerCase()} )`;

  if (state.currentFilter != null && !state.currentFilter.key) {
    state.currentFilter = { ...state.currentFilter, key: val } as Filter;
    return;
  } else {
    state.currentFilter = { ...state.currentFilter, value: val, type: state.currentFilter?.type || 'int' } as Filter;
  }
}

function getBooleanFilter(e: Literal, state: MapperState) {
  state.currentFilter = { ...state.currentFilter, value: e.value, type: 'boolean' } as Filter;
}

function getBinaryFilter(e: BinaryExpression, state: MapperState, fieldsToTypes: Map<string, string>) {
  if (e.operator === 'AND' || e.operator === 'OR') {
    visitExpr(e.left, state, fieldsToTypes);
    state.condition = e.operator;
    visitExpr(e.right, state, fieldsToTypes);
    state.condition = null;
  } else if (Object.values(FilterOperator).find((x) => e.operator === x)) {
    state.currentFilter = {} as Filter;
    state.currentFilter.operator = e.operator as FilterOperator;
    if (state.condition) {
      state.currentFilter.condition = state.condition;
      state.condition = null;
    }
    if (state.notFlag && state.currentFilter.operator === FilterOperator.Like) {
      state.currentFilter.operator = FilterOperator.NotLike;
      state.notFlag = false;
    }
    visitExpr(e.left, state, fieldsToTypes);
    visitExpr(e.right, state, fieldsToTypes);
    state.filters.push(state.currentFilter);
    state.currentFilter = null;
  }
}

function selectCallFunc(s: ExpressionSelectItem): BuilderMetricField | string {
  if (s.expression.type !== 'function') {
    return {} as BuilderMetricField;
  }
  const funcExpr = s.expression as FunctionCall;
  let fields: string[];

  if (funcExpr.star) {
    fields = ['*'];
  } else {
    fields = funcExpr.args.map((x) => {
      if (x.type !== 'column') {
        return '';
      }
      return getColumnName(x as ColumnRef);
    });
  }

  if (
    Object.values(BuilderMetricFieldAggregation).includes(
      funcExpr.name.toLowerCase() as BuilderMetricFieldAggregation
    )
  ) {
    return {
      aggregation: funcExpr.name.toLowerCase() as BuilderMetricFieldAggregation,
      field: fields[0],
      alias: s.alias,
    } as BuilderMetricField;
  }
  return toString(s.expression).toString();
}

function getMetricsFromAst(selectClauses: SelectItem[] | null): {
  metrics: BuilderMetricField[];
  fields: string[];
} {
  if (!selectClauses) {
    return { metrics: [], fields: [] };
  }
  const metrics: BuilderMetricField[] = [];
  const fields: string[] = [];

  for (let s of selectClauses) {
    if (s.type === 'star') {
      fields.push('*');
      continue;
    }
    if (s.type !== 'selectItem') {
      continue;
    }
    const item = s as ExpressionSelectItem;
    switch (item.expression.type) {
      case 'column':
        fields.push(getColumnName(item.expression as ColumnRef));
        break;
      case 'function':
        const f = selectCallFunc(item);
        if (!f) {
          break;
        }
        if (isString(f)) {
          fields.push(f);
        } else {
          metrics.push(f);
        }
        break;
      case 'literal': {
        const lit = item.expression as Literal;
        if (lit.literalType === 'string') {
          fields.push(`'${lit.value}'`);
        } else {
          fields.push(`${lit.value}`);
        }
        break;
      }
      case 'cast': {
        const cast = item.expression as CastExpression;
        fields.push(`cast(${toString(cast.expression)}  as ${cast.dataType.toLowerCase()})`);
        break;
      }
      case 'typeCast': {
        const typeCast = item.expression as any;
        fields.push(`cast(${toString(typeCast.expression)}  as ${typeCast.dataType.toLowerCase()})`);
        break;
      }
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
  const extractedVariableName = filter.substring(1).replace(/[{}]/g, '');
  const varConfigForFilter = templateVars.find((tv) => tv.name === extractedVariableName);
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
  const multiVariableInWhereString = (tv: VariableWithMultiSupport) =>
    tv.multi && (val.includes(`\${${tv.name}}`) || val.includes(`$${tv.name}`));

  if (templateVars.some((tv) => multiVariableInWhereString(tv))) {
    val = val.replace(/'\)/g, ')');
    val = val.replace(/\('\)/g, '(');
  }
  return val;
}
