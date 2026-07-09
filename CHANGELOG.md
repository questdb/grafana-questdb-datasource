# Changelog

All notable changes to `@questdb/grafana-questdb-datasource` project will be documented in this
file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Types of changes

- `Added` for new features.
- `Changed` for changes in existing functionality.
- `Deprecated` for soon-to-be removed features.
- `Removed` for now removed features.
- `Fixed` for any bug fixes.
- `Security` in case of vulnerabilities.

## Unreleased

## Added

- Playwright end-to-end test suite (`yarn e2e`) driving a real Grafana + QuestDB stack via
  `docker compose`: datasource provisioning and health check, config-editor UI, and dashboard
  queries through the full frontend/backend pipeline.
- New datasource option **Disable prepared statements** (`disablePreparedStatements` in
  `jsonData`, default off): inlines the dashboard time bounds as literals instead of binding
  them as query parameters. Required for QuestDB servers older than 8.3.0, which reject bind
  parameters.

## Changed

- Time-bound macros (`$__fromTime`, `$__toTime`, `$__timeFilter`) now bind the dashboard time
  window as query parameters (`$1`, `$2`) instead of inlining literal timestamps, keeping the SQL
  text identical across refreshes so QuestDB serves repeated queries from its compiled-plan cache
  (requires QuestDB 8.3.0+ and `pg.select.cache.enabled=true`, the default). Multi-statement
  queries and queries containing a hand-written `$N` placeholder automatically keep the previous
  literal-inlining behavior. **Breaking for QuestDB older than 8.3.0:** such servers reject bind
  parameters, so time-macro queries fail until the new **Disable prepared statements** datasource
  option is enabled; the plugin logs a warning at connect time when it detects such a server.
- The query inspector's "executed query" shows the placeholder form (`cast($1 as timestamp)`)
  rather than literal epoch values for parameterized queries.

## 0.1.4

## Changed

- Enclose variables and column names in quotes in the generated SQL [#107](https://github.com/questdb/grafana-questdb-datasource/pull/107)
- Add VARCHAR type [#107](https://github.com/questdb/grafana-questdb-datasource/pull/107)
- Update docker-compose yaml to use QuestDB 8.0.3 [#107](https://github.com/questdb/grafana-questdb-datasource/pull/107)

## 0.1.3

## Changed

- Remove the deprecated `vectorator` method and use an array format for manipulation. [#97](https://github.com/questdb/ui/pull/97)
- Update the necessary copyright + add NOTICE to signal derivative work [#97](https://github.com/questdb/ui/pull/97)
- Phase out `@grafana/experimental` in favor of `@grafana/plugin-ui` whenever possible.

## 0.1.0

## Added

- Initial Beta release.
