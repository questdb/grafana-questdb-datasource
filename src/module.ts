import { DataSourcePlugin/*, DashboardLoadedEvent*/ } from '@grafana/data';
import { Datasource } from './data/QuestDbDatasource';
import { ConfigEditor } from './views/QuestDBConfigEditor';
import { QuestDBQueryEditor } from './views/QuestDBQueryEditor';
import { QuestDBQuery, QuestDBConfig } from './types';

export const plugin = new DataSourcePlugin<Datasource, QuestDBQuery, QuestDBConfig>(Datasource)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QuestDBQueryEditor);

