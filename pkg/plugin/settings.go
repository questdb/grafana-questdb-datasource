package plugin

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// Settings - data loaded from grafana settings database
type Settings struct {
	Server   string `json:"server,omitempty"`
	Port     int64  `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"-,omitempty"`

	TlsCACert     string
	TlsClientCert string
	TlsClientKey  string

	Timeout                int64  `json:"timeout,omitempty"`
	QueryTimeout           int64  `json:"queryTimeout,omitempty"`
	MaxOpenConnections     int64  `json:"maxOpenConnections,omitempty"`
	MaxIdleConnections     int64  `json:"maxIdleConnections,omitempty"`
	MaxConnectionLifetime  int64  `json:"maxConnectionLifetime,omitempty"`
	TimeInterval           string `json:"timeInterval,omitempty"`
	EnableSecureSocksProxy bool   `json:"enableSecureSocksProxy,omitempty"`

	TlsMode             string `json:"tlsMode"`
	ConfigurationMethod string `json:"tlsConfigurationMethod"`

	TlsClientCertFile string `json:"tlsClientCertFile"`
	TlsClientKeyFile  string `json:"tlsClientKeyFile"`

	// Per-user service-account routing (optional; QuestDB Enterprise only).
	ServiceAccountRoutingEnabled bool                         `json:"serviceAccountRoutingEnabled,omitempty"`
	DefaultServiceAccount        string                       `json:"defaultServiceAccount,omitempty"`
	ServiceAccountMappings       []ServiceAccountMapping      `json:"serviceAccountMappings,omitempty"`
	ServiceAccountGroupMappings  []ServiceAccountGroupMapping `json:"serviceAccountGroupMappings,omitempty"`
	// GroupsClaim is the ID-token claim that holds the user's groups; defaults to "groups".
	GroupsClaim string `json:"groupsClaim,omitempty"`
}

type CustomSetting struct {
	Setting string `json:"setting"`
	Value   string `json:"value"`
}

// ServiceAccountMapping maps a Grafana user login to a QuestDB service account.
// Several users may point at the same service account to form a "group".
type ServiceAccountMapping struct {
	GrafanaUser    string `json:"grafanaUser"`
	ServiceAccount string `json:"serviceAccount"`
}

// ServiceAccountGroupMapping maps an OAuth/OIDC group (e.g. an Okta group carried in the
// forwarded ID token's groups claim) to a QuestDB service account. Several groups may
// point at the same service account.
type ServiceAccountGroupMapping struct {
	Group          string `json:"group"`
	ServiceAccount string `json:"serviceAccount"`
}

func (settings *Settings) isValid() (err error) {
	if settings.Server == "" {
		return ErrorMessageInvalidServerName
	}
	if settings.Port <= 0 {
		return ErrorMessageInvalidPort
	}
	if len(settings.Username) == 0 {
		return ErrorMessageInvalidUserName
	}
	if len(settings.Password) == 0 {
		return ErrorMessageInvalidPassword
	}
	return nil
}

// LoadSettings will read and validate Settings from the DataSourceConfig
func LoadSettings(config backend.DataSourceInstanceSettings) (settings Settings, err error) {
	var jsonData map[string]interface{}
	if err := json.Unmarshal(config.JSONData, &jsonData); err != nil {
		return settings, fmt.Errorf("%s: %w", err.Error(), ErrorMessageInvalidJSON)
	}

	if jsonData["server"] != nil {
		settings.Server = jsonData["server"].(string)
	}
	if jsonData["port"] != nil {
		if port, ok := jsonData["port"].(string); ok {
			settings.Port, err = strconv.ParseInt(port, 0, 64)
			if err != nil {
				return settings, fmt.Errorf("could not parse port value: %w", err)
			}
		} else {
			settings.Port = int64(jsonData["port"].(float64))
		}
	}
	if jsonData["username"] != nil {
		settings.Username = jsonData["username"].(string)
	}

	if jsonData["timeout"] != nil {
		if val, ok := jsonData["timeout"].(string); ok {
			timeout, err := strconv.ParseInt(val, 0, 64)
			if err != nil {
				return settings, fmt.Errorf("could not parse timeout value: %w", err)
			}
			settings.Timeout = timeout
		}
		if val, ok := jsonData["timeout"].(float64); ok {
			settings.Timeout = int64(val)
		}
	}
	if jsonData["queryTimeout"] != nil {
		if val, ok := jsonData["queryTimeout"].(int64); ok {
			settings.QueryTimeout = val
		}
		if val, ok := jsonData["queryTimeout"].(float64); ok {
			settings.QueryTimeout = int64(val)
		}
	}

	if strings.TrimSpace(strconv.FormatInt(settings.QueryTimeout, 10)) == "" {
		settings.QueryTimeout = 60
	}
	password, ok := config.DecryptedSecureJSONData["password"]
	if ok {
		settings.Password = password
	}
	tlsCACert, ok := config.DecryptedSecureJSONData["tlsCACert"]
	if ok {
		settings.TlsCACert = tlsCACert
	}
	tlsClientCert, ok := config.DecryptedSecureJSONData["tlsClientCert"]
	if ok {
		settings.TlsClientCert = tlsClientCert
	}
	tlsClientKey, ok := config.DecryptedSecureJSONData["tlsClientKey"]
	if ok {
		settings.TlsClientKey = tlsClientKey
	}

	if jsonData["tlsConfigurationMethod"] != nil {
		settings.ConfigurationMethod = jsonData["tlsConfigurationMethod"].(string)
	}
	if jsonData["tlsMode"] != nil {
		settings.TlsMode = jsonData["tlsMode"].(string)
	}

	if jsonData["tlsClientCertFile"] != nil {
		settings.TlsClientCertFile = jsonData["tlsClientCertFile"].(string)
	}
	if jsonData["tlsClientKeyFile"] != nil {
		settings.TlsClientKeyFile = jsonData["tlsClientKeyFile"].(string)
	}

	if jsonData["enableSecureSocksProxy"] != nil {
		settings.EnableSecureSocksProxy = jsonData["enableSecureSocksProxy"].(bool)
	}

	if jsonData["maxOpenConnections"] != nil {
		if maxOpenConnections, ok := jsonData["maxOpenConnections"].(string); ok {
			settings.MaxOpenConnections, err = strconv.ParseInt(maxOpenConnections, 0, 64)
			if err != nil {
				return settings, fmt.Errorf("could not parse maxOpenConnections value: %w", err)
			}
		} else {
			settings.MaxOpenConnections = int64(jsonData["maxOpenConnections"].(float64))
		}
	}

	if jsonData["maxIdleConnections"] != nil {
		if maxIdleConnections, ok := jsonData["maxIdleConnections"].(string); ok {
			settings.MaxIdleConnections, err = strconv.ParseInt(maxIdleConnections, 0, 64)
			if err != nil {
				return settings, fmt.Errorf("could not parse maxIdleConnections value: %w", err)
			}
		} else {
			settings.MaxIdleConnections = int64(jsonData["maxIdleConnections"].(float64))
		}
	}

	if jsonData["maxConnectionLifetime"] != nil {
		if maxConnectionLifetime, ok := jsonData["maxConnectionLifetime"].(string); ok {
			settings.MaxConnectionLifetime, err = strconv.ParseInt(maxConnectionLifetime, 0, 64)
			if err != nil {
				return settings, fmt.Errorf("could not parse maxConnectionLifetime value: %w", err)
			}
		} else {
			settings.MaxConnectionLifetime = int64(jsonData["maxConnectionLifetime"].(float64))
		}
	}

	if jsonData["timeInterval"] != nil {
		if timeInterval, ok := jsonData["timeInterval"].(string); ok {
			settings.TimeInterval = timeInterval
		}
	}

	// Service-account routing fields, read from the jsonData map already parsed above rather
	// than unmarshaling config.JSONData a second time. The credential-free query path
	// (LoadServiceAccountSettings) and the config-time validator (PostCheckHealth) instead
	// decode these from raw bytes via applyServiceAccountSettings, which additionally reports
	// the type mismatches Save & Test must surface; here a partial parse just degrades safely.
	settings.ServiceAccountRoutingEnabled, _ = jsonData["serviceAccountRoutingEnabled"].(bool)
	settings.DefaultServiceAccount, _ = jsonData["defaultServiceAccount"].(string)
	settings.GroupsClaim, _ = jsonData["groupsClaim"].(string)
	redecode(jsonData["serviceAccountMappings"], &settings.ServiceAccountMappings)
	redecode(jsonData["serviceAccountGroupMappings"], &settings.ServiceAccountGroupMappings)

	return settings, settings.isValid()
}

// redecode re-encodes an already-parsed JSON value (a sub-tree of the jsonData map) and
// decodes it into dst. LoadSettings uses it to populate the typed routing slices from the
// map it has already parsed, so config.JSONData is not unmarshaled a second time. A nil
// value or any error leaves dst at its zero value; LoadSettings tolerates a partial routing
// parse (PostCheckHealth reports a malformed block separately).
func redecode(v interface{}, dst interface{}) {
	if v == nil {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	_ = json.Unmarshal(b, dst)
}

// applyServiceAccountSettings parses the (non-secret) service-account routing fields from
// jsonData onto settings and returns the json.Unmarshal error, if any. The per-query path
// ignores that error: a partial parse degrades safely (an unparseable mapping row drops its
// account and the affected user falls through toward the base login). Config-time callers
// (PostCheckHealth) surface it instead, so a provisioned type mismatch — e.g. a numeric
// serviceAccount, or a quoted boolean for the enable flag — fails Save & Test rather than
// silently mis-routing. A whole-document syntax error cannot occur via LoadSettings (it has
// already parsed the same bytes); the realistic error is exactly such a field type mismatch.
func applyServiceAccountSettings(settings *Settings, jsonData []byte) error {
	var sa struct {
		ServiceAccountRoutingEnabled bool                         `json:"serviceAccountRoutingEnabled"`
		DefaultServiceAccount        string                       `json:"defaultServiceAccount"`
		ServiceAccountMappings       []ServiceAccountMapping      `json:"serviceAccountMappings"`
		ServiceAccountGroupMappings  []ServiceAccountGroupMapping `json:"serviceAccountGroupMappings"`
		GroupsClaim                  string                       `json:"groupsClaim"`
	}
	err := json.Unmarshal(jsonData, &sa)
	settings.ServiceAccountRoutingEnabled = sa.ServiceAccountRoutingEnabled
	settings.DefaultServiceAccount = sa.DefaultServiceAccount
	settings.ServiceAccountMappings = sa.ServiceAccountMappings
	settings.ServiceAccountGroupMappings = sa.ServiceAccountGroupMappings
	settings.GroupsClaim = sa.GroupsClaim
	return err
}

// LoadServiceAccountSettings parses only the service-account routing fields. Unlike
// LoadSettings it does not require valid credentials, so it is safe to call on the query
// path (MutateQueryData): routing config is non-secret jsonData and is resolved before a
// connection is established, independent of whether secrets are present in the request.
func LoadServiceAccountSettings(config backend.DataSourceInstanceSettings) Settings {
	var settings Settings
	_ = applyServiceAccountSettings(&settings, config.JSONData)
	return settings
}

// resolveServiceAccount maps the requesting Grafana user (and their OIDC groups, when
// available) to a QuestDB service account. Precedence is most-specific-first:
//
//  1. username mapping  — exact (case-insensitive) match on user.Login
//  2. group mapping     — first configured mapping whose group is in groups (case-insensitive)
//  3. default service account
//
// It returns "" when none of the above yields a name (the query then runs as the base
// login). A nil user (backend-initiated requests such as alerting/reporting) and an empty
// groups slice both simply skip steps 1–2 and fall through to the default.
//
// Mappings with a blank service account are skipped: a half-filled config row must not
// silently make a mapped user bypass the cap by dropping to the base login — such a user
// falls through to the next step instead. Returned names are whitespace-trimmed.
func (settings *Settings) resolveServiceAccount(user *backend.User, groups []string) string {
	// 1. Username mapping (most specific). Both sides are whitespace-trimmed before the
	// case-insensitive compare, matching the group path below, so an operator's stray
	// space in a mapping row does not silently prevent a match.
	var login string
	if user != nil {
		login = strings.TrimSpace(user.Login)
	}
	if login != "" {
		for _, m := range settings.ServiceAccountMappings {
			sa := strings.TrimSpace(m.ServiceAccount)
			if sa == "" {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(m.GrafanaUser), login) {
				return sa
			}
		}
	}
	// 2. Group mapping. The plugin cannot see QuestDB-side limits to pick "most
	// restrictive", so for a user in several mapped groups the first matching row in
	// config order wins — a deterministic, operator-controlled tie-break.
	if len(groups) > 0 {
		for _, gm := range settings.ServiceAccountGroupMappings {
			group := strings.TrimSpace(gm.Group)
			sa := strings.TrimSpace(gm.ServiceAccount)
			if group == "" || sa == "" {
				continue
			}
			if containsFold(groups, group) {
				return sa
			}
		}
	}
	// 3. Default.
	return strings.TrimSpace(settings.DefaultServiceAccount) // "" when unset/blank
}

// containsFold reports whether needle equals any element of haystack, case-insensitively
// and ignoring surrounding whitespace on both sides.
func containsFold(haystack []string, needle string) bool {
	for _, s := range haystack {
		if strings.EqualFold(strings.TrimSpace(s), needle) {
			return true
		}
	}
	return false
}

// forbiddenServiceAccountNameChars mirrors the punctuation that QuestDB Enterprise rejects
// in entity names (server-side AccessListUtils.validateEntityName). We deliberately mirror
// QuestDB's denylist rather than impose a stricter allowlist, so the plugin accepts exactly
// the names QuestDB itself accepts (e.g. email-style `john.doe@mail.com`, or names with
// spaces). Crucially the set includes '"' and '\\', so the name remains injection-safe when
// embedded as the quoted identifier in `ASSUME SERVICE ACCOUNT "<sa>"` below.
const forbiddenServiceAccountNameChars = `?,'"\/:)(+*%~`

// validateServiceAccountName reports whether sa is a valid QuestDB service-account name,
// mirroring QuestDB's server-side rule: reject a fixed set of punctuation plus control
// characters (C0 0x00–0x0F, DEL, and the UTF-8 BOM — matching QuestDB exactly). The caller
// trims surrounding whitespace and treats "" as a no-op, so sa is expected non-empty here.
// QuestDB's configurable max name length is intentionally NOT enforced client-side (the
// server's value is unknown to the plugin); an over-long name is caught by the live ASSUME
// health probe instead.
func validateServiceAccountName(sa string) error {
	if sa == "" {
		return fmt.Errorf("service account name cannot be empty")
	}
	for _, r := range sa {
		if r <= 0x0f || r == 0x7f || r == 0xfeff || strings.ContainsRune(forbiddenServiceAccountNameChars, r) {
			return fmt.Errorf("invalid character %q in service account name %q", r, sa)
		}
	}
	return nil
}

// validateServiceAccountNames checks every configured service-account name (the default
// plus all user- and group-mapping targets) so a typo or unsupported character is reported
// at Save & Test time instead of silently failing every routed query later. Blank names
// are skipped: by design a blank default/mapping means "fall through to the next step",
// not an error. It returns the first offending name's error.
func (settings *Settings) validateServiceAccountNames() error {
	names := make([]string, 0, 1+len(settings.ServiceAccountMappings)+len(settings.ServiceAccountGroupMappings))
	names = append(names, settings.DefaultServiceAccount)
	for _, m := range settings.ServiceAccountMappings {
		names = append(names, m.ServiceAccount)
	}
	for _, gm := range settings.ServiceAccountGroupMappings {
		names = append(names, gm.ServiceAccount)
	}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if err := validateServiceAccountName(n); err != nil {
			return err
		}
	}
	return nil
}

// buildAssumeStatement returns the statement run on each new connection for the given
// service account. It returns ("", nil) when sa is empty, and an error when the name
// fails validation. There is no trailing ';' — it is executed as a single statement
// via ExecContext.
func buildAssumeStatement(sa string) (string, error) {
	if sa == "" {
		return "", nil
	}
	if err := validateServiceAccountName(sa); err != nil {
		return "", err
	}
	// validateServiceAccountName forbids embedded double quotes and backslashes, so the
	// double-quoted identifier cannot be broken out of — the statement is injection-safe.
	return `ASSUME SERVICE ACCOUNT "` + sa + `"`, nil
}
