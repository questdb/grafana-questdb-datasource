package plugin

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
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
	// When routing is enabled connectionArgs is plugin-owned: stamp the resolved account,
	// or strip any client-supplied connectionArgs when the user resolves to no account, so
	// the service account stays server-resolved from a verified identity and a query payload
	// can never select its own account (and thus its own memory limit). The OIDC groups are
	// pulled from the forwarded ID token lazily — resolveServiceAccount calls groupsFn only
	// if it reaches the group step — so the (PII-bearing) token is never decoded for a user a
	// username mapping already covers, nor when no group mappings are configured.
	var connArgs json.RawMessage
	groupsFn := func() []string {
		return extractGroups(req.GetHTTPHeader(backend.OAuthIdentityIDTokenHeaderName), settings.GroupsClaim)
	}
	if sa := settings.resolveServiceAccountLazy(req.PluginContext.User, groupsFn); sa != "" {
		marshaled, err := json.Marshal(connectionArgs{ServiceAccount: sa})
		if err != nil {
			// Marshaling a single validated string should never fail; if it somehow does,
			// fall through with connArgs == nil so the client value is stripped (fail closed).
			log.DefaultLogger.Error("QuestDB failed to marshal connectionArgs", "error", err)
		} else {
			connArgs = marshaled
		}
	}
	for i := range req.Queries {
		req.Queries[i].JSON = withConnectionArgs(req.Queries[i].JSON, connArgs)
	}
	return ctx, req
}

// withConnectionArgs sets the "connectionArgs" field of a query JSON object to connArgs,
// preserving all other fields (rawSql, format, ...). A nil connArgs instead removes the
// field — this is how a client-supplied value is stripped when the user resolves to no
// service account (the field is left untouched when there is nothing to strip). On
// malformed (non-object) JSON it returns the input unchanged.
func withConnectionArgs(queryJSON, connArgs json.RawMessage) json.RawMessage {
	m := map[string]json.RawMessage{}
	// JSON `null` unmarshals into a nil map with no error; without the m == nil guard the
	// m["connectionArgs"] assignment below would panic ("assignment to entry in nil map").
	if err := json.Unmarshal(queryJSON, &m); err != nil || m == nil {
		return queryJSON
	}
	if connArgs == nil {
		if _, ok := m["connectionArgs"]; !ok {
			return queryJSON // nothing to strip; leave the bytes untouched
		}
		delete(m, "connectionArgs")
	} else {
		m["connectionArgs"] = connArgs
	}
	out, err := json.Marshal(m)
	if err != nil {
		return queryJSON
	}
	return out
}

// assumeProbeTimeout bounds the routed health-check probe so a slow or hung ASSUME on a
// misconfigured server cannot stall Save & Test indefinitely.
const assumeProbeTimeout = 30 * time.Second

// PostCheckHealth runs at Save & Test time, after sqlds has already verified base-login
// connectivity against the default pool. That base check never exercises service-account
// routing: it connects with no connectionArgs, so no ASSUME runs. A misconfiguration in
// the routing path — a malformed account name, a missing
// `GRANT ASSUME SERVICE ACCOUNT … TO <login>`, or a non-existent default account — would
// therefore pass Save & Test and only surface later as a failure on every routed query.
//
// To close that gap, when routing is enabled this:
//  1. validates every configured account name syntactically (cheap, no DB round-trip), and
//  2. opens a routed pool for the default account and pings it, so the ASSUME actually runs
//     against the live server.
//
// Per-user/per-group accounts are validated in step 1 but not live-probed: there is no
// specific requesting user at config time, so only the default account is exercised
// end-to-end. Returns nil (healthy) when routing is disabled or there is no default account
// to probe; a non-nil error result fails Save & Test with an actionable message.
func (h *QuestDB) PostCheckHealth(ctx context.Context, req *backend.CheckHealthRequest) *backend.CheckHealthResult {
	dsi := req.PluginContext.DataSourceInstanceSettings
	if dsi == nil {
		return nil
	}
	// Parse directly (rather than via LoadServiceAccountSettings) so a malformed routing block
	// is rejected here. A provisioned type mismatch unmarshals partially: the offending row is
	// dropped/blanked, or — when the enable flag itself fails to parse — routing silently reads
	// as "off". Either way the affected user would run on the uncapped base login with no
	// signal, so it must fail Save & Test. Checked before the enabled short-circuit precisely
	// to catch a mistyped enable flag. validateServiceAccountNames (below) cannot see a row the
	// parser already dropped, so the raw parse error is surfaced too.
	var settings Settings
	if err := applyServiceAccountSettings(&settings, dsi.JSONData); err != nil {
		return routingHealthError(fmt.Sprintf("could not parse routing configuration: %v", err))
	}
	if !settings.ServiceAccountRoutingEnabled {
		return nil
	}
	if err := settings.validateServiceAccountNames(); err != nil {
		return routingHealthError(err.Error())
	}
	sa := strings.TrimSpace(settings.DefaultServiceAccount)
	if sa == "" {
		// Only per-user/per-group mappings are configured; their names are validated above,
		// but there is no default account to exercise end-to-end here.
		return nil
	}
	msg, err := json.Marshal(connectionArgs{ServiceAccount: sa})
	if err != nil {
		return routingHealthError(err.Error())
	}
	db, err := h.Connect(ctx, *dsi, msg)
	if err != nil {
		return routingHealthError(err.Error())
	}
	defer db.Close()
	// PingContext forces a physical connection, which is what actually runs the ASSUME via
	// assumeServiceAccountConnector; a bare OpenDB is lazy and would prove nothing.
	probeCtx, cancel := context.WithTimeout(ctx, assumeProbeTimeout)
	defer cancel()
	if err := db.PingContext(probeCtx); err != nil {
		return routingHealthError(fmt.Sprintf(
			"cannot assume the default service account %q: %v; ensure it exists and that the data source login has been granted ASSUME SERVICE ACCOUNT %s",
			sa, err, sa))
	}
	return nil
}

// routingHealthError builds a failed health-check result tagged so the operator can tell the
// failure came from the service-account routing probe rather than base connectivity.
func routingHealthError(msg string) *backend.CheckHealthResult {
	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusError,
		Message: "Service-account routing: " + msg,
	}
}

// extractGroups decodes the groups claim from a forwarded OAuth/OIDC ID token (the
// X-Id-Token header that Grafana attaches when "Forward OAuth Identity" is on). The token
// is injected by the Grafana server from the user's session, so it is trustworthy for
// governance; per the design we read the claim WITHOUT verifying its signature or expiry
// (which also sidesteps Grafana occasionally forwarding an expired token). It returns nil
// — never an error — for a token that is absent, malformed, or whose claim is missing or
// is neither a JSON string nor an array of strings, so resolution falls through to the
// default account; a
// present-but-unusable token is logged at Debug (an absent one is the normal no-forwarding
// case and is not). The claim name defaults to "groups" (the Okta default) when not configured.
func extractGroups(idToken, claim string) []string {
	if idToken == "" {
		return nil
	}
	if claim == "" {
		claim = "groups"
	}
	// A JWT is header.payload.signature, each base64url-encoded; only the payload is needed.
	// A present-but-unusable token (e.g. an encrypted 5-segment JWE, which some Okta/Azure
	// setups issue) otherwise routes every group-mapped user to the default account silently;
	// log the fallback at Debug so it is diagnosable. The messages omit the token, payload, and
	// parse-error text, any of which can carry the token's contents.
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		log.DefaultLogger.Debug("QuestDB ignoring forwarded ID token for group routing: not a 3-segment JWT (encrypted JWE tokens are unsupported); using default account", "segments", len(parts))
		return nil
	}
	payload, err := decodeJWTSegment(parts[1])
	if err != nil {
		log.DefaultLogger.Debug("QuestDB ignoring forwarded ID token for group routing: payload is not valid base64url; using default account")
		return nil
	}
	var claims map[string]json.RawMessage
	if err := json.Unmarshal(payload, &claims); err != nil {
		log.DefaultLogger.Debug("QuestDB ignoring forwarded ID token for group routing: payload is not valid JSON; using default account")
		return nil
	}
	raw, ok := claims[claim]
	if !ok {
		log.DefaultLogger.Debug("QuestDB groups claim not present in forwarded ID token; using default account", "claim", claim)
		return nil
	}
	var groups []string
	if err := json.Unmarshal(raw, &groups); err == nil {
		return groups
	}
	// Some IdPs serialize a single group as a scalar string ("groups":"Analysts") rather than
	// a one-element array; accept that form too so those users still match a group mapping
	// instead of silently falling through to the default account.
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []string{single}
	}
	log.DefaultLogger.Debug("QuestDB groups claim is neither a string nor an array of strings; using default account", "claim", claim)
	return nil // claim present but not a string or string array
}

// decodeJWTSegment base64url-decodes a single JWT segment. JWT segments use canonical
// unpadded base64url; TrimRight tolerates accidentally-padded variants too.
func decodeJWTSegment(seg string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(seg, "="))
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
