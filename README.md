# QuestDB data source for Grafana

The QuestDB data source plugin enables querying and visualization of your
QuestDB time series data directly within Grafana. Compatible with all
editions—Grafana OSS, Grafana Enterprise, and Grafana Cloud—it also
fully supports both QuestDB OSS and QuestDB Enterprise.


<img alt="Sql builder screenshot" src="https://github.com/questdb/grafana-questdb-datasource/blob/main/sql_builder.png?raw=true" width="800" >

The plugin supports [Private Data Source Connect](https://grafana.com/docs/grafana-cloud/connect-externally-hosted/private-data-source-connect/) (
minimum version required `0.1.6`).

## Installation

For detailed instructions on how to install the plugin on Grafana Cloud or
locally, please check out the [Plugin installation docs](https://grafana.com/docs/grafana/latest/plugins/installation/).

Read the guide on QuestDB website: [Third-party Tools - Grafana](https://questdb.io/docs/third-party-tools/grafana/).

## Configuration

### QuestDB user for the data source

Set up an QuestDB user account with readonly permission and access to
databases and tables you want to query. Please note that Grafana does not
validate that queries are safe. Queries can contain any SQL statement. For
example, statements like `UPDATE users SET name='blahblah'`
and `DROP TABLE importantTable;` would be executed.

To configure a readonly user, follow these steps:

- Open Source version
  1. Set the following properties in server.conf file:
     - pg.readonly.user.enabled=true
     - pg.readonly.user=myuser
     - pg.readonly.password=secret
  2. Restart QuestDB instance.
- Enterprise version
  1. Create user:
     - CREATE USER grafana_readonly;
  2. Grant read permission on selected tables/table columns ;
     - GRANT SELECT ON table1, ... TO grafana_readonly;

### Manual configuration

Once the plugin is installed on your Grafana instance, follow [these
instructions](https://grafana.com/docs/grafana/latest/datasources/add-a-data-source/)
to add a new QuestDB data source, and enter configuration options.

### With a configuration file

It is possible to configure data sources using configuration files with
Grafana’s provisioning system. To read about how it works, including all the
settings that you can set for this data source, refer to [Provisioning Grafana
data sources](https://grafana.com/docs/grafana/latest/administration/provisioning/#data-sources).

Note that the plugin must be previously installed. If you
are using Docker and want to automate installation, you can set the [GF_INSTALL_PLUGINS environment
variable](https://grafana.com/docs/grafana/latest/setup-grafana/configure-docker/#install-plugins-in-the-docker-container)

```bash
docker run -p 3000:3000 -e GF_INSTALL_PLUGINS=questdb-questdb-datasource grafana/grafana-oss
```

This is an example provisioning file for this data source using the default configuration for QuestDB Open Source.

```yaml
apiVersion: 1
datasources:
  - name: QuestDB
    type: questdb-questdb-datasource
    jsonData:
      server: localhost
      port: 8812
      username: admin
      tlsMode: disable
      # timeout: <seconds>
      # queryTimeout: <seconds>
      maxOpenConnections: 100
      maxIdleConnections: 100
      maxConnectionLifetime: 14400
    secureJsonData:
      password: quest
      # tlsCACert: <string>
```

If you are using QuestDB Enterprise and have enabled TLS, you would need to change
`tlsMode: require` in the example above.

### Per-user service accounts (memory limits)

> Requires QuestDB **Enterprise**. With the feature disabled (the default) the plugin
> behaves exactly as before and works against Open Source.

A single, shared data source can apply **per-Grafana-user memory limits** to the queries
each user runs. The data source still authenticates with one common login; when a query
runs, the plugin makes the QuestDB session assume a service account specific to the
requesting Grafana user (or shared by a group of users):

```sql
ASSUME SERVICE ACCOUNT <serviceAccount>;
```

Because an Enterprise service account can carry a memory limit, and a user assuming a
service account picks up that account's limit, this transparently caps the memory of that
user's queries. Grouping is expressed on the QuestDB side: map several Grafana users to
the same service account, and/or set the limit on a QuestDB group of service accounts.

The Grafana user is taken from the backend-verified identity (`PluginContext.User.Login`),
so it cannot be tampered with from the query payload. Matching is case-insensitive.
Unmapped users and backend-initiated queries (alerting, reporting) use the configured
default service account; if no default is set, they run as the base login. A mapping row
with a blank service account is ignored — that user falls back to the default rather than
running uncapped.

**QuestDB setup** (example: analysts capped at 2G, with `baseuser` as the data source login):

```sql
CREATE SERVICE ACCOUNT sa_analysts;
GRANT SELECT ON ALL TABLES TO sa_analysts;
ALTER SERVICE ACCOUNT sa_analysts SET MEMORY LIMIT 2G;
GRANT ASSUME SERVICE ACCOUNT sa_analysts TO baseuser;

-- defense in depth: also bound the base login
ALTER USER baseuser SET MEMORY LIMIT 256M;
```

**Grafana setup**: in the data source config, open **Per-user service accounts**, enable
the toggle, set a **Default service account**, and add **User mappings** (Grafana login →
service account). The same can be provisioned via `jsonData`:

```yaml
    jsonData:
      serviceAccountRoutingEnabled: true
      defaultServiceAccount: sa_default
      serviceAccountMappings:
        - grafanaUser: johndoe
          serviceAccount: sa_analysts
        - grafanaUser: ceo
          serviceAccount: sa_execs
```

This is resource governance, not a hard security boundary: the SQL editor lets a user run
arbitrary SQL, including `EXIT SERVICE ACCOUNT;`, so set a memory limit on the base login
too and grant it only the service accounts used here. Note that one connection pool is
created per active service account (each using the configured connection limits), so favor
groups over a unique account per user.

#### Per-group routing via OIDC groups (Okta)

> Builds on the feature above and is likewise **opt-in**. With no group mappings configured,
> behavior is exactly the per-user feature's.

When users log in through **OIDC / Generic OAuth** (e.g. Okta), the plugin can map a user's
**group** to a service account, so a memory limit on that account caps everyone in the group
without enumerating usernames. This requires **Forward OAuth Identity**
(`jsonData.oauthPassThru: true`) on the data source: Grafana then forwards the user's ID
token (as the `X-Id-Token` header) and the plugin reads the groups from it. The token is
injected by the Grafana server from the user's session, so — like the username — the group
identity cannot be forged from the query payload.

Resolution is most-specific-first: **user mapping → group mapping → default service
account**. When a user belongs to several mapped groups, the **first matching row (top-down)**
wins. Matching is case-insensitive. Groups are read from the `groups` claim by default;
override the claim name with `groupsClaim` if your IdP uses a different one.

**Okta**: add a `groups` claim to the OIDC app's **ID token** (Sign On → OpenID Connect ID
Token → Edit), scoped to the groups you map. **Grafana**: enable **Forward OAuth Identity**
on the data source, then under **Per-user service accounts** add **Group mappings** (group →
service account). Provisioned via `jsonData`:

```yaml
    jsonData:
      oauthPassThru: true                 # Forward OAuth Identity (core Grafana setting)
      serviceAccountRoutingEnabled: true
      defaultServiceAccount: sa_default
      groupsClaim: groups                 # optional; defaults to "groups"
      serviceAccountGroupMappings:
        - group: Analysts
          serviceAccount: sa_analysts
        - group: Execs
          serviceAccount: sa_execs
```

SAML logins do not mint an OIDC ID token, so this route is unavailable there; those users
fall back to username mappings and the default service account. The groups claim is read for
governance without verifying the token's signature or expiry, so do not treat it as a hard
security boundary (the `EXIT SERVICE ACCOUNT` caveat above still applies).

## Building queries

The query editor allows you to query QuestDB to return time series or
tabular data. Queries can contain macros which simplify syntax and allow for
dynamic parts.

### Time series

Time series visualization options are selectable after adding a `timestamp`
field type to your query. This field will be used as the timestamp. You can
select time series visualizations using the visualization options. Grafana
interprets timestamp rows without explicit time zone as UTC. Any column except
`time` is treated as a value column.

#### Multi-line time series

To create multi-line time series, the query must return at least 3 fields in
the following order:

- field 1: `timestamp` field with an alias of `time`
- field 2: value to group by
- field 3+: the metric values

For example:

```sql
SELECT pickup_datetime AS time, cab_type, avg(fare_amount) AS avg_fare_amount
FROM trips
GROUP BY cab_type, pickup_datetime
ORDER BY pickup_datetime
```

### Tables

Table visualizations will always be available for any valid QuestDB query.

### Macros

To simplify syntax and to allow for dynamic parts, like date range filters, the query can contain macros.

Here is an example of a query with a macro that will use Grafana's time filter:

```sql
SELECT desginated_timestamp, data_stuff
FROM test_data
WHERE $__timeFilter(desginated_timestamp)
```

| Macro                                          | Description                                                                                                                                                                         | Output example                                                                                          |
| ---------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| _$\_\_timeFilter(columnName)_                  | Replaced by a conditional that filters the data (using the provided column) based on the time range of the panel in seconds                                                         | `timestamp >= cast(1706263425598000 as timestamp) AND timestamp <= cast(1706285057560000 as timestamp)` |
| _$\_\_fromTime_                                | Replaced by the starting time of the range of the panel cast to timestamp                                                                                                           | `cast(1706263425598000 as timestamp)`                                                                   |
| _$\_\_toTime_                                  | Replaced by the ending time of the range of the panel cast to timestamp                                                                                                             | `cast(1706285057560000 as timestamp)`                                                                   |
| _$\_\_sampleByInterval_                        | Replaced by the interval followed by unit: d, h, s or T (millisecond). Example: 1d, 5h, 20s, 1T.                                                                                    | `20s` (20 seconds) , `1T` (1 millisecond)                                                               |
| _$\_\_conditionalAll(condition, $templateVar)_ | Replaced by the first parameter when the template variable in the second parameter does not select every value. Replaced by the 1=1 when the template variable selects every value. | `condition` or `1=1`                                                                                    |

The plugin also supports notation using braces {}. Use this notation when queries are needed inside parameters.

Additionally, Grafana has the built-in [`$__interval` macro][query-transform-data-query-options], which calculates an interval in seconds or milliseconds.
It shouldn't be used with SAMPLE BY because of time unit incompatibility, 1ms vs 1T (expected by QuestDB). Use `$__sampleByInterval` instead.

### Templates and variables

To add a new QuestDB query variable, refer to [Add a query
variable](https://grafana.com/docs/grafana/latest/variables/variable-types/add-query-variable/).

After creating a variable, you can use it in your QuestDB queries by using
[Variable syntax](https://grafana.com/docs/grafana/latest/variables/syntax/).
For more information about variables, refer to [Templates and
variables](https://grafana.com/docs/grafana/latest/variables/).

### Ad Hoc Filters

Ad hoc filters allow you to add key/value filters that are automatically added
to all metric queries that use the specified data source, without being
explicitly used in queries.

By default, Ad Hoc filters will be populated with all Tables and Columns. If
you have a default database defined in the Datasource settings, all Tables from
that database will be used to populate the filters. As this could be
slow/expensive, you can introduce a second variable to allow limiting the
Ad Hoc filters. It should be a `constant` type named `questdb_adhoc_query`
and can contain: a comma delimited list of tables to show only columns for one or more tables.

For more information on Ad Hoc filters, check the [Grafana
docs](https://grafana.com/docs/grafana/latest/variables/variable-types/add-ad-hoc-filters/)

#### Using a query for Ad Hoc filters

The constant `questdb_adhoc_query` also allows any valid QuestDB query. The
query results will be used to populate your ad-hoc filter's selectable filters.
You may choose to hide this variable from view as it serves no further purpose.

## Learn more

- Add [Annotations](https://grafana.com/docs/grafana/latest/dashboards/annotations/).
- Configure and use [Templates and variables](https://grafana.com/docs/grafana/latest/variables/).
- Add [Transformations](https://grafana.com/docs/grafana/latest/panels/transformations/).
- Set up alerting; refer to [Alerts overview](https://grafana.com/docs/grafana/latest/alerting/).
- Read the [Plugin guide](https://questdb.io/docs/third-party-tools/grafana/) on QuestDB website
