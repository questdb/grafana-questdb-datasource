import * as fs from 'fs';
import { Props } from '../views/QuestDBConfigEditor';
import { QuestDBConfig } from 'types';

const pluginJson = JSON.parse(fs.readFileSync('./src/plugin.json', 'utf-8'));

export const mockConfigEditorProps = (overrides?: Partial<QuestDBConfig>): Props => ({
  options: {
    ...pluginJson,
    jsonData: {
      server: 'questdb.com',
      port: 8812,
      username: 'user',
      ...overrides
    },
  },
  onOptionsChange: jest.fn(),
});
