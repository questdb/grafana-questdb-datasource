import { PluginType } from '@grafana/data';
import {QuestDBQuery, QueryType} from '../types';
import { Datasource } from '../data/QuestDbDatasource';

export const mockDatasource = new Datasource({
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

export const mockQuery: QuestDBQuery = {
  rawSql: 'select * from foo',
  refId: '',
  format: 1,
  queryType: QueryType.SQL,
  selectedFormat: 4,
};
