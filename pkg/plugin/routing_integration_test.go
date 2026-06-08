package plugin_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/questdb/grafana-questdb-datasource/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServiceAccountRoutingIntegration verifies the end-to-end ASSUME SERVICE ACCOUNT
// behavior (design §12) against a running QuestDB *Enterprise* instance. Service accounts
// and memory limits are Enterprise-only, so the default OSS test container cannot run it;
// the test is skipped unless QUESTDB_ENTERPRISE=true. To run it, point QUESTDB_HOST /
// QUESTDB_PORT (and credentials) at an Enterprise instance whose login can create service
// accounts and set memory limits, e.g.:
//
//	QUESTDB_USE_DOCKER=false QUESTDB_ENTERPRISE=true QUESTDB_HOST=... QUESTDB_PORT=... \
//	  go test ./pkg/plugin/ -run TestServiceAccountRoutingIntegration -v
//
// NOTE: the memory-limit thresholds / heavy query below are best-effort and may need
// tuning for the target instance's resources.
func TestServiceAccountRoutingIntegration(t *testing.T) {
	if strings.ToLower(getEnv("QUESTDB_ENTERPRISE", "false")) != "true" {
		t.Skip("requires QuestDB Enterprise; set QUESTDB_ENTERPRISE=true and point QUESTDB_HOST/PORT at it")
	}

	admin := setupConnection(t)
	defer admin.Close()

	const sa = "sa_grafana_it"
	username := getEnv("QUESTDB_USERNAME", "admin")

	mustExec := func(q string) {
		_, err := admin.Exec(q)
		require.NoError(t, err, q)
	}
	_, _ = admin.Exec("DROP SERVICE ACCOUNT " + sa) // best-effort cleanup from a prior run
	mustExec("CREATE SERVICE ACCOUNT " + sa)
	_, _ = admin.Exec("GRANT SELECT ON ALL TABLES TO " + sa) // best-effort; not needed by long_sequence()
	mustExec(fmt.Sprintf("GRANT ASSUME SERVICE ACCOUNT %s TO %s", sa, username))
	t.Cleanup(func() { _, _ = admin.Exec("DROP SERVICE ACCOUNT " + sa) })

	// A modestly memory-hungry query: a high-cardinality aggregation.
	const heavy = "SELECT x, count() FROM long_sequence(1000000) GROUP BY x"

	t.Run("assume runs and queries work through the routed pool", func(t *testing.T) {
		routed := routedConnection(t, sa)
		defer routed.Close()
		require.NoError(t, routed.Ping())
		var x int64
		require.NoError(t, routed.QueryRow("SELECT x FROM long_sequence(1)").Scan(&x))
		assert.Equal(t, int64(1), x)
	})

	t.Run("memory limit on the service account is enforced", func(t *testing.T) {
		// Unlimited: the query succeeds through the routed (assumed) pool.
		mustExec(fmt.Sprintf("ALTER SERVICE ACCOUNT %s SET MEMORY LIMIT 0", sa))
		unlimited := routedConnection(t, sa)
		_, err := unlimited.Exec(heavy)
		require.NoError(t, err, "unlimited service account should run the query")
		unlimited.Close()

		// Tight limit: the same query on a fresh pool must hit the cap. Only the limit
		// changed, so a failure here is attributable to the service account's limit.
		mustExec(fmt.Sprintf("ALTER SERVICE ACCOUNT %s SET MEMORY LIMIT 1K", sa))
		limited := routedConnection(t, sa)
		defer limited.Close()
		_, err = limited.Exec(heavy)
		assert.Error(t, err, "tightly-capped service account should fail the query")
	})

	t.Run("EXIT SERVICE ACCOUNT reverts to the base login", func(t *testing.T) {
		mustExec(fmt.Sprintf("ALTER SERVICE ACCOUNT %s SET MEMORY LIMIT 0", sa))
		routed := routedConnection(t, sa)
		defer routed.Close()
		// Pin a single physical connection so EXIT and the follow-up run on the same session.
		conn, err := routed.Conn(context.Background())
		require.NoError(t, err)
		defer conn.Close()
		_, err = conn.ExecContext(context.Background(), "EXIT SERVICE ACCOUNT")
		require.NoError(t, err)
		var x int64
		require.NoError(t, conn.QueryRowContext(context.Background(), "SELECT x FROM long_sequence(1)").Scan(&x))
		assert.Equal(t, int64(1), x)
	})
}

// routedConnection opens a *sql.DB through the plugin's Connect with service-account
// routing enabled, mirroring how sqlds would call it with a stamped connectionArgs
// message. Every physical connection in the returned pool assumes sa.
func routedConnection(t *testing.T, sa string) *sql.DB {
	t.Helper()
	host := getEnv("QUESTDB_HOST", "localhost")
	port := getEnv("QUESTDB_PORT", "8812")
	username := getEnv("QUESTDB_USERNAME", "admin")
	password := getEnv("QUESTDB_PASSWORD", "quest")
	tlsEnabled := getEnv("QUESTDB_TLS_ENABLED", "false")

	secure := map[string]string{"password": password}
	tlsMode := "disable"
	tlsMethod := ""
	if tlsEnabled == "true" {
		tlsMode = "verify-full"
		tlsMethod = "file-content"
		cwd, err := os.Getwd()
		require.NoError(t, err)
		caCert, err := os.ReadFile(path.Join(cwd, "../../keys/my-own-ca.crt"))
		require.NoError(t, err)
		secure["tlsCACert"] = string(caCert)
	}

	jsonData := fmt.Sprintf(
		`{"server":%q,"port":%s,"username":%q,"tlsMode":%q,"tlsConfigurationMethod":%q,"serviceAccountRoutingEnabled":true}`,
		host, port, username, tlsMode, tlsMethod)
	cfg := backend.DataSourceInstanceSettings{
		JSONData:                []byte(jsonData),
		DecryptedSecureJSONData: secure,
	}
	msg, err := json.Marshal(map[string]string{"serviceAccount": sa})
	require.NoError(t, err)

	db, err := (&plugin.QuestDB{}).Connect(context.Background(), cfg, msg)
	require.NoError(t, err)
	return db
}
