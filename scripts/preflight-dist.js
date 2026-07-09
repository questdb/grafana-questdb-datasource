#!/usr/bin/env node
/* eslint-disable no-console */
// Preflight for `yarn server`: docker-compose mounts ./dist into the Grafana
// container, and starting the stack without a built plugin is a trap — Docker
// auto-creates the missing mount source as a root-owned empty directory, Grafana
// reports healthy with no plugin loaded, and the next `yarn build` then fails with
// EACCES on the root-owned dist/. Fail fast with the remedy instead.
const fs = require('fs');

const problems = [];
if (!fs.existsSync('dist/plugin.json')) {
  problems.push('dist/plugin.json is missing — build the frontend:   yarn build');
}
if (!fs.existsSync('dist') || !fs.readdirSync('dist').some((f) => f.startsWith('gpx_questdb_linux_'))) {
  problems.push('dist/ has no linux backend binary — build it:       mage build:linux');
}
if (problems.length > 0) {
  console.error('dist/ does not contain a built plugin; refusing to start the stack:\n');
  for (const p of problems) {
    console.error(`  - ${p}`);
  }
  console.error('\nSee RELEASE.md sections 3.3 and 4.1.');
  process.exit(1);
}
