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
	"github.com/lib/pq"
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
		requireMemoryLimitError(t, err, "tightly-capped service account should fail the query")
	})

	t.Run("memory limit still applies after a prior error on the same pooled connection", func(t *testing.T) {
		// Regression guard for the per-connection ASSUME model (review #2/#6): a query that
		// errors — here a memory-limit abort — must NOT silently revert the physical
		// connection to the base login. We pin ONE physical connection, fail a heavy query on
		// it, then prove the SAME reused connection is still capped. If the assumed account
		// had reverted to the (unlimited admin) base login, the second heavy query would
		// instead succeed. Confirmed against the Enterprise PGWire source: the assumed account
		// lives in the per-connection SecurityContext.accessList, survives the per-query reset
		// and non-fatal query errors, and reverts only on explicit EXIT, grant revocation, or
		// connection teardown.
		mustExec(fmt.Sprintf("ALTER SERVICE ACCOUNT %s SET MEMORY LIMIT 1K", sa))
		routed := routedConnection(t, sa)
		defer routed.Close()

		conn, err := routed.Conn(context.Background())
		require.NoError(t, err)
		defer conn.Close()

		_, err = conn.ExecContext(context.Background(), heavy)
		requireMemoryLimitError(t, err, "first heavy query on the pinned connection should hit the cap")

		_, err = conn.ExecContext(context.Background(), heavy)
		requireMemoryLimitError(t, err, "reused connection must still be capped after the prior error")
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

// TestPostCheckHealthRoutingIntegration verifies review #1's fix end-to-end against a
// running QuestDB *Enterprise* instance: Save & Test (PostCheckHealth) must actually run an
// ASSUME for the default service account, so a routing misconfiguration fails here rather
// than passing the green base-login check and breaking only on routed dashboard queries.
// Enterprise-gated for the same reason as TestServiceAccountRoutingIntegration.
func TestPostCheckHealthRoutingIntegration(t *testing.T) {
	if strings.ToLower(getEnv("QUESTDB_ENTERPRISE", "false")) != "true" {
		t.Skip("requires QuestDB Enterprise; set QUESTDB_ENTERPRISE=true and point QUESTDB_HOST/PORT at it")
	}

	admin := setupConnection(t)
	defer admin.Close()

	const sa = "sa_grafana_health_it"
	username := getEnv("QUESTDB_USERNAME", "admin")

	_, _ = admin.Exec("DROP SERVICE ACCOUNT " + sa) // best-effort cleanup from a prior run
	_, err := admin.Exec("CREATE SERVICE ACCOUNT " + sa)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.Exec("DROP SERVICE ACCOUNT " + sa) })

	ctx := context.Background()
	h := &plugin.QuestDB{}

	// Review #1's exact failure scenario: the account exists but the data source login was
	// never GRANTed ASSUME on it. The base Save & Test (default pool, no ASSUME) is green, so
	// without PostCheckHealth this misconfiguration would surface only on routed queries.
	t.Run("unhealthy when GRANT ASSUME is missing", func(t *testing.T) {
		res := h.PostCheckHealth(ctx, healthCheckRequest(t, sa))
		require.NotNil(t, res, "missing GRANT ASSUME must fail Save & Test")
		assert.Equal(t, backend.HealthStatusError, res.Status)
	})

	t.Run("healthy once the default account is granted and assumable", func(t *testing.T) {
		_, err := admin.Exec(fmt.Sprintf("GRANT ASSUME SERVICE ACCOUNT %s TO %s", sa, username))
		require.NoError(t, err)
		res := h.PostCheckHealth(ctx, healthCheckRequest(t, sa))
		assert.Nil(t, res, "a granted, assumable default account should pass Save & Test")
	})

	t.Run("unhealthy when the default account does not exist", func(t *testing.T) {
		res := h.PostCheckHealth(ctx, healthCheckRequest(t, "sa_does_not_exist_xyz"))
		require.NotNil(t, res, "a non-existent default account must fail Save & Test")
		assert.Equal(t, backend.HealthStatusError, res.Status)
	})
}

// routingTestConfig builds the shared jsonData + decrypted-secure map pointing at the test
// QuestDB instance with service-account routing enabled. extraJSON is appended verbatim as
// additional jsonData fields (e.g. `,"defaultServiceAccount":"sa"`); pass "" for none.
func routingTestConfig(t *testing.T, extraJSON string) ([]byte, map[string]string) {
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
		`{"server":%q,"port":%s,"username":%q,"tlsMode":%q,"tlsConfigurationMethod":%q,"serviceAccountRoutingEnabled":true%s}`,
		host, port, username, tlsMode, tlsMethod, extraJSON)
	return []byte(jsonData), secure
}

// healthCheckRequest builds a CheckHealthRequest whose data source enables routing with the
// given default service account, pointed at the test QuestDB instance (mirrors how Grafana
// invokes CheckHealth with the decrypted password present).
func healthCheckRequest(t *testing.T, defaultSA string) *backend.CheckHealthRequest {
	t.Helper()
	jsonData, secure := routingTestConfig(t, fmt.Sprintf(`,"defaultServiceAccount":%q`, defaultSA))
	return &backend.CheckHealthRequest{
		PluginContext: backend.PluginContext{
			DataSourceInstanceSettings: &backend.DataSourceInstanceSettings{
				JSONData:                jsonData,
				DecryptedSecureJSONData: secure,
			},
		},
	}
}

// routedConnection opens a *sql.DB through the plugin's Connect with service-account
// routing enabled, mirroring how sqlds would call it with a stamped connectionArgs
// message. Every physical connection in the returned pool assumes sa.
func routedConnection(t *testing.T, sa string) *sql.DB {
	t.Helper()
	jsonData, secure := routingTestConfig(t, "")
	cfg := backend.DataSourceInstanceSettings{
		JSONData:                jsonData,
		DecryptedSecureJSONData: secure,
	}
	msg, err := json.Marshal(map[string]string{"serviceAccount": sa})
	require.NoError(t, err)

	db, err := (&plugin.QuestDB{}).Connect(context.Background(), cfg, msg)
	require.NoError(t, err)
	return db
}

// requireMemoryLimitError asserts that err is QuestDB's memory-limit abort: a server-side
// pq error (a normal PGWire ErrorResponse, so the connection stays open and lib/pq returns
// *pq.Error) whose message is the memory-limit cap specifically. Requiring *pq.Error rather
// than just "some error" proves the physical connection stayed alive — so a passing reuse
// case means the assumed service account survived the prior error rather than the connection
// having silently died — and matching the message proves the failure is the cap, not some
// other server-side rejection. A service-account MEMORY LIMIT is enforced as a per-workload
// tracker limit, so QuestDB Enterprise emits "query memory limit exceeded [workload=...]";
// the server-wide cap emits "global RSS memory limit exceeded [...]". Both contain the
// substring "memory limit exceeded" (see io.questdb.std.Unsafe in questdb-enterprise).
func requireMemoryLimitError(t *testing.T, err error, msg string) {
	t.Helper()
	require.Error(t, err, msg)
	var pqErr *pq.Error
	require.ErrorAs(t, err, &pqErr, msg+" (expected a server-side pq error, not a dropped connection)")
	assert.Contains(t, strings.ToLower(pqErr.Message), "memory limit exceeded",
		msg+" (expected a QuestDB memory-limit abort, not another server-side error)")
}
