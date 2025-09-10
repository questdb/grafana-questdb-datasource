package plugin_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/mount"
	"github.com/lib/pq"

	"github.com/docker/docker/api/types/container"
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
	questDbTlsEnabled := GetEnv("QUESTDB_TLS_ENABLED", "false")
	fmt.Printf("Using Docker for tests with QuestDB %s:%s\n", questDbName, questDbVersion)

	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	keysPath := "../../keys"

	req := testcontainers.ContainerRequest{
		Env: map[string]string{
			"TZ":                          "UTC",
			"QDB_PG_TLS_ENABLED":          questDbTlsEnabled,
			"QDB_PG_TLS_CERT_PATH":        "/var/lib/questdb/conf/keys/server.crt",
			"QDB_PG_TLS_PRIVATE_KEY_PATH": "/var/lib/questdb/conf/keys/server.key",
		},
		ExposedPorts: []string{"9000/tcp", "8812/tcp"},
		HostConfigModifier: func(config *container.HostConfig) {
			config.Mounts = append(config.Mounts,
				mount.Mount{Source: path.Join(cwd, keysPath), Target: "/var/lib/questdb/conf/keys", ReadOnly: true, Type: mount.TypeBind})
		},
		Image:      fmt.Sprintf("%s:%s", questDbName, questDbVersion),
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
	tlsEnabled := getEnv("QUESTDB_TLS_ENABLED", "false")
	queryTimeout := 3600
	connectTimeout := 1000
	maxOpenConns := 10
	maxIdleConns := 5
	maxConnLife := 14400

	questdb := plugin.QuestDB{}

	var tlsModes []string
	if tlsEnabled == "true" {
		tlsModes = []string{"require", "verify-ca", "verify-full"}
	} else {
		tlsModes = []string{"disable"}
	}

	for _, tlsMode := range tlsModes {
		t.Run("should not error when valid settings passed, tlsMode: "+tlsMode, func(t *testing.T) {
			secure := map[string]string{}
			secure["password"] = password
			settings := backend.DataSourceInstanceSettings{JSONData: []byte(fmt.Sprintf(
				`{ "server": "%s", "port": %s, "username": "%s", "tlsMode": "%s", "queryTimeout": "%d", "timeout": "%d", "maxOpenConnections": "%d", "maxIdleConnections": "%d", "maxConnectionLifetime": "%d" }`,
				host, port, username, tlsMode, queryTimeout, connectTimeout, maxOpenConns, maxIdleConns, maxConnLife)), DecryptedSecureJSONData: secure}

			db, err := questdb.Connect(context.Background(), settings, json.RawMessage{})
			assert.Equal(t, nil, err)

			err = db.Ping()
			assert.Equal(t, nil, err)
		})
	}
}

func setupConnection(t *testing.T) *sql.DB {
	port, err := strconv.ParseInt(getEnv("QUESTDB_PORT", "8812"), 10, 64)
	if err != nil {
		panic(err)
	}

	host := getEnv("QUESTDB_HOST", "localhost")
	username := getEnv("QUESTDB_USERNAME", "admin")
	password := getEnv("QUESTDB_PASSWORD", "quest")
	tlsEnabled := getEnv("QUESTDB_TLS_ENABLED", "false")
	tlsConfigurationMethod := getEnv("QUESTDB_METHOD", "file-content")

	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	tlsCaCert, err := os.ReadFile(path.Join(cwd, "../../keys/my-own-ca.crt"))
	if err != nil {
		panic(err)
	}

	var tlsMode string
	if tlsEnabled == "true" {
		tlsMode = "verify-full"
	} else {
		tlsMode = "disable"
	}

	cnnstr, err := plugin.GenerateConnectionString(plugin.Settings{
		Server:              host,
		Port:                port,
		Username:            username,
		Password:            password,
		TlsMode:             tlsMode,
		ConfigurationMethod: tlsConfigurationMethod,
		TlsCACert:           string(tlsCaCert),
	}, "version")
	if err != nil {
		panic(err)
	}

	connector, err := pq.NewConnector(cnnstr)
	conn := sql.OpenDB(connector)
	return conn
}

func TestInsertAndQueryData(t *testing.T) {
	conn := setupConnection(t)

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
		"cast($13 as geohash(1c)), cast($14 as geohash(2c)) , cast($15  as geohash(4c)), cast($16 as geohash(8c)), $17, $18, $19)")
	require.NoError(t, err)
	defer stmt.Close()

	var data = [][]interface{}{
		{bool(false), int16(0), int16(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, timestamp},
		{bool(true), int16(1), int16(1), mkstring("a"), mkint32(4), mkint64(5), &date, &timestamp, mkfloat32(12.345), mkfloat64(1.0234567890123),
			mkstring("string"), mkstring("symbol"), mkstring("r"), mkstring("rj"), mkstring("rjtw"), mkstring("rjtwedd0"), mkstring("1.2.3.4"),
			mkstring("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"), &timestamp},
		{bool(true), int16(math.MaxInt8), int16(math.MaxInt16), mkstring("z"), mkint32(math.MaxInt32), mkint64(math.MaxInt64), &date, mktimestamp("1970-01-01T00:00:00.000000", t),
			mkfloat32(math.MaxFloat32), mkfloat64(math.MaxFloat64), mkstring("XXX"), mkstring(" "), mkstring("e"), mkstring("ee"), mkstring("eeee"), mkstring("eeeeeeee"),
			mkstring("255.255.255.255"), mkstring("a0eebc99-ffff-ffff-ffff-ffffffffffff"), mktimestamp("2020-03-31T00:00:00.987654", t)}}

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
