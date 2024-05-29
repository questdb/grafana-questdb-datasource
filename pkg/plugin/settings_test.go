package plugin

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/proxy"
	"github.com/stretchr/testify/assert"
)

func TestLoadSettings(t *testing.T) {
	t.Run("should parse settings correctly", func(t *testing.T) {
		type args struct {
			config backend.DataSourceInstanceSettings
		}
		tests := []struct {
			name             string
			args             args
			expectedSettings Settings
			expectedErr      error
		}{
			{
				name: "should parse json with tls disabled",
				args: args{
					config: backend.DataSourceInstanceSettings{
						UID: "ds-uid",
						JSONData: []byte(`{ "server": "test", "port": 8812, "username": "john", "timeout": 10, "queryTimeout": 50, 
											"enableSecureSocksProxy": false, "tlsMode": "disable",
											"maxOpenConnections": 100, "maxIdleConnections": 100, "maxConnectionLifetime": 14400 }`),
						DecryptedSecureJSONData: map[string]string{"password": "doe"},
					},
				},
				expectedSettings: Settings{
					Server:                "test",
					Port:                  8812,
					Username:              "john",
					Password:              "doe",
					Timeout:               10,
					QueryTimeout:          50,
					MaxOpenConnections:    100,
					MaxIdleConnections:    100,
					MaxConnectionLifetime: 14400,
					TlsMode:               "disable",
				},
				expectedErr: nil,
			},
			{
				name: "should parse json with tls and file-content mode",
				args: args{
					config: backend.DataSourceInstanceSettings{
						UID: "ds-uid",
						JSONData: []byte(`{ "server": "test", "port": 1000, "username": "john", "timeout": 10, "queryTimeout": 50, 
											"enableSecureSocksProxy": true, "tlsMode": "verify-full", "tlsConfigurationMethod": "file-content",
											"maxOpenConnections": 100, "maxIdleConnections": 100, "maxConnectionLifetime": 14400 }`),
						DecryptedSecureJSONData: map[string]string{"password": "doe", "tlsCACert": "caCert", "tlsClientCert": "clientCert", "tlsClientKey": "clientKey", "secureSocksProxyPassword": "test"},
					},
				},
				expectedSettings: Settings{
					Server:                "test",
					Port:                  1000,
					Username:              "john",
					Password:              "doe",
					TlsCACert:             "caCert",
					TlsClientCert:         "clientCert",
					TlsClientKey:          "clientKey",
					Timeout:               10,
					QueryTimeout:          50,
					MaxOpenConnections:    100,
					MaxIdleConnections:    100,
					MaxConnectionLifetime: 14400,
					TlsMode:               "verify-full",
					ConfigurationMethod:   "file-content",
					ProxyOptions: &proxy.Options{
						Enabled: true,
						Auth: &proxy.AuthOptions{
							Username: "ds-uid",
							Password: "test",
						},
						Timeouts: &proxy.TimeoutOptions{
							Timeout:   10 * time.Second,
							KeepAlive: proxy.DefaultTimeoutOptions.KeepAlive,
						},
					},
				},
				expectedErr: nil,
			},
			{
				name: "should parse json with tls and file-path mode",
				args: args{
					config: backend.DataSourceInstanceSettings{
						UID: "ds-uid",
						JSONData: []byte(`{ "server": "test", "port": 8812, "username": "john",
											"enableSecureSocksProxy": true, "tlsMode": "verify-ca", "tlsConfigurationMethod": "file-path",
											"tlsCACertFile": "/var/caCertFile", "tlsClientCertFile": "/var/clientCertFile", "tlsClientKeyFile": "/var/clientKeyFile",
											"timeout": 10, "queryTimeout": 50, "maxOpenConnections": 100, "maxIdleConnections": 100, "maxConnectionLifetime": 14400 }`),
						DecryptedSecureJSONData: map[string]string{"password": "rambo", "secureSocksProxyPassword": "test"},
					},
				},
				expectedSettings: Settings{
					Server:                "test",
					Port:                  8812,
					Username:              "john",
					Password:              "rambo",
					TlsCACertFile:         "/var/caCertFile",
					TlsClientCertFile:     "/var/clientCertFile",
					TlsClientKeyFile:      "/var/clientKeyFile",
					Timeout:               10,
					QueryTimeout:          50,
					MaxOpenConnections:    100,
					MaxIdleConnections:    100,
					MaxConnectionLifetime: 14400,
					TlsMode:               "verify-ca",
					ConfigurationMethod:   "file-path",
					ProxyOptions: &proxy.Options{
						Enabled: true,
						Auth: &proxy.AuthOptions{
							Username: "ds-uid",
							Password: "test",
						},
						Timeouts: &proxy.TimeoutOptions{
							Timeout:   10 * time.Second,
							KeepAlive: proxy.DefaultTimeoutOptions.KeepAlive,
						},
					},
				},
				expectedErr: nil,
			},
			{
				name: "should converting string values to the correct type",
				args: args{
					config: backend.DataSourceInstanceSettings{
						JSONData:                []byte(`{"server": "test", "username": "u", "port": "1234", "timeout": 15, "queryTimeout": 25, "maxOpenConnections": 10, "maxIdleConnections": 5, "maxConnectionLifetime": 3600   }`),
						DecryptedSecureJSONData: map[string]string{"password": "p"},
					},
				},
				expectedSettings: Settings{
					Server:                "test",
					Port:                  1234,
					Username:              "u",
					Password:              "p",
					Timeout:               15,
					QueryTimeout:          25,
					MaxOpenConnections:    10,
					MaxIdleConnections:    5,
					MaxConnectionLifetime: 3600,
					ProxyOptions:          nil,
				},
				expectedErr: nil,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				gotSettings, err := LoadSettings(tt.args.config)
				assert.Equal(t, tt.expectedErr, err)
				if !reflect.DeepEqual(gotSettings, tt.expectedSettings) {
					t.Errorf("LoadSettings() = %v, want %v", gotSettings, tt.expectedSettings)
				}
			})
		}
	})
	t.Run("should capture invalid settings", func(t *testing.T) {
		tests := []struct {
			jsonData    string
			password    string
			wantErr     error
			description string
		}{
			{jsonData: `{ "server": "", "port": 123 }`, password: "", wantErr: ErrorMessageInvalidServerName, description: "should capture empty server name"},
			{jsonData: `{ "server": "foo" }`, password: "", wantErr: ErrorMessageInvalidPort, description: "should capture nil port"},
			{jsonData: `  "server": "foo", "port": 443, "username" : "foo" }`, password: "", wantErr: ErrorMessageInvalidJSON, description: "should capture invalid json"},
		}
		for i, tc := range tests {
			t.Run(fmt.Sprintf("[%v/%v] %s", i+1, len(tests), tc.description), func(t *testing.T) {
				_, err := LoadSettings(backend.DataSourceInstanceSettings{
					JSONData:                []byte(tc.jsonData),
					DecryptedSecureJSONData: map[string]string{"password": tc.password},
				})
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("%s not captured. %s", tc.wantErr, err.Error())
				}
			})
		}
	})
}
