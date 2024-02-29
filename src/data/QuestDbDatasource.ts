import {
  DataFrame,
  DataFrameView,
  DataQueryRequest,
  DataQueryResponse,
  DataSourceInstanceSettings,
  getTimeZone,
  getTimeZoneInfo,
  MetricFindValue,
  ScopedVars,
  TypedVariableModel,
  vectorator,
} from '@grafana/data';
import { DataSourceWithBackend, getTemplateSrv } from '@grafana/runtime';
import { Observable } from 'rxjs';
import {
  QuestDBConfig,
  QuestDBQuery,
  FullField,
  QueryType,
} from '../types';
import { AdHocFilter } from './adHocFilter';
import { isString } from 'lodash';
import {Table} from "../components/questdb-sql/utils";
import {InformationSchemaColumn} from "../components/questdb-sql/types";

export class Datasource
  extends DataSourceWithBackend<QuestDBQuery, QuestDBConfig>
{
  // This enables default annotation support for 7.2+
  annotations = {};
  settings: DataSourceInstanceSettings<QuestDBConfig>;
  adHocFilter: AdHocFilter;
  skipAdHocFilter = false; // don't apply adhoc filters to the query

  constructor(instanceSettings: DataSourceInstanceSettings<QuestDBConfig>) {
    super(instanceSettings);
    this.settings = instanceSettings;
    this.adHocFilter = new AdHocFilter();
  }

  async metricFindQuery(query: QuestDBQuery | string, options: any) {
    const chQuery = isString(query) ? { rawSql: query, queryType: QueryType.SQL } : query;

    if (!(chQuery.queryType === QueryType.SQL || chQuery.queryType === QueryType.Builder || !chQuery.queryType)) {
      return [];
    }

    if (!chQuery.rawSql) {
      return [];
    }
    const q = { ...chQuery, queryType: chQuery.queryType || QueryType.SQL };
    const frame = await this.runQuery(q, options);
    if (frame.fields?.length === 0) {
      return [];
    }
    if (frame?.fields?.length === 1) {
      return vectorator(frame?.fields[0]?.values).map((text) => ({ text, value: text }));
    }
    // convention - assume the first field is an id field
    const ids = frame?.fields[0]?.values;
    return vectorator(frame?.fields[1]?.values).map((text, i) => ({ text, value: ids.get(i) }));
  }

  applyTemplateVariables(query: QuestDBQuery, scoped: ScopedVars): QuestDBQuery {
    let rawQuery = query.rawSql || '';
    // we want to skip applying ad hoc filters when we are getting values for ad hoc filters
    const templateSrv = getTemplateSrv();
    if (!this.skipAdHocFilter) {
      const adHocFilters = (templateSrv as any)?.getAdhocFilters(this.name);
      rawQuery = this.adHocFilter.apply(rawQuery, adHocFilters);
    }
    this.skipAdHocFilter = false;
    rawQuery = this.applyConditionalAll(rawQuery, getTemplateSrv().getVariables());
    return {
      ...query,
      rawSql: this.replace(rawQuery, scoped) || '',
    };
  }

  applyConditionalAll(rawQuery: string, templateVars: TypedVariableModel[]): string {
    if (!rawQuery) {
      return rawQuery;
    }
    const macro = '$__conditionalAll(';
    let macroIndex = rawQuery.lastIndexOf(macro);

    while (macroIndex !== -1) {
      const params = this.getMacroArgs(rawQuery, macroIndex + macro.length - 1);
      if (params.length !== 2) {
        return rawQuery;
      }
      const templateVarParam = params[1].trim();
      const varRegex = new RegExp(/(?<=\$\{)[\w\d]+(?=\})|(?<=\$)[\w\d]+/);
      const templateVar = varRegex.exec(templateVarParam);
      let phrase = params[0];
      if (templateVar) {
        const key = templateVars.find((x) => x.name === templateVar[0]) as any;
        let value = key?.current.value.toString();
        if (value === '' || value === '$__all') {
          phrase = '1=1';
        }
      }
      rawQuery = rawQuery.replace(`${macro}${params[0]},${params[1]})`, phrase);
      macroIndex = rawQuery.lastIndexOf(macro);
    }
    return rawQuery;
  }

  private getMacroArgs(query: string, argsIndex: number): string[] {
    const args = [] as string[];
    const re = /\(|\)|,/g;
    let bracketCount = 0;
    let lastArgEndIndex = 1;
    let regExpArray: RegExpExecArray | null;
    const argsSubstr = query.substring(argsIndex, query.length);
    while ((regExpArray = re.exec(argsSubstr)) !== null) {
      const foundNode = regExpArray[0];
      if (foundNode === '(') {
        bracketCount++;
      } else if (foundNode === ')') {
        bracketCount--;
      }
      if (foundNode === ',' && bracketCount === 1) {
        args.push(argsSubstr.substring(lastArgEndIndex, re.lastIndex - 1));
        lastArgEndIndex = re.lastIndex;
      }
      if (bracketCount === 0) {
        args.push(argsSubstr.substring(lastArgEndIndex, re.lastIndex - 1));
        return args;
      }
    }
    return [];
  }

  private replace(value?: string, scopedVars?: ScopedVars) {
    if (value !== undefined) {
      return getTemplateSrv().replace(value, scopedVars, this.format);
    }
    return value;
  }

  private format(value: any) {
    if (Array.isArray(value)) {
      return `'${value.join("','")}'`;
    }
    return value;
  }

  async fetchTables(): Promise<Table[]> {
    const rawSql = `select table_name, partitionBy, designatedTimestamp, walEnabled, dedup  from tables()`;
    const frame = await this.runQuery({ rawSql });
    if (frame.fields?.length === 0) {
      return [];
    }
    const view = new DataFrameView(frame);
    return view.map((item) => ({
      tableName: item[0],
      partitionBy: item[1],
      designatedTimestamp: item[2] === null ? "" : item[2],
      walEnabled: item[3],
      dedup: item[4]
    }));
  }

  async fetchFields(table: string): Promise<FullField[]> {
    const rawSql = `select "column", type, designated from table_columns(\'${table}\')`;
    const frame = await this.runQuery({ rawSql });
    if (frame.fields?.length === 0) {
      return [];
    }
    const view = new DataFrameView(frame);
    return view.map((item) => ({
      name: item[0],
      type: item[1],
      label: item[0] + ' [' + item[1] + ']',
      picklistValues: [],
      designated: item[2],
    }));
  }

  async fetchTableFields(): Promise<InformationSchemaColumn[]> {
    const rawSql = 'select table_name, ordinal_position, column_name, data_type from information_schema.columns';
    const frame = await this.runQuery({ rawSql });
    if (frame.fields?.length === 0) {
      return [];
    }
    const view = new DataFrameView(frame);
    return view.map((item) => ({
      tableName: item[0],
      ordinalPosition:item[1],
      columnName: item[2],
      dataType: item[3]
    }));
  }

  private getTimezone(request: DataQueryRequest<QuestDBQuery>): string | undefined {
    // timezone specified in the time picker
    if (request.timezone && request.timezone !== 'browser') {
      return request.timezone;
    }
    // fall back to the local timezone
    const localTimezoneInfo = getTimeZoneInfo(getTimeZone(), Date.now());
    return localTimezoneInfo?.ianaName;
  }

  query(request: DataQueryRequest<QuestDBQuery>): Observable<DataQueryResponse> {
    const targets = request.targets
      // filters out queries disabled in UI
      .filter((t) => t.hide !== true)
      // attach timezone information
      .map((t) => {
        return {
          ...t,
          meta: {
            ...t.meta,
            timezone: this.getTimezone(request),
          },
        };
      });

    return super.query({
      ...request,
      targets,
    });
  }

  private runQuery(request: Partial<QuestDBQuery>, options?: any): Promise<DataFrame> {
    return new Promise((resolve) => {
      const req = {
        targets: [{ ...request, refId: String(Math.random()) }],
        range: options ? options.range : (getTemplateSrv() as any).timeRange,
      } as DataQueryRequest<QuestDBQuery>;
      this.query(req).subscribe((res: DataQueryResponse) => {
        resolve(res.data[0] || { fields: [] });
      });
    });
  }

  async getTagKeys(): Promise<MetricFindValue[]> {
    const { type, frame } = await this.fetchTags();
    if (type === TagType.query) {
      return frame.fields.map((f) => ({ text: f.name }));
    }
    const view = new DataFrameView(frame);
    return view.map((item) => ({
      text: `${item[2]}.${item[0]}`,
    }));
  }

  async getTagValues({ key }: any): Promise<MetricFindValue[]> {
    const { type } = this.getTagSource();
    this.skipAdHocFilter = true;
    if (type === TagType.query) {
      return this.fetchTagValuesFromQuery(key);
    }
    return this.fetchTagValuesFromSchema(key);
  }

  private async fetchTagValuesFromSchema(key: string): Promise<MetricFindValue[]> {
    const { from } = this.getTagSource();
    const [table, col] = key.split('.');
    const source = from?.includes('.') ? `${from.split('.')[0]}.${table}` : table;
    const rawSql = `select distinct ${col} from ${source} limit 1000`;
    const frame = await this.runQuery({ rawSql });
    if (frame.fields?.length === 0) {
      return [];
    }
    const field = frame.fields[0];
    // Convert to string to avoid https://github.com/grafana/grafana/issues/12209
    return vectorator(field.values)
      .filter((value) => value !== null)
      .map((value) => {
        return { text: String(value) };
      });
  }

  private async fetchTagValuesFromQuery(key: string): Promise<MetricFindValue[]> {
    const { frame } = await this.fetchTags();
    const field = frame.fields.find((f) => f.name === key);
    if (field) {
      // Convert to string to avoid https://github.com/grafana/grafana/issues/12209
      return vectorator(field.values)
        .filter((value) => value !== null)
        .map((value) => {
          return { text: String(value) };
        });
    }
    return [];
  }

  private async fetchTags(): Promise<Tags> {
    const tagSource = this.getTagSource();
    this.skipAdHocFilter = true;

    if (tagSource.source === undefined) {
      this.adHocFilter.setTargetTable('default');
      const rawSql = 'select column_name, data_type, table_name from information_schema.columns';
      const results = await this.runQuery({ rawSql });
      return { type: TagType.schema, frame: results };
    }

    if (tagSource.type === TagType.query) {
      this.adHocFilter.setTargetTableFromQuery(tagSource.source);
    } else {
      let table = tagSource.from;
      if (table?.includes('.')) {
        table = table.split('.')[1];
      }
      this.adHocFilter.setTargetTable(table || '');
    }

    const results = await this.runQuery({ rawSql: tagSource.source });
    return { type: tagSource.type, frame: results };
  }

  private getTagSource() {
    // @todo https://github.com/grafana/grafana/issues/13109
    const ADHOC_VAR = '$questdb_adhoc_query';
    let source = getTemplateSrv().replace(ADHOC_VAR);
    source = source === ADHOC_VAR ? "" : source;
    if ( !source ){
      const sql = 'select column_name, data_type, table_name from information_schema.columns';
      return { type: TagType.schema, source: sql, from: source };
    }
    if (source.toLowerCase().startsWith('select')) {
      return { type: TagType.query, source };
    }
    const tables = source.split(',').filter((t) => t?.trim()).join(',');
    const sql = `select column_name, data_type, table_name from information_schema.columns WHERE table_name IN ('${tables}')`;
    return { type: TagType.schema, source: sql, from: source };
  }
}

enum TagType {
  query,
  schema,
}

interface Tags {
  type?: TagType;
  frame: DataFrame;
}
