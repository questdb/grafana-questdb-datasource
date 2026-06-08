package plugin

import (
	"encoding/json"
	"fmt"
	"regexp"
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

	// Service-account routing fields are simple native JSON types (bool/string/array),
	// so a typed unmarshal is cleaner than the map extraction used above.
	applyServiceAccountSettings(&settings, config.JSONData)

	return settings, settings.isValid()
}

// applyServiceAccountSettings parses the (non-secret) service-account routing fields from
// jsonData onto settings. jsonData is assumed to be valid JSON; a parse error simply
// leaves the routing fields at their zero values (routing disabled).
func applyServiceAccountSettings(settings *Settings, jsonData []byte) {
	var sa struct {
		ServiceAccountRoutingEnabled bool                         `json:"serviceAccountRoutingEnabled"`
		DefaultServiceAccount        string                       `json:"defaultServiceAccount"`
		ServiceAccountMappings       []ServiceAccountMapping      `json:"serviceAccountMappings"`
		ServiceAccountGroupMappings  []ServiceAccountGroupMapping `json:"serviceAccountGroupMappings"`
		GroupsClaim                  string                       `json:"groupsClaim"`
	}
	_ = json.Unmarshal(jsonData, &sa)
	settings.ServiceAccountRoutingEnabled = sa.ServiceAccountRoutingEnabled
	settings.DefaultServiceAccount = sa.DefaultServiceAccount
	settings.ServiceAccountMappings = sa.ServiceAccountMappings
	settings.ServiceAccountGroupMappings = sa.ServiceAccountGroupMappings
	settings.GroupsClaim = sa.GroupsClaim
}

// LoadServiceAccountSettings parses only the service-account routing fields. Unlike
// LoadSettings it does not require valid credentials, so it is safe to call on the query
// path (MutateQueryData): routing config is non-secret jsonData and is resolved before a
// connection is established, independent of whether secrets are present in the request.
func LoadServiceAccountSettings(config backend.DataSourceInstanceSettings) Settings {
	var settings Settings
	applyServiceAccountSettings(&settings, config.JSONData)
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
			if strings.TrimSpace(gm.Group) == "" || strings.TrimSpace(gm.ServiceAccount) == "" {
				continue
			}
			if containsFold(groups, strings.TrimSpace(gm.Group)) {
				return strings.TrimSpace(gm.ServiceAccount)
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

// serviceAccountNamePattern guards against SQL injection / malformed names from config.
// It forbids whitespace, quotes and ';', so the name can be safely embedded in the
// ASSUME statement below.
var serviceAccountNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.\-]+$`)

// buildAssumeStatement returns the statement run on each new connection for the given
// service account. It returns ("", nil) when sa is empty, and an error when the name
// fails validation. There is no trailing ';' — it is executed as a single statement
// via ExecContext.
func buildAssumeStatement(sa string) (string, error) {
	if sa == "" {
		return "", nil
	}
	if !serviceAccountNamePattern.MatchString(sa) {
		return "", fmt.Errorf("invalid service account name %q", sa)
	}
	// The pattern forbids embedded double quotes, so the quoted identifier is injection-safe.
	return `ASSUME SERVICE ACCOUNT "` + sa + `"`, nil
}
