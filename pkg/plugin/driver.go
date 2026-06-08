package plugin

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"golang.org/x/net/proxy"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/build"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/grafana/grafana-plugin-sdk-go/data/sqlutil"
	"github.com/grafana/sqlds/v4"
	_ "github.com/lib/pq"
	"github.com/pkg/errors"
	"github.com/questdb/grafana-questdb-datasource/pkg/converters"
	"github.com/questdb/grafana-questdb-datasource/pkg/macros"
)

// QuestDB defines how to connect to a QuestDB datasource
type QuestDB struct{}

func getClientVersion(ctx context.Context) string {
	result := ""

	version := backend.UserAgentFromContext(ctx).GrafanaVersion()
	if version != "" {
		result += fmt.Sprintf("grafana:%s", version)
	}

	if info, err := build.GetBuildInfo(); err == nil {
		if version != "" {
			version += ";"
		}
		result += fmt.Sprintf("questdb-questdb-datasource:%s", info.Version)
	}

	return result
}

func (h *QuestDB) Connect(ctx context.Context, config backend.DataSourceInstanceSettings, message json.RawMessage) (*sql.DB, error) {
	settings, err := LoadSettings(config)
	if err != nil {
		log.DefaultLogger.Debug("Invalid settings found", "error", err)
		return nil, err
	}
	connstr, err := GenerateConnectionString(settings, getClientVersion(ctx))
	if err != nil {
		log.DefaultLogger.Error("QuestDB connection string generation failed", "error", err)
		return nil, err
	}

	log.DefaultLogger.Debug("QuestDB connection string generated",
		"server", settings.Server,
		"port", settings.Port,
		"tlsMode", settings.TlsMode)

	connector, err := pq.NewConnector(connstr)
	if err != nil {
		log.DefaultLogger.Error("QuestDB connector creation failed", "error", err)
		return nil, fmt.Errorf("QuestDB connector creation failed")
	}

	proxyClient, err := config.ProxyClient(ctx)
	if err != nil {
		log.DefaultLogger.Error("QuestDB proxy client creation failed", "error", err)
		return nil, err
	}

	log.DefaultLogger.Debug("QuestDB proxy status",
		"enableSecureSocksProxy", settings.EnableSecureSocksProxy,
		"proxyClientNil", proxyClient == nil,
		"server", settings.Server,
		"port", settings.Port)

	if proxyClient != nil {
		if proxyClient.SecureSocksProxyEnabled() {
			log.DefaultLogger.Info("QuestDB secure socks proxy is enabled")
			dialer, err := proxyClient.NewSecureSocksProxyContextDialer()
			if err != nil {
				log.DefaultLogger.Error("QuestDB secure socks proxy dialer creation failed", "error", err)
				return nil, err
			}
			connector.Dialer(&postgresProxyDialer{d: dialer})
			log.DefaultLogger.Debug("QuestDB secure socks proxy dialer configured")
		} else {
			log.DefaultLogger.Debug("QuestDB secure socks proxy is not enabled by SDK",
				"datasourceHasFlag", settings.EnableSecureSocksProxy)
		}
	}

	// When service-account routing is enabled, sqlds creates one pool per distinct
	// connectionArgs. The message carries the service account stamped by MutateQueryData;
	// we wrap the connector so each new physical connection assumes that account exactly
	// once. The account's memory limit then applies to every query on this pool.
	var conn driver.Connector = connector
	if settings.ServiceAccountRoutingEnabled && len(message) > 0 {
		var args connectionArgs
		if err := json.Unmarshal(message, &args); err == nil && args.ServiceAccount != "" {
			stmt, err := buildAssumeStatement(args.ServiceAccount)
			if err != nil {
				log.DefaultLogger.Error("QuestDB invalid service account name", "error", err)
				return nil, err
			}
			conn = &assumeServiceAccountConnector{base: connector, stmt: stmt}
			log.DefaultLogger.Debug("QuestDB service account routing enabled for pool",
				"serviceAccount", args.ServiceAccount)
		}
	}

	db := sql.OpenDB(conn)
	db.SetMaxOpenConns(int(settings.MaxOpenConnections))
	db.SetMaxIdleConns(int(settings.MaxIdleConnections))
	db.SetConnMaxLifetime(time.Duration(settings.MaxConnectionLifetime) * time.Second)

	log.DefaultLogger.Debug("Connection settings", "max open", int(settings.MaxOpenConnections),
		"max idle", int(settings.MaxIdleConnections),
		"max lifetime", time.Duration(settings.MaxConnectionLifetime)*time.Second)

	log.DefaultLogger.Info("Successfully connected to QuestDB")
	return db, nil
}

func GenerateConnectionString(settings Settings, version string) (string, error) {
	connStr := fmt.Sprintf("user='%s' password='%s' host='%s' dbname='%s'",
		escape(settings.Username), escape(settings.Password), escape(settings.Server), "qdb")

	if settings.Port > 0 {
		connStr += fmt.Sprintf(" port=%d", settings.Port)
	}

	if len(version) > 0 {
		connStr += fmt.Sprintf(" application_name='%s'", version)
	}

	if settings.Timeout > 0 {
		t := strconv.Itoa(int(settings.Timeout))
		if i, err := strconv.Atoi(t); err == nil && i > -1 {
			connStr += fmt.Sprintf(" connect_timeout=%d", i)
		}
	}

	if settings.TlsMode != "disable" &&
		settings.TlsMode != "require" &&
		settings.TlsMode != "verify-ca" &&
		settings.TlsMode != "verify-full" {
		return "", errors.New(fmt.Sprintf("invalid tls mode: %s", settings.TlsMode))
	}
	mode := settings.TlsMode
	connStr += fmt.Sprintf(" sslmode='%s'", escape(mode))

	if mode != "disable" {
		if settings.ConfigurationMethod == "file-content" {
			connStr += " sslinline=true"

			// Attach root certificate if provided
			if settings.TlsCACert != "" {
				log.DefaultLogger.Debug("Setting server root certificate", "tlsRootCert", settings.TlsCACert)
				connStr += fmt.Sprintf(" sslrootcert='%s'", escape(settings.TlsCACert))
			}

			// Attach client certificate and key if both are provided
			if settings.TlsClientCert != "" && settings.TlsClientKey != "" {
				log.DefaultLogger.Debug("Setting TLS/SSL client auth", "tlsCert", settings.TlsClientCert, "tlsKey", settings.TlsClientKey)
				connStr += fmt.Sprintf(" sslcert='%s' sslkey='%s'", escape(settings.TlsClientCert), escape(settings.TlsClientKey))
			} else if settings.TlsClientCert != "" || settings.TlsClientKey != "" {
				return "", fmt.Errorf("TLS/SSL client certificate and key must both be specified")
			} else {
				//HACK: sslinline fails if client key or cert are empty, so we use sample ones [they're ignored by qdb anyway]
				tlsClientCert := `
-----BEGIN CERTIFICATE-----
MIICEjCCAXsCAg36MA0GCSqGSIb3DQEBBQUAMIGbMQswCQYDVQQGEwJKUDEOMAwG
A1UECBMFVG9reW8xEDAOBgNVBAcTB0NodW8ta3UxETAPBgNVBAoTCEZyYW5rNERE
MRgwFgYDVQQLEw9XZWJDZXJ0IFN1cHBvcnQxGDAWBgNVBAMTD0ZyYW5rNEREIFdl
YiBDQTEjMCEGCSqGSIb3DQEJARYUc3VwcG9ydEBmcmFuazRkZC5jb20wHhcNMTIw
ODIyMDUyNjU0WhcNMTcwODIxMDUyNjU0WjBKMQswCQYDVQQGEwJKUDEOMAwGA1UE
CAwFVG9reW8xETAPBgNVBAoMCEZyYW5rNEREMRgwFgYDVQQDDA93d3cuZXhhbXBs
ZS5jb20wXDANBgkqhkiG9w0BAQEFAANLADBIAkEAm/xmkHmEQrurE/0re/jeFRLl
8ZPjBop7uLHhnia7lQG/5zDtZIUC3RVpqDSwBuw/NTweGyuP+o8AG98HxqxTBwID
AQABMA0GCSqGSIb3DQEBBQUAA4GBABS2TLuBeTPmcaTaUW/LCB2NYOy8GMdzR1mx
8iBIu2H6/E2tiY3RIevV2OW61qY2/XRQg7YPxx3ffeUugX9F4J/iPnnu1zAxxyBy
2VguKv4SWjRFoRkIfIlHX0qVviMhSlNy2ioFLy7JcPZb+v3ftDGywUqcBiVDoea0
Hn+GmxZA
-----END CERTIFICATE-----
`
				tlsClientKey := `
-----BEGIN RSA PRIVATE KEY-----
MIIBOwIBAAJBAJv8ZpB5hEK7qxP9K3v43hUS5fGT4waKe7ix4Z4mu5UBv+cw7WSF
At0Vaag0sAbsPzU8Hhsrj/qPABvfB8asUwcCAwEAAQJAG0r3ezH35WFG1tGGaUOr
QA61cyaII53ZdgCR1IU8bx7AUevmkFtBf+aqMWusWVOWJvGu2r5VpHVAIl8nF6DS
kQIhAMjEJ3zVYa2/Mo4ey+iU9J9Vd+WoyXDQD4EEtwmyG1PpAiEAxuZlvhDIbbce
7o5BvOhnCZ2N7kYb1ZC57g3F+cbJyW8CIQCbsDGHBto2qJyFxbAO7uQ8Y0UVHa0J
BO/g900SAcJbcQIgRtEljIShOB8pDjrsQPxmI1BLhnjD1EhRSubwhDw5AFUCIQCN
A24pDtdOHydwtSB5+zFqFLfmVZplQM/g5kb4so70Yw==
-----END RSA PRIVATE KEY-----
`
				connStr += fmt.Sprintf(" sslcert='%s' sslkey='%s'", escape(tlsClientCert), escape(tlsClientKey))
			}

		} else if settings.ConfigurationMethod != "" {
			return "", errors.New(fmt.Sprintf("invalid ssl configuration method: %s", settings.ConfigurationMethod))
		}
	}

	log.DefaultLogger.Debug("Generated QuestDB connection string successfully")
	return connStr, nil
}

// escape single quotes and backslashes in Postgres connection string parameters.
func escape(input string) string {
	return strings.ReplaceAll(strings.ReplaceAll(input, `\`, `\\`), "'", `\'`)
}

func (h *QuestDB) Converters() []sqlutil.Converter {
	return converters.QdbConverters
}

// Macros returns list of macro functions convert the macros of raw query
func (h *QuestDB) Macros() sqlds.Macros {
	return map[string]sqlds.MacroFunc{
		"fromTime":         macros.FromTimeFilter,
		"toTime":           macros.ToTimeFilter,
		"timeFilter":       macros.TimeFilter,
		"sampleByInterval": macros.SampleByInterval,
	}
}

func (h *QuestDB) Settings(ctx context.Context, config backend.DataSourceInstanceSettings) sqlds.DriverSettings {
	settings, err := LoadSettings(config)
	timeout := 60
	if err == nil {
		t, err := strconv.Atoi(strconv.FormatInt(settings.QueryTimeout, 10))
		if err == nil {
			timeout = t
		}
	}
	return sqlds.DriverSettings{
		Timeout: time.Second * time.Duration(timeout),
		FillMode: &data.FillMissing{
			Mode: data.FillModeNull,
		},
	}
}

func (h *QuestDB) MutateQuery(ctx context.Context, req backend.DataQuery) (context.Context, backend.DataQuery) {
	var dataQuery struct {
		Meta struct {
			TimeZone string `json:"timezone"`
		} `json:"meta"`
		Format int `json:"format"`
	}

	if err := json.Unmarshal(req.JSON, &dataQuery); err != nil {
		return ctx, req
	}
	if dataQuery.Meta.TimeZone == "" {
		return ctx, req
	}
	return ctx, req
}

// connectionArgs is the per-query routing payload carried in the query's connectionArgs
// field. sqlds keys a separate connection pool per distinct connectionArgs value, which
// is how we get one pool (and thus one ASSUME) per service account.
type connectionArgs struct {
	ServiceAccount string `json:"serviceAccount"`
}

// MutateQueryData resolves the requesting Grafana user to a service account (when
// routing is enabled) and stamps it into every query's connectionArgs, so sqlds routes
// the queries to the matching per-service-account connection pool. When routing is off,
// or the user resolves to no service account, the request is returned unchanged and the
// queries run on the default (base login) pool.
func (h *QuestDB) MutateQueryData(ctx context.Context, req *backend.QueryDataRequest) (context.Context, *backend.QueryDataRequest) {
	dsi := req.PluginContext.DataSourceInstanceSettings
	if dsi == nil {
		return ctx, req
	}
	// Resolve routing from non-secret jsonData only; this must not depend on credentials
	// being present in the request's PluginContext.
	settings := LoadServiceAccountSettings(*dsi)
	if !settings.ServiceAccountRoutingEnabled {
		return ctx, req
	}
	sa := settings.resolveServiceAccount(req.PluginContext.User)
	if sa == "" {
		return ctx, req // base pool, no ASSUME
	}
	connArgs, err := json.Marshal(connectionArgs{ServiceAccount: sa})
	if err != nil {
		log.DefaultLogger.Error("QuestDB failed to marshal connectionArgs", "error", err)
		return ctx, req
	}
	for i := range req.Queries {
		req.Queries[i].JSON = withConnectionArgs(req.Queries[i].JSON, connArgs)
	}
	return ctx, req
}

// withConnectionArgs merges "connectionArgs": <connArgs> into a query JSON object,
// preserving existing fields (rawSql, format, ...). On malformed JSON it returns the
// input unchanged.
func withConnectionArgs(queryJSON, connArgs json.RawMessage) json.RawMessage {
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal(queryJSON, &m); err != nil {
		return queryJSON
	}
	m["connectionArgs"] = connArgs
	out, err := json.Marshal(m)
	if err != nil {
		return queryJSON
	}
	return out
}

// MutateResponse For any view other than traces we convert FieldTypeNullableJSON to string
func (h *QuestDB) MutateResponse(ctx context.Context, res data.Frames) (data.Frames, error) {
	return res, nil
}

// assumeServiceAccountConnector wraps a driver.Connector so that every new physical
// connection runs `ASSUME SERVICE ACCOUNT <sa>` exactly once before it is used. Because
// sqlds keeps one pool per service account, the ASSUME runs per connection (not per
// query) and never leaks between Grafana users; the account's memory limit then applies
// to every query on the pool.
type assumeServiceAccountConnector struct {
	base driver.Connector
	stmt string
}

func (c *assumeServiceAccountConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.base.Connect(ctx)
	if err != nil {
		return nil, err
	}
	execer, ok := conn.(driver.ExecerContext)
	if !ok {
		_ = conn.Close()
		return nil, fmt.Errorf("connection does not support ExecContext; cannot ASSUME SERVICE ACCOUNT")
	}
	if _, err := execer.ExecContext(ctx, c.stmt, nil); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to assume service account: %w", err)
	}
	return conn, nil
}

func (c *assumeServiceAccountConnector) Driver() driver.Driver { return c.base.Driver() }

// postgresProxyDialer implements the postgres dialer using a proxy dialer, as their functions differ slightly
type postgresProxyDialer struct {
	d proxy.Dialer
}

// Dial uses the normal proxy dial function with the updated dialer
func (p *postgresProxyDialer) Dial(network, addr string) (c net.Conn, err error) {
	return p.d.Dial(network, addr)
}

// DialTimeout uses the normal postgres dial timeout function with the updated dialer
func (p *postgresProxyDialer) DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return p.d.(proxy.ContextDialer).DialContext(ctx, network, address)
}
