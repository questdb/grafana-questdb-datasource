package plugin_test

import (
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/docker/docker/api/types/mount"
	"github.com/lib/pq"
	"math"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-units"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/grafana/grafana-plugin-sdk-go/data/sqlutil"
	"github.com/questdb/grafana-questdb-datasource/pkg/converters"
	"github.com/questdb/grafana-questdb-datasource/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func TestMain(m *testing.M) {
	useDocker := strings.ToLower(getEnv("QUESTDB_USE_DOCKER", "true"))
	if useDocker == "false" {
		fmt.Printf("Using external QuestDB for IT tests -  %s:%s\n",
			getEnv("QUESTDB_PORT", "8812"), getEnv("QUESTDB_HOST", "localhost"))
		os.Exit(m.Run())
	}
	// create a QuestDB container
	ctx := context.Background()
	provider, err := testcontainers.ProviderDocker.GetProvider()
	if err != nil {
		fmt.Printf("Docker is not running and no questdb connections details were provided. Skipping IT tests: %s\n", err)
		os.Exit(0)
	}
	err = provider.Health(ctx)
	if err != nil {
		fmt.Printf("Docker is not running and no questdb connections details were provided. Skipping IT tests: %s\n", err)
		os.Exit(0)
	}
	questDbName := GetEnv("QUESTDB_NAME", "questdb/questdb")
	questDbVersion := GetEnv("QUESTDB_VERSION", "latest")
	fmt.Printf("Using Docker for tests with QuestDB %s:%s\n", questDbName, questDbVersion)
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	keysPath := "../../config/keys"
	serverConfPath := "../../config/server.conf"

	req := testcontainers.ContainerRequest{
		Env: map[string]string{
			"TZ": "UTC",
		},
		ExposedPorts: []string{"9000/tcp", "8812/tcp"},
		HostConfigModifier: func(config *container.HostConfig) {
			config.Mounts = append(config.Mounts,
				mount.Mount{Source: path.Join(cwd, serverConfPath), Target: "/var/lib/questdb/conf/server.conf", ReadOnly: true, Type: mount.TypeBind},
				mount.Mount{Source: path.Join(cwd, keysPath), Target: "/var/lib/questdb/conf/keys", ReadOnly: true, Type: mount.TypeBind})
		},
		Image: fmt.Sprintf("%s:%s", questDbName, questDbVersion),
		Resources: container.Resources{
			Ulimits: []*units.Ulimit{
				{
					Name: "nofile",
					Hard: 262144,
					Soft: 262144,
				},
			},
		},
		WaitingFor: wait.ForLog("A server-main enjoy"),
	}
	questdbContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		// can't test without container
		panic(err)
	}
	p, _ := questdbContainer.MappedPort(ctx, "8812")
	os.Setenv("QUESTDB_PORT", p.Port())
	os.Setenv("QUESTDB_HOST", "localhost")
	defer questdbContainer.Terminate(ctx) //nolint
	os.Exit(m.Run())
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func TestConnect(t *testing.T) {
	port := getEnv("QUESTDB_PORT", "8812")
	host := getEnv("QUESTDB_HOST", "localhost")
	username := getEnv("QUESTDB_USERNAME", "admin")
	password := getEnv("QUESTDB_PASSWORD", "quest")
	tlsMode := getEnv("QUESTDB_SSL", "disable")
	queryTimeoutNumber := 3600
	queryTimeoutString := "3600"
	questdb := plugin.QuestDB{}
	t.Run("should not error when valid settings passed", func(t *testing.T) {
		secure := map[string]string{}
		secure["password"] = password
		settings := backend.DataSourceInstanceSettings{JSONData: []byte(fmt.Sprintf(`{ "server": "%s", "port": %s, "username": "%s", "queryTimeout": "%s", "tlsMode": "%s"}`,
			host, port, username, queryTimeoutString, tlsMode)), DecryptedSecureJSONData: secure}
		_, err := questdb.Connect(settings, json.RawMessage{})
		assert.Equal(t, nil, err)
	})
	t.Run("should not error when valid settings passed - with query timeout as number", func(t *testing.T) {
		secure := map[string]string{}
		secure["password"] = password
		settings := backend.DataSourceInstanceSettings{JSONData: []byte(fmt.Sprintf(`{ "server": "%s", "port": %s, "username": "%s", "queryTimeout": %d, "tlsMode": "%s"}`,
			host, port, username, queryTimeoutNumber, tlsMode)), DecryptedSecureJSONData: secure}
		_, err := questdb.Connect(settings, json.RawMessage{})
		assert.Equal(t, nil, err)
	})
}

func TestPgWireConnect(t *testing.T) {
	port := getEnv("QUESTDB_PORT", "8812")
	host := getEnv("QUESTDB_HOST", "localhost")
	username := getEnv("QUESTDB_USERNAME", "admin")
	password := getEnv("QUESTDB_PASSWORD", "quest")
	tlsMode := getEnv("QUESTDB_SSL", "disable")
	questdb := plugin.QuestDB{}
	t.Run("should not error when valid settings passed", func(t *testing.T) {
		secure := map[string]string{}
		secure["password"] = password
		settings := backend.DataSourceInstanceSettings{JSONData: []byte(fmt.Sprintf(`{ "server": "%s", "port": %s, "username": "%s", "password": "%s", "tlsMode": "%s"}`, host, port, username, password, tlsMode)), DecryptedSecureJSONData: secure}
		_, err := questdb.Connect(settings, json.RawMessage{})
		assert.Equal(t, nil, err)
	})
}

func setupConnection(t *testing.T, settings *plugin.Settings) *sql.DB {
	port, err := strconv.ParseInt(getEnv("QUESTDB_PORT", "8812"), 10, 64)
	if err != nil {
		panic(err)
	}

	host := getEnv("QUESTDB_HOST", "localhost")
	username := getEnv("QUESTDB_USERNAME", "admin")
	password := getEnv("QUESTDB_PASSWORD", "quest")
	tlsMode := getEnv("QUESTDB_SSL", "disable")
	tlsConfigurationMethod := getEnv("QUESTDB_METHOD", "file-content")
	tlsCaCert := getEnv("QUESTDB_CA_CERT", `
-----BEGIN CERTIFICATE-----
MIIDHDCCAgSgAwIBAgIUJ0QbXYlE2EEuBtlURPgDc5Z9QaowDQYJKoZIhvcNAQEL
BQAwEzERMA8GA1UEAwwIcWRiX3Jvb3QwHhcNMjQwMjIwMTI1NTIwWhcNMzQwMjE3
MTI1NTIwWjATMREwDwYDVQQDDAhxZGJfcm9vdDCCASIwDQYJKoZIhvcNAQEBBQAD
ggEPADCCAQoCggEBALP08uf35zioPW+p1MsLwtAPuMAgUfRDF/G9IbSAIIMJ65v4
GVS6NXCf7qJmoLdfL+h/+DHhfscONs7o3Rzdj5ZNwGpJ3zvaxI7AGQwyvGxmLrq4
+UiQTWaP8ivTJGLAReRlfznjpouwJFluhp03rPtj5h6kYsiFbBWvHKf+KbUDotI8
xnGshba+IGJNR+jC1zto3vVkrzcL+D52HVG9nczCiRNtLa8lhsRmVR8YUSitn3ly
9xE75XlC7AxatI/011bSpDIDka2+Au8vLcZDk8q+i6/vkYK0FUdSL5WmvtfOspnP
5M5AQEGLQvrhYV1ojRlgLo/rJX02+2baEwQzxDECAwEAAaNoMGYwHQYDVR0OBBYE
FG5kKRTI/Oz/kGF22WZNw9UcOb1xMB8GA1UdIwQYMBaAFG5kKRTI/Oz/kGF22WZN
w9UcOb1xMA8GA1UdEwEB/wQFMAMBAf8wEwYDVR0RBAwwCoIIcWRiX3Jvb3QwDQYJ
KoZIhvcNAQELBQADggEBADR6VnCB3iB6Mr5S8MvuDlwdANkT0Gmm7rvJi/4mOj0A
5hd4S39684RrzzNyakb0aEEuDdzlbJ6EC7rorks37vMNmUAa7LrFESBHPcPnmDcq
rjW8amE17P5QTtJiEKiIRG8xD8grCK2MF61I285BY4pbqE+oNeQw33Y73SfQZHjV
5ZCQpdxYur3Z5BFFBqFowimrRBb/HpMd/9P+/jFNxeYXQWuzjt5cEcQtdx2ca/Ix
hbpD1K0Asm0IA2AoiC+5F9zmp6+f4UtHFKU6PeDBQVLQyzjiIb4tF1ZX9M4LrdW+
TIFr7kfJsOwa+y1x3aTs/7VSwNjfS4FqbvXy3S7OAOs=
-----END CERTIFICATE-----
`)

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM([]byte(tlsCaCert))

	// we create a direct connection since we need specific settings for insert
	cnnstr, err := plugin.GenerateConnectionString(plugin.Settings{
		Server:              host,
		Port:                port,
		Username:            username,
		Password:            password,
		TlsMode:             tlsMode,
		ConfigurationMethod: tlsConfigurationMethod,
		TlsCACert:           tlsCaCert,
	}, "version")
	if err != nil {
		panic(err)
	}

	connector, err := pq.NewConnector(cnnstr)
	conn := sql.OpenDB(connector)
	return conn
}

func TestInsertAndQueryData(t *testing.T) {
	conn := setupConnection(t, nil)

	_, err := conn.Exec("DROP TABLE IF EXISTS all_types")
	require.NoError(t, err)

	_, err = conn.Exec("CREATE TABLE all_types (\n" +
		"  bool boolean," +
		"  byte_ byte," +
		"  short_ short," +
		"  char_ char," +
		"  int_ int," +
		"  long_ long," +
		"  date_ date, " +
		"  tstmp timestamp, " +
		"  float_ float," +
		"  double_ double," +
		"  str string," +
		"  sym symbol," +
		"  ge1 geohash(1c)," +
		"  ge2 geohash(2c)," +
		"  ge4 geohash(4c)," +
		"  ge8 geohash(8c)," +
		"  ip ipv4, " +
		"  uuid_ uuid ," +
		"  l256 long256," +
		"  ts timestamp " +
		") TIMESTAMP(ts) PARTITION BY YEAR BYPASS WAL")
	require.NoError(t, err)

	close := func(t *testing.T) {
		_, err := conn.Exec("DROP TABLE all_types")
		require.NoError(t, err)
	}
	defer close(t)

	date, err := time.ParseInLocation("2006-01-02T15:04:05.999", "2022-01-12T12:01:01.120", time.UTC)
	require.NoError(t, err)
	timestamp, err := time.ParseInLocation("2006-01-02T15:04:05.999999", "2020-02-13T10:11:12.123450", time.UTC)
	require.NoError(t, err)

	// insert data
	_, err = conn.Exec("INSERT INTO all_types(ts) VALUES ('2020-02-13T10:11:12.123450')")
	require.NoError(t, err)

	stmt, err := conn.Prepare("INSERT INTO all_types values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, " +
		"cast($13 as geohash(1c)), cast($14 as geohash(2c)) , cast($15  as geohash(4c)), cast($16 as geohash(8c)), $17, $18, cast('' || $19 as long256), $20)")
	require.NoError(t, err)
	defer stmt.Close()

	var data = [][]interface{}{
		{bool(false), int16(0), int16(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, timestamp},

		{bool(true), int16(1), int16(1), mkstring("a"), mkint32(4), mkint64(5), &date, &timestamp, mkfloat32(12.345), mkfloat64(1.0234567890123),
			mkstring("string"), mkstring("symbol"), mkstring("r"), mkstring("rj"), mkstring("rjtw"), mkstring("rjtwedd0"), mkstring("1.2.3.4"),
			mkstring("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"), mkstring("0x5dd94b8492b4be20632d0236ddb8f47c91efc2568b4d452847b4a645dbe4871a"), &timestamp},

		{bool(true), int16(math.MaxInt8), int16(math.MaxInt16), mkstring("z"), mkint32(math.MaxInt32), mkint64(math.MaxInt64), &date, mktimestamp("1970-01-01T00:00:00.000000", t),
			mkfloat32(math.MaxFloat32), mkfloat64(math.MaxFloat64), mkstring("XXX"), mkstring(" "), mkstring("e"), mkstring("ee"), mkstring("eeee"), mkstring("eeeeeeee"),
			mkstring("255.255.255.255"), mkstring("a0eebc99-ffff-ffff-ffff-ffffffffffff"), mkstring("0x5dd94b8492b4be20632d0236ddb8f47c91efc2568b4d452847b4a645dbefffff"), mktimestamp("2020-03-31T00:00:00.987654", t)}}

	for i := 1; i < len(data); i++ {
		_, err = stmt.Exec(data[i]...)
		require.NoError(t, err)
	}

	// assert data in table
	rows, err := conn.Query("SELECT * FROM all_types")
	require.NoError(t, err)
	frame, err := sqlutil.FrameFromRows(rows, 10, converters.QdbConverters...)
	require.NoError(t, err)

	for i, row := range data {
		assertRow(t, frame.Fields, i, row...)
	}
}

func mktimestamp(s string, t *testing.T) *time.Time {
	timestamp, err := time.ParseInLocation("2006-01-02T15:04:05.999999", s, time.UTC)
	require.NoError(t, err)
	return &timestamp
}

func mkstring(s string) *string {
	return &s
}

func mkfloat32(f float32) *float32 {
	return &f
}

func mkfloat64(f float64) *float64 {
	return &f
}

func mkint32(i int32) *int32 {
	return &i
}

func mkint64(i int64) *int64 {
	return &i
}

func assertRow(t *testing.T, field []*data.Field, rowIdx int, expectedValues ...interface{}) {
	for i, expectedValue := range expectedValues {
		actual := field[i].At(rowIdx)
		if expectedValue == nil {
			assert.Nil(t, actual)
			return
		}
		switch valueType := expectedValue.(type) {
		case float32:
			assert.InDelta(t, valueType, actual, 0.01)
		case float64:
			assert.InDelta(t, valueType, actual, 0.01)
		default:
			assert.Equal(t, expectedValue, actual, "row %d field %d", rowIdx, i)
		}
	}
}
