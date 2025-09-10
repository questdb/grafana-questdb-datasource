import { DataQuery, DataSourceJsonData } from '@grafana/data';

export const defaultQuery: Partial<QuestDBQuery> = {};

export enum PostgresTLSModes {
  disable = 'disable',
  require = 'require',
  verifyCA = 'verify-ca',
  verifyFull = 'verify-full',
}

export enum PostgresTLSMethods {
  filePath = 'file-path',
  fileContent = 'file-content',
}

export interface QuestDBConfig extends DataSourceJsonData {
  username: string;
  server: string;
  port: number;

  tlsAuth?: boolean;
  tlsAuthWithCACert?: boolean;
  secure?: boolean;
  validate?: boolean;
  timeout?: number;
  queryTimeout?: number;
  enableSecureSocksProxy?: boolean;
  maxOpenConnections?: number;
  maxIdleConnections?: number;
  maxConnectionLifetime?: number;
  timeInterval?: string;

  tlsMode?: PostgresTLSModes;
  tlsConfigurationMethod?: PostgresTLSMethods;

  tlsClientCertFile?: string;
  tlsClientKeyFile?: string;
}

export interface QuestDBSecureConfig {
  password: string;
  tlsCACert?: string; //sslRootCert
  tlsClientCert?: string; //sslCertFile
  tlsClientKey?: string; //sslKeyFile
}

export enum Format {
  TIMESERIES = 0,
  TABLE = 1,
  AUTO = 2,
}

//#region Query
export enum QueryType {
  SQL = 'sql',
  Builder = 'builder',
}

export interface QuestDBQueryBase extends DataQuery {}

export interface QuestDBSQLQuery extends QuestDBQueryBase {
  queryType: QueryType.SQL;
  rawSql: string;
  meta?: {
    timezone?: string;
    // meta fields to be used just for building builder options when migrating  back to QueryType.Builder
    builderOptions?: SqlBuilderOptions;
  };
  format: Format;
  selectedFormat: Format;
  expand?: boolean;
}

export interface QuestDBBuilderQuery extends QuestDBQueryBase {
  queryType: QueryType.Builder;
  rawSql: string;
  builderOptions: SqlBuilderOptions;
  format: Format;
  selectedFormat: Format;
  meta?: {
    timezone?: string;
  };
}

export type QuestDBQuery = QuestDBSQLQuery | QuestDBBuilderQuery;

export enum BuilderMode {
  List = 'list',
  Aggregate = 'aggregate',
  Trend = 'trend',
}

export enum SampleByFillMode {
  None = 'NONE',
  Null = 'NULL',
  Prev = 'PREV',
  Linear = 'LINEAR',
}

export enum SampleByAlignToMode {
  FirstObservation = 'FIRST OBSERVATION',
  Calendar = 'CALENDAR',
  CalendarTimeZone = 'CALENDAR TIME ZONE',
  CalendarOffset = 'CALENDAR WITH OFFSET',
}

export const modeRequiresValue = (mode: SampleByAlignToMode): boolean => {
  return mode === SampleByAlignToMode.CalendarTimeZone || mode === SampleByAlignToMode.CalendarOffset;
};

export interface SampleByAlignTo {
  mode: SampleByAlignToMode;
  value?: string;
}

export interface SqlBuilderOptionsList {
  mode: BuilderMode.List;
  table?: string;
  fields?: string[];
  filters?: Filter[];
  partitionBy?: string[];
  orderBy?: OrderBy[];
  limit?: string;
  timeField: string;
}
export enum BuilderMetricFieldAggregation {
  Sum = 'sum',
  Average = 'avg',
  Min = 'min',
  Max = 'max',
  Count = 'count',
  Count_Distinct = 'count_distinct',
  First = 'first',
  FirstNotNull = 'first_not_null',
  Last = 'last',
  LastNotNull = 'last_not_null',
  KSum = 'ksum',
  NSum = 'nsum',
  StDev = 'stddev',
  StDevPop = 'stddev_pop',
  VarSamp = 'var',
  VarPop = 'var_pop',
}
export type BuilderMetricField = {
  field: string;
  aggregation: BuilderMetricFieldAggregation;
  alias?: string;
};
export interface SqlBuilderOptionsAggregate {
  mode: BuilderMode.Aggregate;
  table: string;
  fields: string[];
  metrics: BuilderMetricField[];
  groupBy?: string[];
  filters?: Filter[];
  orderBy?: OrderBy[];
  limit?: string;
}
export interface SqlBuilderOptionsTrend {
  mode: BuilderMode.Trend;
  table: string;
  fields: string[];
  metrics: BuilderMetricField[];
  filters?: Filter[];
  groupBy?: string[];
  sampleByAlignTo: SampleByAlignToMode;
  sampleByAlignToValue?: string;
  sampleByFill?: string[];
  timeField: string;
  orderBy?: OrderBy[];
  limit?: string;
}

export type SqlBuilderOptions = SqlBuilderOptionsList | SqlBuilderOptionsAggregate | SqlBuilderOptionsTrend;
export interface Field {
  name: string;
  type: string;
  rel: string;
  label: string;
  ref: string[];
}

interface FullFieldPickListItem {
  value: string;
  label: string;
}
export interface FullField {
  name: string;
  label: string;
  type: string;
  picklistValues: FullFieldPickListItem[];
  filterable?: boolean;
  sortable?: boolean;
  groupable?: boolean;
  aggregatable?: boolean;
  designated?: boolean;
}

export enum OrderByDirection {
  ASC = 'ASC',
  DESC = 'DESC',
}

export interface OrderBy {
  name: string;
  dir: OrderByDirection;
}

export enum FilterOperator {
  IsNull = 'IS NULL',
  IsNotNull = 'IS NOT NULL',
  Equals = '=',
  NotEquals = '!=',
  LessThan = '<',
  LessThanOrEqual = '<=',
  GreaterThan = '>',
  GreaterThanOrEqual = '>=',
  Like = 'LIKE',
  ILike = 'ILIKE',
  NotLike = 'NOT LIKE',
  NotILike = 'NOT ILIKE',
  Match = '~',
  NotMatch = '!~',
  In = 'IN',
  NotIn = 'NOT IN',
  ContainedBy = '<<',
  ContainedByOrEqual = '<<=',
  WithInGrafanaTimeRange = 'WITH IN DASHBOARD TIME RANGE',
  OutsideGrafanaTimeRange = 'OUTSIDE DASHBOARD TIME RANGE',
}

export interface CommonFilterProps {
  filterType: 'custom';
  key: string;
  type: string;
  condition: 'AND' | 'OR';
}

export interface NullFilter extends CommonFilterProps {
  operator: FilterOperator.IsNull | FilterOperator.IsNotNull;
}

export interface BooleanFilter extends CommonFilterProps {
  type: 'boolean';
  operator: FilterOperator.Equals | FilterOperator.NotEquals;
  value: boolean;
}

export interface StringFilter extends CommonFilterProps {
  operator:
    | FilterOperator.Equals
    | FilterOperator.NotEquals
    | FilterOperator.Like
    | FilterOperator.NotLike
    | FilterOperator.ILike
    | FilterOperator.NotILike
    | FilterOperator.Match
    | FilterOperator.NotMatch;
  value: string;
}

export interface IpFilter extends CommonFilterProps {
  operator: FilterOperator.ContainedBy | FilterOperator.ContainedByOrEqual;
  value: string;
}

export interface NumberFilter extends CommonFilterProps {
  operator:
    | FilterOperator.Equals
    | FilterOperator.NotEquals
    | FilterOperator.LessThan
    | FilterOperator.LessThanOrEqual
    | FilterOperator.GreaterThan
    | FilterOperator.GreaterThanOrEqual
    | FilterOperator.In
    | FilterOperator.NotIn;
  value: number;
}

export interface DateFilterWithValue extends CommonFilterProps {
  type: 'timestamp' | 'date';
  operator:
    | FilterOperator.Equals
    | FilterOperator.NotEquals
    | FilterOperator.LessThan
    | FilterOperator.LessThanOrEqual
    | FilterOperator.GreaterThan
    | FilterOperator.GreaterThanOrEqual;
  value: string;
}

export interface DateFilterWithoutValue extends CommonFilterProps {
  type: 'timestamp' | 'date';
  operator: FilterOperator.WithInGrafanaTimeRange | FilterOperator.OutsideGrafanaTimeRange;
}

export type DateFilter = DateFilterWithValue | DateFilterWithoutValue;

export interface MultiFilter extends CommonFilterProps {
  operator: FilterOperator.In | FilterOperator.NotIn;
  value: string[];
}

export type Filter = NullFilter | BooleanFilter | NumberFilter | DateFilter | StringFilter | MultiFilter | IpFilter;

//#endregion

//#region Default Queries
export const defaultQueryType: QueryType = QueryType.Builder;
export const defaultBuilderQuery: Omit<QuestDBBuilderQuery, 'refId'> = {
  queryType: QueryType.Builder,
  rawSql: '',
  builderOptions: {
    mode: BuilderMode.List,
    fields: [],
    limit: '',
    timeField: '',
  },
  format: Format.TABLE,
  selectedFormat: Format.AUTO,
};
export const defaultSQLQuery: Omit<QuestDBSQLQuery, 'refId'> = {
  queryType: QueryType.SQL,
  rawSql: '',
  format: Format.TABLE,
  selectedFormat: Format.AUTO,
  expand: false,
};
//#endregion
