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
// behavior (design §12) against a running QuestDB *Enterprise* instance (4.0.2+, for
// per-principal memory limits). Service accounts and memory limits are Enterprise-only, so
// the default OSS test container cannot run it; the test is skipped unless
// QUESTDB_ENTERPRISE=true. To run it, point QUESTDB_HOST / QUESTDB_PORT (and credentials)
// at an Enterprise instance whose login — e.g. the built-in admin — can create users and
// service accounts and set memory limits, e.g.:
//
//	QUESTDB_USE_DOCKER=false QUESTDB_ENTERPRISE=true QUESTDB_HOST=... QUESTDB_PORT=... \
//	  go test ./pkg/plugin/ -run 'RoutingIntegration' -v
//
// That login only sets the fixture up. The routed pools log in as a dedicated non-admin
// user the test creates, as the README recommends for the data source: the built-in admin
// can assume any service account without a grant and cannot be given a memory limit, so the
// base-login limit cases below could not fail against it.
//
// NOTE: the memory-limit thresholds / heavy query below are best-effort and may need
// tuning for the target instance's resources.
func TestServiceAccountRoutingIntegration(t *testing.T) {
	if strings.ToLower(getEnv("QUESTDB_ENTERPRISE", "false")) != "true" {
		t.Skip("requires QuestDB Enterprise; set QUESTDB_ENTERPRISE=true and point QUESTDB_HOST/PORT at it")
	}

	admin := setupConnection(t)
	// Registered before the fixtures, so it runs after their DROP cleanups (a deferred Close
	// would run first and silently fail them).
	t.Cleanup(func() { _ = admin.Close() })

	const sa = "sa_grafana_it"
	base := createRoutingLogin(t, admin, "grafana_it_base")
	createServiceAccount(t, admin, sa)
	mustExec(t, admin, fmt.Sprintf("GRANT ASSUME SERVICE ACCOUNT %s TO %s", sa, base.username))

	// setLimits pins both limits at the start of each subtest so none depends on the order
	// they run in. UNLIMITED clears a limit; the server-wide query limit (0 by default) then
	// applies.
	setLimits := func(t *testing.T, saLimit, baseLimit string) {
		t.Helper()
		mustExec(t, admin, fmt.Sprintf("ALTER SERVICE ACCOUNT %s SET MEMORY LIMIT %s", sa, saLimit))
		mustExec(t, admin, fmt.Sprintf("ALTER USER %s SET MEMORY LIMIT %s", base.username, baseLimit))
	}
	t.Cleanup(func() { _, _ = admin.Exec(fmt.Sprintf("ALTER USER %s SET MEMORY LIMIT UNLIMITED", base.username)) })

	// A modestly memory-hungry query: a high-cardinality aggregation.
	const heavy = "SELECT x, count() FROM long_sequence(1000000) GROUP BY x"
	ctx := context.Background()

	t.Run("assume runs and queries work through the routed pool", func(t *testing.T) {
		setLimits(t, "UNLIMITED", "UNLIMITED")
		routed := routedConnection(t, base, sa)
		defer routed.Close()
		require.NoError(t, routed.Ping())
		var x int64
		require.NoError(t, routed.QueryRow("SELECT x FROM long_sequence(1)").Scan(&x))
		assert.Equal(t, int64(1), x)
	})

	t.Run("memory limit on the service account is enforced", func(t *testing.T) {
		// Unlimited: the query succeeds through the routed (assumed) pool.
		setLimits(t, "UNLIMITED", "UNLIMITED")
		unlimited := routedConnection(t, base, sa)
		_, err := unlimited.Exec(heavy)
		require.NoError(t, err, "unlimited service account should run the query")
		unlimited.Close()

		// Tight limit: the same query on a fresh pool must hit the cap. Only the limit
		// changed, so a failure here is attributable to the service account's limit.
		setLimits(t, "1K", "UNLIMITED")
		limited := routedConnection(t, base, sa)
		defer limited.Close()
		_, err = limited.Exec(heavy)
		requireMemoryLimitError(t, err, "tightly-capped service account should fail the query")
	})

	t.Run("a changed limit reaches an already-open routed connection", func(t *testing.T) {
		// QuestDB resolves the principal's limit per query, so an operator can retune a limit
		// without Grafana reconnecting (the README relies on this). Pin one physical
		// connection so both queries provably run on the same assumed session.
		setLimits(t, "UNLIMITED", "UNLIMITED")
		routed := routedConnection(t, base, sa)
		defer routed.Close()
		conn, err := routed.Conn(ctx)
		require.NoError(t, err)
		defer conn.Close()

		_, err = conn.ExecContext(ctx, heavy)
		require.NoError(t, err, "unlimited service account should run the query")

		setLimits(t, "1K", "UNLIMITED")
		_, err = conn.ExecContext(ctx, heavy)
		requireMemoryLimitError(t, err, "the tightened limit must apply to the open connection's next query")
	})

	t.Run("the service account's limit replaces the base login's", func(t *testing.T) {
		// A per-principal limit follows the assumed principal: the capped base login runs
		// the query uncapped once it assumes an unlimited service account. The unrouted
		// control proves the base login's cap is live, so the routed success is due to ASSUME.
		setLimits(t, "UNLIMITED", "1K")

		unrouted := baseConnection(t, base)
		defer unrouted.Close()
		_, err := unrouted.Exec(heavy)
		requireMemoryLimitError(t, err, "the capped base login should fail the query without ASSUME")

		routed := routedConnection(t, base, sa)
		defer routed.Close()
		_, err = routed.Exec(heavy)
		require.NoError(t, err, "the assumed service account's limit, not the base login's, should apply")
	})

	t.Run("memory limit still applies after a prior error on the same pooled connection", func(t *testing.T) {
		// Regression guard for the per-connection ASSUME model (review #2/#6): a query that
		// errors — here a memory-limit abort — must NOT silently revert the physical
		// connection to the base login. We pin ONE physical connection, fail a heavy query on
		// it, then prove the SAME reused connection is still capped. If the assumed account
		// had reverted to the (unlimited) base login, the second heavy query would instead
		// succeed. Confirmed against the Enterprise source: ASSUME swaps the per-connection
		// security context's access list, which survives the per-query reset and non-fatal
		// query errors, and reverts only on explicit EXIT, grant revocation, or connection
		// teardown.
		setLimits(t, "1K", "UNLIMITED")
		routed := routedConnection(t, base, sa)
		defer routed.Close()

		conn, err := routed.Conn(ctx)
		require.NoError(t, err)
		defer conn.Close()

		_, err = conn.ExecContext(ctx, heavy)
		requireMemoryLimitError(t, err, "first heavy query on the pinned connection should hit the cap")

		_, err = conn.ExecContext(ctx, heavy)
		requireMemoryLimitError(t, err, "reused connection must still be capped after the prior error")
	})

	t.Run("EXIT SERVICE ACCOUNT reverts to the base login's limit", func(t *testing.T) {
		// The README's reason to cap the base login too: raw SQL can EXIT the assumed account,
		// after which the base login's own limit is what applies.
		setLimits(t, "UNLIMITED", "1K")
		routed := routedConnection(t, base, sa)
		defer routed.Close()
		// Pin a single physical connection so EXIT and the follow-up run on the same session.
		conn, err := routed.Conn(ctx)
		require.NoError(t, err)
		defer conn.Close()

		_, err = conn.ExecContext(ctx, heavy)
		require.NoError(t, err, "the unlimited assumed account should run the query")

		_, err = conn.ExecContext(ctx, fmt.Sprintf("EXIT SERVICE ACCOUNT %s", sa))
		require.NoError(t, err)
		_, err = conn.ExecContext(ctx, heavy)
		requireMemoryLimitError(t, err, "after EXIT the capped base login's limit should apply")
	})

	t.Run("a connection released after EXIT runs its next user's query under the service account", func(t *testing.T) {
		// EXIT changes the session, not just the query that ran it. The pool holds one
		// connection and keeps it idle, so the second user provably borrows the session the
		// first user EXITed; the plugin must assume the account again before that reuse.
		setLimits(t, "1K", "UNLIMITED")
		routed := routedConnectionWithConfig(t, base, sa, `,"maxOpenConnections":1,"maxIdleConnections":1`)
		defer routed.Close()

		conn, err := routed.Conn(ctx)
		require.NoError(t, err)
		_, err = conn.ExecContext(ctx, fmt.Sprintf("EXIT SERVICE ACCOUNT %s", sa))
		require.NoError(t, err)
		_, err = conn.ExecContext(ctx, heavy)
		require.NoError(t, err, "after EXIT the rest of the session runs as the unlimited base login")
		require.NoError(t, conn.Close())

		_, err = routed.ExecContext(ctx, heavy)
		requireMemoryLimitError(t, err, "the next user of the released connection must run under the service account's limit")
		assert.Equal(t, 1, routed.Stats().OpenConnections)
	})

	t.Run("current_user() differs from session_user() exactly while an account is assumed", func(t *testing.T) {
		// The base login pool relies on this to detect a connection a query left assuming an
		// account. A clean session must compare equal, or every reuse would reconnect.
		identity := func(t *testing.T, q interface {
			QueryRowContext(context.Context, string, ...any) *sql.Row
		}) (current, session string) {
			t.Helper()
			require.NoError(t, q.QueryRowContext(ctx, "SELECT current_user(), session_user()").Scan(&current, &session))
			return current, session
		}

		current, session := identity(t, admin)
		assert.Equal(t, current, session, "the built-in admin's clean session")

		unrouted := baseConnection(t, base)
		defer unrouted.Close()
		conn, err := unrouted.Conn(ctx)
		require.NoError(t, err)
		defer conn.Close()

		current, session = identity(t, conn)
		assert.Equal(t, []string{base.username, base.username}, []string{current, session})
		_, err = conn.ExecContext(ctx, fmt.Sprintf("ASSUME SERVICE ACCOUNT %s", sa))
		require.NoError(t, err)
		current, session = identity(t, conn)
		assert.Equal(t, []string{sa, base.username}, []string{current, session})
		_, err = conn.ExecContext(ctx, fmt.Sprintf("EXIT SERVICE ACCOUNT %s", sa))
		require.NoError(t, err)
		current, session = identity(t, conn)
		assert.Equal(t, []string{base.username, base.username}, []string{current, session})
	})

	t.Run("a base login connection released while assuming an account runs its next query as the login", func(t *testing.T) {
		// Unrouted queries (unmapped users with no default account) share the base login pool,
		// where raw SQL can ASSUME an account the login is granted. With one pooled connection,
		// the next query would inherit that account unless the pool drops the connection.
		setLimits(t, "1K", "UNLIMITED")
		unrouted := baseConnectionWithConfig(t, base, `,"maxOpenConnections":1,"maxIdleConnections":1`)
		defer unrouted.Close()

		conn, err := unrouted.Conn(ctx)
		require.NoError(t, err)
		_, err = conn.ExecContext(ctx, fmt.Sprintf("ASSUME SERVICE ACCOUNT %s", sa))
		require.NoError(t, err)
		_, err = conn.ExecContext(ctx, heavy)
		requireMemoryLimitError(t, err, "after ASSUME the rest of the session runs under the account's limit")
		require.NoError(t, conn.Close())

		_, err = unrouted.ExecContext(ctx, heavy)
		require.NoError(t, err, "the next query on the base login pool must run as the unlimited login")
	})
}

// TestPostCheckHealthRoutingIntegration verifies review #1's fix end-to-end against a
// running QuestDB *Enterprise* instance: Save & Test (PostCheckHealth) must actually run an
// ASSUME for the default service account, so a routing misconfiguration fails here rather
// than passing the green base-login check and breaking only on routed dashboard queries.
// Enterprise-gated, and run through a dedicated non-admin login, for the same reasons as
// TestServiceAccountRoutingIntegration.
func TestPostCheckHealthRoutingIntegration(t *testing.T) {
	if strings.ToLower(getEnv("QUESTDB_ENTERPRISE", "false")) != "true" {
		t.Skip("requires QuestDB Enterprise; set QUESTDB_ENTERPRISE=true and point QUESTDB_HOST/PORT at it")
	}

	admin := setupConnection(t)
	// Registered before the fixtures, so it runs after their DROP cleanups (a deferred Close
	// would run first and silently fail them).
	t.Cleanup(func() { _ = admin.Close() })

	const sa = "sa_grafana_health_it"
	const saNoPgwire = "sa_grafana_health_nopg_it"
	base := createRoutingLogin(t, admin, "grafana_health_it_base")
	createServiceAccount(t, admin, sa)

	ctx := context.Background()
	h := &plugin.QuestDB{}

	// Review #1's exact failure scenario: the account exists but the data source login was
	// never GRANTed ASSUME on it. The base Save & Test (default pool, no ASSUME) is green, so
	// without PostCheckHealth this misconfiguration would surface only on routed queries.
	t.Run("unhealthy when GRANT ASSUME is missing", func(t *testing.T) {
		res := h.PostCheckHealth(ctx, healthCheckRequest(t, base, sa))
		require.NotNil(t, res, "missing GRANT ASSUME must fail Save & Test")
		assert.Equal(t, backend.HealthStatusError, res.Status)
		assert.Contains(t, res.Message, "User cannot assume service account")
	})

	t.Run("unhealthy when the service account lacks PGWIRE", func(t *testing.T) {
		// ASSUME over PGWire also checks the service account's own endpoint permission, so a
		// granted account without GRANT PGWIRE still breaks every routed query.
		_, _ = admin.Exec("DROP SERVICE ACCOUNT " + saNoPgwire) // best-effort cleanup from a prior run
		mustExec(t, admin, "CREATE SERVICE ACCOUNT "+saNoPgwire)
		t.Cleanup(func() { _, _ = admin.Exec("DROP SERVICE ACCOUNT " + saNoPgwire) })
		mustExec(t, admin, fmt.Sprintf("GRANT ASSUME SERVICE ACCOUNT %s TO %s", saNoPgwire, base.username))

		res := h.PostCheckHealth(ctx, healthCheckRequest(t, base, saNoPgwire))
		require.NotNil(t, res, "a service account without PGWIRE must fail Save & Test")
		assert.Equal(t, backend.HealthStatusError, res.Status)
		assert.Contains(t, res.Message, "Access denied for "+saNoPgwire+" [PGWIRE]")
	})

	t.Run("healthy once the default account is granted and assumable", func(t *testing.T) {
		mustExec(t, admin, fmt.Sprintf("GRANT ASSUME SERVICE ACCOUNT %s TO %s", sa, base.username))
		res := h.PostCheckHealth(ctx, healthCheckRequest(t, base, sa))
		assert.Nil(t, res, "a granted, assumable default account should pass Save & Test")
	})

	t.Run("unhealthy when the default account does not exist", func(t *testing.T) {
		res := h.PostCheckHealth(ctx, healthCheckRequest(t, base, "sa_does_not_exist_xyz"))
		require.NotNil(t, res, "a non-existent default account must fail Save & Test")
		assert.Equal(t, backend.HealthStatusError, res.Status)
		assert.Contains(t, res.Message, "Service account does not exist")
	})
}

// routingLogin is the data source login the routing integration tests connect as.
type routingLogin struct {
	username string
	password string
}

func mustExec(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	_, err := db.Exec(q)
	require.NoError(t, err, q)
}

// createRoutingLogin creates a non-admin user with PGWire access to act as the data source's
// base login, and drops it when the test ends.
func createRoutingLogin(t *testing.T, admin *sql.DB, username string) routingLogin {
	t.Helper()
	login := routingLogin{username: username, password: "grafana_it_pwd"}
	_, _ = admin.Exec("DROP USER " + username) // best-effort cleanup from a prior run
	mustExec(t, admin, fmt.Sprintf("CREATE USER %s WITH PASSWORD '%s'", username, login.password))
	t.Cleanup(func() { _, _ = admin.Exec("DROP USER " + username) })
	mustExec(t, admin, "GRANT PGWIRE TO "+username)
	return login
}

// createServiceAccount creates a service account that can be assumed over PGWire, and drops
// it when the test ends. It needs no table grants: the tests only query long_sequence().
func createServiceAccount(t *testing.T, admin *sql.DB, name string) {
	t.Helper()
	_, _ = admin.Exec("DROP SERVICE ACCOUNT " + name) // best-effort cleanup from a prior run
	mustExec(t, admin, "CREATE SERVICE ACCOUNT "+name)
	t.Cleanup(func() { _, _ = admin.Exec("DROP SERVICE ACCOUNT " + name) })
	mustExec(t, admin, "GRANT PGWIRE TO "+name)
}

// routingTestConfig builds the shared jsonData + decrypted-secure map pointing at the test
// QuestDB instance as login, with service-account routing enabled. extraJSON is appended
// verbatim as additional jsonData fields (e.g. `,"defaultServiceAccount":"sa"`); pass "" for
// none.
func routingTestConfig(t *testing.T, login routingLogin, extraJSON string) backend.DataSourceInstanceSettings {
	t.Helper()
	host := getEnv("QUESTDB_HOST", "localhost")
	port := getEnv("QUESTDB_PORT", "8812")
	tlsEnabled := getEnv("QUESTDB_TLS_ENABLED", "false")

	secure := map[string]string{"password": login.password}
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
		host, port, login.username, tlsMode, tlsMethod, extraJSON)
	return backend.DataSourceInstanceSettings{
		JSONData:                []byte(jsonData),
		DecryptedSecureJSONData: secure,
	}
}

// healthCheckRequest builds a CheckHealthRequest whose data source logs in as login and
// enables routing with the given default service account, pointed at the test QuestDB
// instance (mirrors how Grafana invokes CheckHealth with the decrypted password present).
func healthCheckRequest(t *testing.T, login routingLogin, defaultSA string) *backend.CheckHealthRequest {
	t.Helper()
	cfg := routingTestConfig(t, login, fmt.Sprintf(`,"defaultServiceAccount":%q`, defaultSA))
	return &backend.CheckHealthRequest{
		PluginContext: backend.PluginContext{DataSourceInstanceSettings: &cfg},
	}
}

// routedConnection opens a *sql.DB through the plugin's Connect with service-account
// routing enabled, mirroring how sqlds would call it with a stamped connectionArgs
// message. Every physical connection in the returned pool logs in as login and assumes sa.
func routedConnection(t *testing.T, login routingLogin, sa string) *sql.DB {
	t.Helper()
	return routedConnectionWithConfig(t, login, sa, "")
}

// routedConnectionWithConfig is routedConnection with extraJSON appended to the data source's
// jsonData, as in routingTestConfig.
func routedConnectionWithConfig(t *testing.T, login routingLogin, sa, extraJSON string) *sql.DB {
	t.Helper()
	msg, err := json.Marshal(map[string]string{"serviceAccount": sa})
	require.NoError(t, err)

	db, err := (&plugin.QuestDB{}).Connect(context.Background(), routingTestConfig(t, login, extraJSON), msg)
	require.NoError(t, err)
	return db
}

// baseConnection opens the pool sqlds uses for queries without connectionArgs: it logs in
// as login and assumes nothing, even though routing is enabled on the data source.
func baseConnection(t *testing.T, login routingLogin) *sql.DB {
	t.Helper()
	return baseConnectionWithConfig(t, login, "")
}

// baseConnectionWithConfig is baseConnection with extraJSON appended to the data source's
// jsonData, as in routingTestConfig.
func baseConnectionWithConfig(t *testing.T, login routingLogin, extraJSON string) *sql.DB {
	t.Helper()
	db, err := (&plugin.QuestDB{}).Connect(context.Background(), routingTestConfig(t, login, extraJSON), nil)
	require.NoError(t, err)
	return db
}

// requireMemoryLimitError asserts that err is QuestDB's per-query memory-limit abort: a
// server-side pq error (a normal PGWire ErrorResponse, so the connection stays open and
// lib/pq returns *pq.Error) whose message is the per-query cap specifically. Requiring
// *pq.Error rather than just "some error" proves the physical connection stayed alive — so a
// passing reuse case means the assumed service account survived the prior error rather than
// the connection having silently died — and matching the message proves the failure is the
// cap, not some other server-side rejection. A principal's MEMORY LIMIT is enforced by the
// query's memory tracker, which emits "query memory limit exceeded [workload=QUERY, ...]";
// the process-wide cap's "global RSS memory limit exceeded [...]" deliberately does not match
// (see io.questdb.std.Unsafe in questdb).
func requireMemoryLimitError(t *testing.T, err error, msg string) {
	t.Helper()
	require.Error(t, err, msg)
	var pqErr *pq.Error
	require.ErrorAs(t, err, &pqErr, msg+" (expected a server-side pq error, not a dropped connection)")
	assert.Contains(t, pqErr.Message, "query memory limit exceeded",
		msg+" (expected a QuestDB per-query memory-limit abort, not another server-side error)")
}
