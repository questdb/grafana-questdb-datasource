package plugin

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/proxy"
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

	Timeout               string `json:"timeout,omitempty"`
	QueryTimeout          string `json:"queryTimeout,omitempty"`
	ProxyOptions          *proxy.Options
	MaxOpenConnections    int64 `json:"maxOpenConnections,omitempty"`
	MaxIdleConnections    int64 `json:"maxIdleConnections,omitempty"`
	MaxConnectionLifetime int64 `json:"maxConnectionLifetime,omitempty"`

	TlsMode             string `json:"tlsMode"`
	ConfigurationMethod string `json:"tlsConfigurationMethod"`

	TlsCACertFile     string `json:"tlsCaCertFile"`
	TlsClientCertFile string `json:"tlsClientCertFile"`
	TlsClientKeyFile  string `json:"tlsClientKeyFile"`
}

type CustomSetting struct {
	Setting string `json:"setting"`
	Value   string `json:"value"`
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
		settings.Timeout = jsonData["timeout"].(string)
	}
	if jsonData["queryTimeout"] != nil {
		if val, ok := jsonData["queryTimeout"].(string); ok {
			settings.QueryTimeout = val
		}
		if val, ok := jsonData["queryTimeout"].(float64); ok {
			settings.QueryTimeout = fmt.Sprintf("%d", int64(val))
		}
	}

	if strings.TrimSpace(settings.Timeout) == "" {
		settings.Timeout = "10"
	}
	if strings.TrimSpace(settings.QueryTimeout) == "" {
		settings.QueryTimeout = "60"
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

	if jsonData["tlsCACertFile"] != nil {
		settings.TlsCACertFile = jsonData["tlsCACertFile"].(string)
	}
	if jsonData["tlsClientCertFile"] != nil {
		settings.TlsClientCertFile = jsonData["tlsClientCertFile"].(string)
	}
	if jsonData["tlsClientKeyFile"] != nil {
		settings.TlsClientKeyFile = jsonData["tlsClientKeyFile"].(string)
	}

	// proxy options are only able to be loaded via environment variables
	// currently, so we pass `nil` here so they are loaded with defaults
	proxyOpts, err := config.ProxyOptions(nil)

	if err == nil && proxyOpts != nil {
		// the sdk expects the timeout to not be a string
		timeout, err := strconv.ParseFloat(settings.Timeout, 64)
		if err == nil {
			proxyOpts.Timeouts.Timeout = (time.Duration(timeout) * time.Second)
		}

		settings.ProxyOptions = proxyOpts
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

	return settings, settings.isValid()
}
