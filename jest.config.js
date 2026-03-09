// force timezone to UTC to allow tests to work regardless of local timezone
// generally used by snapshots, but can affect specific tests
process.env.TZ = 'UTC';

const baseConfig = require('./.config/jest.config');
const { grafanaESModules, nodeModulesToTransform } = require('./.config/jest/utils');

module.exports = {
  ...baseConfig,
  moduleNameMapper: {
    ...baseConfig.moduleNameMapper,
    // Jest can't resolve @chevrotain/* ESM packages from nested node_modules; map to entry files
    '^@chevrotain/(.+)$': '<rootDir>/node_modules/@chevrotain/$1/lib/src/api.js',
  },
  transformIgnorePatterns: [
    nodeModulesToTransform([...grafanaESModules, '@questdb/sql-parser', 'chevrotain', '@chevrotain', 'lodash-es']),
  ],
};
