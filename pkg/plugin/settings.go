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

// int64OrString is an integer setting that tolerates being provisioned either as a JSON
// number (8812) or as a quoted string ("8812"); Grafana's frontend and provisioning have
// historically stored these numeric settings both ways, so a single json.Unmarshal of the
// config needs a type that accepts both forms. An absent field or a JSON null leaves it zero.
type int64OrString int64

func (n *int64OrString) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		// Base 0 matches the previous strconv.ParseInt(..., 0, 64) behavior.
		v, err := strconv.ParseInt(s, 0, 64)
		if err != nil {
			return err
		}
		*n = int64OrString(v)
		return nil
	}
	// A JSON number; truncate a fractional form, as the old float64->int64 path did.
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	*n = int64OrString(f)
	return nil
}

// jsonDataSettings is the wire shape of the non-secret jsonData blob. LoadSettings unmarshals
// the config into it once — numeric fields via int64OrString so a string-encoded number still
// parses — and copies it into Settings; secrets come separately from DecryptedSecureJSONData.
// The routing fields are the shared serviceAccountConfig, so they are declared in exactly one
// place (also used by applyServiceAccountSettings).
type jsonDataSettings struct {
	Server                 string        `json:"server"`
	Port                   int64OrString `json:"port"`
	Username               string        `json:"username"`
	Timeout                int64OrString `json:"timeout"`
	QueryTimeout           int64OrString `json:"queryTimeout"`
	TlsMode                string        `json:"tlsMode"`
	ConfigurationMethod    string        `json:"tlsConfigurationMethod"`
	TlsClientCertFile      string        `json:"tlsClientCertFile"`
	TlsClientKeyFile       string        `json:"tlsClientKeyFile"`
	EnableSecureSocksProxy bool          `json:"enableSecureSocksProxy"`
	MaxOpenConnections     int64OrString `json:"maxOpenConnections"`
	MaxIdleConnections     int64OrString `json:"maxIdleConnections"`
	MaxConnectionLifetime  int64OrString `json:"maxConnectionLifetime"`
	TimeInterval           string        `json:"timeInterval"`
	serviceAccountConfig
}

// LoadSettings will read and validate Settings from the DataSourceConfig
func LoadSettings(config backend.DataSourceInstanceSettings) (Settings, error) {
	var settings Settings
	// Single parse of the non-secret jsonData. int64OrString lets a numeric setting arrive as
	// either a JSON number or a quoted string; a type mismatch — or a syntactically invalid
	// document — surfaces as ErrorMessageInvalidJSON.
	var data jsonDataSettings
	if err := json.Unmarshal(config.JSONData, &data); err != nil {
		return settings, fmt.Errorf("%s: %w", err.Error(), ErrorMessageInvalidJSON)
	}

	settings.Server = data.Server
	settings.Port = int64(data.Port)
	settings.Username = data.Username
	settings.Timeout = int64(data.Timeout)
	settings.QueryTimeout = int64(data.QueryTimeout)
	settings.TlsMode = data.TlsMode
	settings.ConfigurationMethod = data.ConfigurationMethod
	settings.TlsClientCertFile = data.TlsClientCertFile
	settings.TlsClientKeyFile = data.TlsClientKeyFile
	settings.EnableSecureSocksProxy = data.EnableSecureSocksProxy
	settings.MaxOpenConnections = int64(data.MaxOpenConnections)
	settings.MaxIdleConnections = int64(data.MaxIdleConnections)
	settings.MaxConnectionLifetime = int64(data.MaxConnectionLifetime)
	settings.TimeInterval = data.TimeInterval
	data.serviceAccountConfig.applyTo(&settings)

	// Secrets live in the decrypted secure JSON blob, not in jsonData.
	if password, ok := config.DecryptedSecureJSONData["password"]; ok {
		settings.Password = password
	}
	if tlsCACert, ok := config.DecryptedSecureJSONData["tlsCACert"]; ok {
		settings.TlsCACert = tlsCACert
	}
	if tlsClientCert, ok := config.DecryptedSecureJSONData["tlsClientCert"]; ok {
		settings.TlsClientCert = tlsClientCert
	}
	if tlsClientKey, ok := config.DecryptedSecureJSONData["tlsClientKey"]; ok {
		settings.TlsClientKey = tlsClientKey
	}

	return settings, settings.isValid()
}

// serviceAccountConfig is the wire shape of the (non-secret) service-account routing fields.
// It is declared once and shared by LoadSettings (embedded in jsonDataSettings) and
// applyServiceAccountSettings, so adding or changing a routing field touches a single struct.
type serviceAccountConfig struct {
	ServiceAccountRoutingEnabled bool                         `json:"serviceAccountRoutingEnabled"`
	DefaultServiceAccount        string                       `json:"defaultServiceAccount"`
	ServiceAccountMappings       []ServiceAccountMapping      `json:"serviceAccountMappings"`
	ServiceAccountGroupMappings  []ServiceAccountGroupMapping `json:"serviceAccountGroupMappings"`
	GroupsClaim                  string                       `json:"groupsClaim"`
}

// applyTo copies the parsed routing fields onto settings.
func (sa serviceAccountConfig) applyTo(settings *Settings) {
	settings.ServiceAccountRoutingEnabled = sa.ServiceAccountRoutingEnabled
	settings.DefaultServiceAccount = sa.DefaultServiceAccount
	settings.ServiceAccountMappings = sa.ServiceAccountMappings
	settings.ServiceAccountGroupMappings = sa.ServiceAccountGroupMappings
	settings.GroupsClaim = sa.GroupsClaim
}

// applyServiceAccountSettings parses the (non-secret) service-account routing fields from
// jsonData onto settings and returns the json.Unmarshal error, if any. The per-query path
// (LoadServiceAccountSettings) ignores that error: a partial parse degrades safely (an
// unparseable mapping row drops its account and the affected user falls through toward the
// base login). Config-time callers (PostCheckHealth) surface it instead, so a provisioned
// type mismatch — e.g. a numeric serviceAccount, or a quoted boolean for the enable flag —
// fails Save & Test rather than silently mis-routing.
func applyServiceAccountSettings(settings *Settings, jsonData []byte) error {
	var sa serviceAccountConfig
	err := json.Unmarshal(jsonData, &sa)
	sa.applyTo(settings)
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

// resolveServiceAccountLazy maps the requesting Grafana user (and their OIDC groups) to a
// QuestDB service account. Precedence is most-specific-first:
//
//  1. username mapping  — exact (case-insensitive) match on user.Login
//  2. group mapping     — first configured mapping whose group is in the user's groups (case-insensitive)
//  3. default service account
//
// It returns "" when none of the above yields a name (the query then runs as the base
// login). A nil user (backend-initiated requests such as alerting/reporting) simply skips
// step 1 and falls through.
//
// groupsFn is invoked at most once, and only if resolution reaches step 2 (no username
// mapping matched AND at least one group mapping is configured). This lets the caller defer
// decoding a forwarded OIDC ID token until it is actually needed, so the token is not parsed
// for username-mapped users or when no group mappings exist.
//
// Mappings with a blank service account are skipped: a half-filled config row must not
// silently make a mapped user bypass the cap by dropping to the base login — such a user
// falls through to the next step instead. Returned names are whitespace-trimmed.
func (settings *Settings) resolveServiceAccountLazy(user *backend.User, groupsFn func() []string) string {
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
	// config order wins — a deterministic, operator-controlled tie-break. Resolve the
	// user's groups lazily and only when there are mappings to match them against, so a
	// forwarded ID token is not decoded once a username mapping has already won.
	if len(settings.ServiceAccountGroupMappings) > 0 && groupsFn != nil {
		if groups := groupsFn(); len(groups) > 0 {
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
