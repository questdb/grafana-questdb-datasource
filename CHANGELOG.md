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
