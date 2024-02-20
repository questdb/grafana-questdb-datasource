import { E2ESelectors } from '@grafana/e2e-selectors';
export const Components = {
  ConfigEditor: {
    ServerAddress: {
      label: 'Server address',
      placeholder: 'localhost',
      tooltip: 'QuestdDB TCP server address',
    },
    ServerPort: {
      label: 'Server port',
      placeholder: `8812`,
      tooltip: 'QuestDB PG wire TCP port. Typically 8812.',
    },
    Username: {
      label: 'Username',
      placeholder: 'Username',
      tooltip: 'QuestDB username',
    },
    Password: {
      label: 'Password',
      placeholder: 'Password',
      tooltip: 'QuestDB password',
    },
    TLSCACert: {
      label: 'TLS/SSL Root Certificate',
      placeholder: 'CA Cert. Begins with -----BEGIN CERTIFICATE-----',
    },
    TLSClientCert: {
      label: 'TLS/SSL Client Certificate',
      placeholder: 'Client Cert. Begins with -----BEGIN CERTIFICATE-----',
    },
    TLSClientKey: {
      label: 'TLS/SSL Client Key',
      placeholder: 'Client Key. Begins with -----BEGIN RSA PRIVATE KEY-----',
    },

    TLSCACertFile: {
      label: 'TLS/SSL Root Certificate File',
      placeholder: 'If the selected TLS/SSL mode requires a server root certificate, provide the path to the file here.',
    },
    TLSClientCertFile: {
      label: 'TLS/SSL Client Certificate File',
      placeholder: 'To authenticate with an TLS/SSL client certificate, provide the path to the file here. Be sure that the file is readable by the user executing the grafana process.',
    },
    TLSClientKeyFile: {
      label: 'TLS/SSL Client Key File',
      placeholder: 'To authenticate with a client TLS/SSL certificate, provide the path to the corresponding key file here. Be sure that the file is only readable by the user executing the grafana process.'
    },

    Timeout: {
      label: 'Connect Timeout (seconds)',
      placeholder: '10',
      tooltip: 'Timeout in seconds for connection',
    },
    QueryTimeout: {
      label: 'Query Timeout (seconds)',
      placeholder: '60',
      tooltip: 'Timeout in seconds for read queries',
    },
    TlsMode: {
      label: 'TLS/SSL Mode',
      tooltip: 'This option determines whether or with what priority a secure TLS/SSL TCP/IP connection will be negotiated with the server',
      placeholder: "TLS/SSL Mode"
    },
    TlsMethod: {
      label: 'TLS/SSL Method',
      tooltip: 'This option determines how TLS/SSL certifications are configured. Selecting ' +
          '"File system path" will allow you to configure certificates by specifying paths to existing ' +
          'certificates on the local file system where Grafana is running. Be sure that the file is ' +
          'readable by the user executing the Grafana process. ' +
          'Selecting "Certificate content" will allow you to configure certificates by specifying its ' +
          'content. The content will be stored encrypted in Grafana\'s database.',
      placeholder: 'TLS/SSL Method'
    },
    SecureSocksProxy: {
      label: 'Enable Secure Socks Proxy',
      tooltip: 'Enable proxying the datasource connection through the secure socks proxy to a different network.',
    },
    MaxOpenConnections: {
      label: 'Max open',
      placeholder: '100',
      tooltip: 'Maximum number of open connections to the database.',
    },
    MaxIdleConnections: {
      label: 'Max idle',
      placeholder: '100',
      tooltip: 'Maximum number of idle connections.',
    },
    MaxConnectionLifetime: {
      label: 'Max lifetime',
      placeholder: '14400',
      tooltip: 'The maximum amount of time (in seconds) a connection may be reused. If set to 0, connections are reused forever.',
    },
  },
  QueryEditor: {
    CodeEditor: {
      input: () => '.monaco-editor textarea',
      container: 'data-testid-code-editor-container',
      Expand: 'data-testid-code-editor-expand-button',
    },
    Format: {
      label: 'Format',
      tooltip: 'Query Type',
      options: {
        AUTO: 'Auto',
        TABLE: 'Table',
        TIME_SERIES: 'Time Series',
      },
    },
    Types: {
      label: 'Query Type',
      tooltip: 'Query Type',
      options: {
        SQLEditor: 'SQL Editor',
        QueryBuilder: 'Query Builder',
      },
      switcher: {
        title: 'Are you sure?',
        body: 'Queries that are too complex for the Query Builder will be altered.',
        confirmText: 'Continue',
        dismissText: 'Cancel',
      },
      cannotConvert: {
        title: 'Cannot convert',
        confirmText: 'Yes',
      },
    },
    QueryBuilder: {
      TYPES: {
        label: 'Query type',
        tooltip: 'Query type',
        options: {
          LIST: 'Table',
          AGGREGATE: 'Aggregate',
          TREND: 'Time Series',
        },
      },
      DATABASE: {
        label: 'Database',
        tooltip: 'QuestDB database to query from',
      },
      FROM: {
        label: 'Table',
        tooltip: 'QuestDB table to query from',
      },
      SELECT: {
        label: 'Fields',
        tooltipTable: 'List of fields to show',
        tooltipAggregate: `List of metrics to show. Use any of the given aggregation along with the field`,
        ALIAS: {
          label: 'as',
          tooltip: 'alias',
        },
        AddLabel: 'Field',
        RemoveLabel: '',
      },
      AGGREGATES: {
        label: 'Aggregates',
        tooltipTable: 'Aggregate functions to use',
        tooltipAggregate: `Aggregate functions to use`,
        ALIAS: {
          label: 'as',
          tooltip: 'alias',
        },
        AddLabel: 'Aggregate',
        RemoveLabel: '',
      },
      WHERE: {
        label: 'Filters',
        tooltip: `List of filters`,
        AddLabel: 'Filter',
        RemoveLabel: '',
      },
      GROUP_BY: {
        label: 'Group by',
        tooltip: 'Group the results by specific field',
      },
      SAMPLE_BY: {
        label: 'Sample by keys',
        tooltip: 'Sample the results by specific field',
      },
      FILL: {
        label: 'Sample by fill',
        tooltip: 'Fill missing aggregate columns using a strategy or constant value',
      },
      ORDER_BY: {
        label: 'Order by',
        tooltip: 'Order by field',
        AddLabel: 'Order by',
        RemoveLabel: '',
      },
      LIMIT: {
        label: 'Limit',
        tooltip: 'Number of records/results to show.',
      },
      PARTITION_BY: {
        label: 'Latest on partition by',
        tooltip: 'List of fields to partition by with LATEST ON clause',
      },
      DESIGNATED_TIMESTAMP: {
        label: 'Designated timestamp',
        tooltip: 'Select table\'s designated timestamp',
      },
      ALIGN_TO: {
        label: 'Align to',
        tooltip: 'Align sampling to first observation or calendar dates',
      },
      CALENDAR_OFF_TZ: {
        label: 'Calendar offset/timezone',
        tooltip: 'Align sampling to calendar offset or timezone',
      },
      PREVIEW: {
        label: 'SQL Preview',
        tooltip: 'SQL Preview. You can safely switch to SQL Editor to customize the generated query',
      },
    },
  },
};
export const selectors: { components: E2ESelectors<typeof Components> } = {
  components: Components,
};
