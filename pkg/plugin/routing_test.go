package plugin

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/sqlds/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadServiceAccountSettings(t *testing.T) {
	t.Run("parses routing fields", func(t *testing.T) {
		cfg := backend.DataSourceInstanceSettings{
			JSONData: []byte(`{ "server": "h", "port": 8812, "username": "u",
				"serviceAccountRoutingEnabled": true, "defaultServiceAccount": "sa_default",
				"serviceAccountMappings": [
					{ "grafanaUser": "john", "serviceAccount": "sa_analysts" },
					{ "grafanaUser": "jane", "serviceAccount": "sa_analysts" }
				],
				"groupsClaim": "roles",
				"serviceAccountGroupMappings": [
					{ "group": "Analysts", "serviceAccount": "sa_analysts" },
					{ "group": "Execs", "serviceAccount": "sa_execs" }
				] }`),
			DecryptedSecureJSONData: map[string]string{"password": "p"},
		}
		s, err := LoadSettings(cfg)
		require.NoError(t, err)
		assert.True(t, s.ServiceAccountRoutingEnabled)
		assert.Equal(t, "sa_default", s.DefaultServiceAccount)
		assert.Equal(t, []ServiceAccountMapping{
			{GrafanaUser: "john", ServiceAccount: "sa_analysts"},
			{GrafanaUser: "jane", ServiceAccount: "sa_analysts"},
		}, s.ServiceAccountMappings)
		assert.Equal(t, "roles", s.GroupsClaim)
		assert.Equal(t, []ServiceAccountGroupMapping{
			{Group: "Analysts", ServiceAccount: "sa_analysts"},
			{Group: "Execs", ServiceAccount: "sa_execs"},
		}, s.ServiceAccountGroupMappings)
	})

	t.Run("defaults to disabled when fields absent", func(t *testing.T) {
		cfg := backend.DataSourceInstanceSettings{
			JSONData:                []byte(`{ "server": "h", "port": 8812, "username": "u" }`),
			DecryptedSecureJSONData: map[string]string{"password": "p"},
		}
		s, err := LoadSettings(cfg)
		require.NoError(t, err)
		assert.False(t, s.ServiceAccountRoutingEnabled)
		assert.Equal(t, "", s.DefaultServiceAccount)
		assert.Nil(t, s.ServiceAccountMappings)
		assert.Nil(t, s.ServiceAccountGroupMappings)
		assert.Equal(t, "", s.GroupsClaim)
	})

	t.Run("LoadServiceAccountSettings parses without requiring credentials", func(t *testing.T) {
		// No server/port/username/password — routing config must still parse.
		cfg := backend.DataSourceInstanceSettings{
			JSONData: []byte(`{ "serviceAccountRoutingEnabled": true, "defaultServiceAccount": "sa_default",
				"serviceAccountMappings": [{ "grafanaUser": "john", "serviceAccount": "sa_analysts" }] }`),
		}
		s := LoadServiceAccountSettings(cfg)
		assert.True(t, s.ServiceAccountRoutingEnabled)
		assert.Equal(t, "sa_default", s.DefaultServiceAccount)
		assert.Equal(t, []ServiceAccountMapping{{GrafanaUser: "john", ServiceAccount: "sa_analysts"}}, s.ServiceAccountMappings)
	})

	t.Run("type mismatch returns an error but still parses what it can", func(t *testing.T) {
		// Hand-provisioned YAML can produce a type mismatch the frontend never would (here a
		// numeric serviceAccount). json.Unmarshal records the error yet keeps populating: the
		// per-query path runs with this partial parse (the bad row's account is blanked, so
		// resolveServiceAccount skips it rather than dropping the user onto the uncapped base
		// login unguarded), while PostCheckHealth surfaces the error at Save & Test.
		var s Settings
		err := applyServiceAccountSettings(&s, []byte(`{
			"serviceAccountRoutingEnabled": true,
			"defaultServiceAccount": "sa_default",
			"serviceAccountMappings": [{ "grafanaUser": "john", "serviceAccount": 123 }]
		}`))
		require.Error(t, err)
		assert.True(t, s.ServiceAccountRoutingEnabled)
		assert.Equal(t, "sa_default", s.DefaultServiceAccount)
		require.Len(t, s.ServiceAccountMappings, 1)
		assert.Equal(t, "", s.ServiceAccountMappings[0].ServiceAccount)
	})
}

// resolveServiceAccount is a test-only convenience wrapper: the eager form of
// resolveServiceAccountLazy that takes already-resolved groups. Production code calls
// resolveServiceAccountLazy directly so a forwarded OIDC token is decoded only when the
// group step is actually reached; the tests don't need that laziness.
func (settings *Settings) resolveServiceAccount(user *backend.User, groups []string) string {
	return settings.resolveServiceAccountLazy(user, func() []string { return groups })
}

func TestResolveServiceAccount(t *testing.T) {
	settings := Settings{serviceAccountConfig: serviceAccountConfig{
		DefaultServiceAccount: "sa_default",
		ServiceAccountMappings: []ServiceAccountMapping{
			{GrafanaUser: "john", ServiceAccount: "sa_analysts"},
			{GrafanaUser: "ceo", ServiceAccount: "sa_execs"},
		},
	}}

	tests := []struct {
		name string
		user *backend.User
		want string
	}{
		{name: "mapped user", user: &backend.User{Login: "john"}, want: "sa_analysts"},
		{name: "case-insensitive match", user: &backend.User{Login: "JoHn"}, want: "sa_analysts"},
		{name: "another mapped user", user: &backend.User{Login: "ceo"}, want: "sa_execs"},
		{name: "unmapped user falls back to default", user: &backend.User{Login: "nobody"}, want: "sa_default"},
		{name: "nil user falls back to default", user: nil, want: "sa_default"},
		{name: "empty login falls back to default", user: &backend.User{Login: ""}, want: "sa_default"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, settings.resolveServiceAccount(tt.user, nil))
		})
	}

	t.Run("empty default and no match returns empty", func(t *testing.T) {
		s := Settings{serviceAccountConfig: serviceAccountConfig{ServiceAccountMappings: []ServiceAccountMapping{{GrafanaUser: "john", ServiceAccount: "sa_analysts"}}}}
		assert.Equal(t, "", s.resolveServiceAccount(&backend.User{Login: "nobody"}, nil))
		assert.Equal(t, "", s.resolveServiceAccount(nil, nil))
	})

	t.Run("blank mapping service account falls through to the default", func(t *testing.T) {
		s := Settings{serviceAccountConfig: serviceAccountConfig{
			DefaultServiceAccount:  "sa_default",
			ServiceAccountMappings: []ServiceAccountMapping{{GrafanaUser: "john", ServiceAccount: "   "}},
		}}
		// Without a default, a blank mapping must not assume the base login implicitly.
		assert.Equal(t, "sa_default", s.resolveServiceAccount(&backend.User{Login: "john"}, nil))
		s.DefaultServiceAccount = ""
		assert.Equal(t, "", s.resolveServiceAccount(&backend.User{Login: "john"}, nil))
	})

	t.Run("a later non-blank row for the same user still matches", func(t *testing.T) {
		s := Settings{serviceAccountConfig: serviceAccountConfig{ServiceAccountMappings: []ServiceAccountMapping{
			{GrafanaUser: "john", ServiceAccount: ""},
			{GrafanaUser: "john", ServiceAccount: "sa_analysts"},
		}}}
		assert.Equal(t, "sa_analysts", s.resolveServiceAccount(&backend.User{Login: "john"}, nil))
	})

	t.Run("whitespace is trimmed from resolved names", func(t *testing.T) {
		s := Settings{serviceAccountConfig: serviceAccountConfig{
			DefaultServiceAccount:  "  sa_default  ",
			ServiceAccountMappings: []ServiceAccountMapping{{GrafanaUser: "john", ServiceAccount: "  sa_analysts  "}},
		}}
		assert.Equal(t, "sa_analysts", s.resolveServiceAccount(&backend.User{Login: "john"}, nil))
		assert.Equal(t, "sa_default", s.resolveServiceAccount(&backend.User{Login: "nobody"}, nil))
	})

	t.Run("whitespace-only default resolves to empty", func(t *testing.T) {
		s := Settings{serviceAccountConfig: serviceAccountConfig{DefaultServiceAccount: "   "}}
		assert.Equal(t, "", s.resolveServiceAccount(nil, nil))
		assert.Equal(t, "", s.resolveServiceAccount(&backend.User{Login: "nobody"}, nil))
	})

	t.Run("whitespace around the mapping username still matches", func(t *testing.T) {
		s := Settings{serviceAccountConfig: serviceAccountConfig{ServiceAccountMappings: []ServiceAccountMapping{{GrafanaUser: "  john  ", ServiceAccount: "sa_analysts"}}}}
		assert.Equal(t, "sa_analysts", s.resolveServiceAccount(&backend.User{Login: "john"}, nil))
		assert.Equal(t, "sa_analysts", s.resolveServiceAccount(&backend.User{Login: "  john  "}, nil))
	})
}

func TestResolveServiceAccountGroups(t *testing.T) {
	settings := Settings{serviceAccountConfig: serviceAccountConfig{
		DefaultServiceAccount:  "sa_default",
		ServiceAccountMappings: []ServiceAccountMapping{{GrafanaUser: "john", ServiceAccount: "sa_user"}},
		ServiceAccountGroupMappings: []ServiceAccountGroupMapping{
			{Group: "Analysts", ServiceAccount: "sa_analysts"},
			{Group: "Execs", ServiceAccount: "sa_execs"},
		},
	}}

	t.Run("group match when the user is unmapped", func(t *testing.T) {
		assert.Equal(t, "sa_analysts", settings.resolveServiceAccount(&backend.User{Login: "nobody"}, []string{"Analysts"}))
	})

	t.Run("group match is case-insensitive", func(t *testing.T) {
		assert.Equal(t, "sa_execs", settings.resolveServiceAccount(&backend.User{Login: "nobody"}, []string{"execs"}))
	})

	t.Run("username mapping beats group mapping", func(t *testing.T) {
		// john has a username mapping; even though his token also carries a mapped group,
		// the more specific username mapping wins.
		assert.Equal(t, "sa_user", settings.resolveServiceAccount(&backend.User{Login: "john"}, []string{"Analysts"}))
	})

	t.Run("first configured group wins for multi-group membership", func(t *testing.T) {
		// Member of both Execs and Analysts: Analysts is configured first, so it wins.
		assert.Equal(t, "sa_analysts", settings.resolveServiceAccount(&backend.User{Login: "nobody"}, []string{"Execs", "Analysts"}))
	})

	t.Run("no matching group falls back to default", func(t *testing.T) {
		assert.Equal(t, "sa_default", settings.resolveServiceAccount(&backend.User{Login: "nobody"}, []string{"Other"}))
	})

	t.Run("nil and empty groups fall back to default", func(t *testing.T) {
		assert.Equal(t, "sa_default", settings.resolveServiceAccount(&backend.User{Login: "nobody"}, nil))
		assert.Equal(t, "sa_default", settings.resolveServiceAccount(&backend.User{Login: "nobody"}, []string{}))
	})

	t.Run("nil user (alerting) with no groups falls back to default", func(t *testing.T) {
		assert.Equal(t, "sa_default", settings.resolveServiceAccount(nil, nil))
	})

	t.Run("blank group service account is skipped", func(t *testing.T) {
		s := Settings{serviceAccountConfig: serviceAccountConfig{
			DefaultServiceAccount:       "sa_default",
			ServiceAccountGroupMappings: []ServiceAccountGroupMapping{{Group: "Analysts", ServiceAccount: "  "}},
		}}
		assert.Equal(t, "sa_default", s.resolveServiceAccount(&backend.User{Login: "nobody"}, []string{"Analysts"}))
	})

	t.Run("blank group name matches nothing", func(t *testing.T) {
		s := Settings{serviceAccountConfig: serviceAccountConfig{ServiceAccountGroupMappings: []ServiceAccountGroupMapping{{Group: "  ", ServiceAccount: "sa_blank"}}}}
		assert.Equal(t, "", s.resolveServiceAccount(&backend.User{Login: "nobody"}, []string{"", "Analysts"}))
	})

	t.Run("group names and resolved SA are trimmed on both sides", func(t *testing.T) {
		s := Settings{serviceAccountConfig: serviceAccountConfig{ServiceAccountGroupMappings: []ServiceAccountGroupMapping{{Group: "  Analysts  ", ServiceAccount: "  sa_analysts  "}}}}
		assert.Equal(t, "sa_analysts", s.resolveServiceAccount(&backend.User{Login: "nobody"}, []string{"  Analysts  "}))
	})
}

// TestResolveServiceAccountLazy locks in that the groups callback is consulted only when
// resolution actually reaches the group step, so a forwarded OIDC token is not decoded for
// users a username mapping already covers (nor when no group mappings are configured).
func TestResolveServiceAccountLazy(t *testing.T) {
	withMappings := Settings{serviceAccountConfig: serviceAccountConfig{
		DefaultServiceAccount:       "sa_default",
		ServiceAccountMappings:      []ServiceAccountMapping{{GrafanaUser: "john", ServiceAccount: "sa_user"}},
		ServiceAccountGroupMappings: []ServiceAccountGroupMapping{{Group: "Analysts", ServiceAccount: "sa_grp"}},
	}}

	t.Run("groups func not called when a username mapping matches", func(t *testing.T) {
		called := false
		sa := withMappings.resolveServiceAccountLazy(&backend.User{Login: "john"}, func() []string {
			called = true
			return []string{"Analysts"}
		})
		assert.Equal(t, "sa_user", sa)
		assert.False(t, called, "groups func must not run once a username mapping has won")
	})

	t.Run("groups func not called when no group mappings are configured", func(t *testing.T) {
		s := Settings{serviceAccountConfig: serviceAccountConfig{
			DefaultServiceAccount:  "sa_default",
			ServiceAccountMappings: []ServiceAccountMapping{{GrafanaUser: "john", ServiceAccount: "sa_user"}},
		}}
		called := false
		sa := s.resolveServiceAccountLazy(&backend.User{Login: "nobody"}, func() []string {
			called = true
			return []string{"Analysts"}
		})
		assert.Equal(t, "sa_default", sa)
		assert.False(t, called, "groups func must not run when there are no group mappings to match")
	})

	t.Run("groups func called and used when username misses and group mappings exist", func(t *testing.T) {
		called := false
		sa := withMappings.resolveServiceAccountLazy(&backend.User{Login: "nobody"}, func() []string {
			called = true
			return []string{"Analysts"}
		})
		assert.Equal(t, "sa_grp", sa)
		assert.True(t, called, "groups func must run to reach a group mapping")
	})

	t.Run("nil groups func is tolerated and falls through to the default", func(t *testing.T) {
		assert.Equal(t, "sa_default", withMappings.resolveServiceAccountLazy(&backend.User{Login: "nobody"}, nil))
	})
}

// makeIDToken hand-crafts a JWT (header.payload.signature) carrying the given claims. The
// signature segment is a placeholder — extractGroups does not verify it (design D11/D12).
func makeIDToken(t *testing.T, claims map[string]interface{}) string {
	t.Helper()
	enc := base64.RawURLEncoding
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	return enc.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`)) + "." + enc.EncodeToString(payload) + ".sig"
}

func TestExtractGroups(t *testing.T) {
	t.Run("reads the default groups claim", func(t *testing.T) {
		token := makeIDToken(t, map[string]interface{}{"groups": []string{"Analysts", "Execs"}})
		assert.Equal(t, []string{"Analysts", "Execs"}, extractGroups(token, ""))
		assert.Equal(t, []string{"Analysts", "Execs"}, extractGroups(token, "groups"))
	})

	t.Run("reads a custom claim name", func(t *testing.T) {
		token := makeIDToken(t, map[string]interface{}{"roles": []string{"Admins"}})
		assert.Equal(t, []string{"Admins"}, extractGroups(token, "roles"))
		assert.Nil(t, extractGroups(token, "groups")) // default claim absent in this token
	})

	t.Run("missing claim returns nil", func(t *testing.T) {
		token := makeIDToken(t, map[string]interface{}{"sub": "u1"})
		assert.Nil(t, extractGroups(token, "groups"))
	})

	t.Run("empty header returns nil", func(t *testing.T) {
		assert.Nil(t, extractGroups("", "groups"))
	})

	t.Run("non-JWT strings return nil", func(t *testing.T) {
		for _, s := range []string{"not-a-jwt", "a.b", "a.b.c.d", "header..sig"} {
			assert.Nil(t, extractGroups(s, "groups"), s)
		}
	})

	t.Run("non-base64url payload returns nil", func(t *testing.T) {
		assert.Nil(t, extractGroups("aGVhZGVy.!!!not-base64!!!.sig", "groups"))
	})

	t.Run("payload that is not JSON returns nil", func(t *testing.T) {
		enc := base64.RawURLEncoding
		token := enc.EncodeToString([]byte("hdr")) + "." + enc.EncodeToString([]byte("not json")) + ".sig"
		assert.Nil(t, extractGroups(token, "groups"))
	})

	t.Run("claim that is neither a string nor a string array returns nil", func(t *testing.T) {
		assert.Nil(t, extractGroups(makeIDToken(t, map[string]interface{}{"groups": []int{1, 2}}), "groups"))
		assert.Nil(t, extractGroups(makeIDToken(t, map[string]interface{}{"groups": map[string]interface{}{"a": 1}}), "groups"))
		assert.Nil(t, extractGroups(makeIDToken(t, map[string]interface{}{"groups": 123}), "groups"))
	})

	t.Run("single string claim is treated as a one-element group list", func(t *testing.T) {
		// Some IdPs serialize a single group as a scalar string rather than a 1-element array.
		assert.Equal(t, []string{"Analysts"}, extractGroups(makeIDToken(t, map[string]interface{}{"groups": "Analysts"}), "groups"))
	})

	t.Run("empty groups array returns an empty (non-nil) slice", func(t *testing.T) {
		assert.Equal(t, []string{}, extractGroups(makeIDToken(t, map[string]interface{}{"groups": []string{}}), "groups"))
	})
}

func TestBuildAssumeStatement(t *testing.T) {
	t.Run("valid names", func(t *testing.T) {
		valid := map[string]string{
			"sa_analysts": `ASSUME SERVICE ACCOUNT "sa_analysts"`,
			"sa-execs":    `ASSUME SERVICE ACCOUNT "sa-execs"`,
			"team.a_1":    `ASSUME SERVICE ACCOUNT "team.a_1"`,
			// QuestDB allows these (mirrored denylist), so the plugin must too — see
			// AccessListUtilsTest in questdb-enterprise. Notably a space, '@' and email form.
			"john.doe@mail.com": `ASSUME SERVICE ACCOUNT "john.doe@mail.com"`,
			"data team":         `ASSUME SERVICE ACCOUNT "data team"`,
			"team!1":            `ASSUME SERVICE ACCOUNT "team!1"`,
			"sa^x":              `ASSUME SERVICE ACCOUNT "sa^x"`,
			"sa;ok":             `ASSUME SERVICE ACCOUNT "sa;ok"`,
		}
		for name, want := range valid {
			got, err := buildAssumeStatement(name)
			require.NoError(t, err, name)
			assert.Equal(t, want, got)
		}
	})

	t.Run("empty name is a no-op", func(t *testing.T) {
		got, err := buildAssumeStatement("")
		require.NoError(t, err)
		assert.Equal(t, "", got)
	})

	t.Run("rejects injection / malformed names", func(t *testing.T) {
		// Every entry contains a character QuestDB itself forbids in entity names
		// (AccessListUtils.validateEntityName); '"' and '\\' in particular keep the quoted
		// identifier injection-safe. A space or ';' is intentionally NOT here — QuestDB
		// permits those, and the "valid names" case above asserts the plugin does too.
		bad := []string{
			`sa"; DROP TABLE x;--`, // contains '"'
			`sa"a`,                 // '"'
			`sa\a`,                 // '\'
			`sa'a`,                 // '\''
			"sa\na",                // newline (control char)
			"sa\ta",                // tab (control char)
			`"`,                    // '"'
			`sa)`,                  // ')'
			`sa(x`,                 // '('
			`sa/x`,                 // '/'
			`sa:x`,                 // ':'
			`sa,x`,                 // ','
			`sa+x`,                 // '+'
			`sa*x`,                 // '*'
			`sa%x`,                 // '%'
			`sa~x`,                 // '~'
			`sa?x`,                 // '?'
		}
		for _, name := range bad {
			_, err := buildAssumeStatement(name)
			assert.Error(t, err, name)
		}
	})
}

// --- fakes for the connector wrapper ---

const fakeBaseLogin = "baseuser"

// fakeConnector opens fakeSessions logged in as fakeBaseLogin, or returns conn when set.
type fakeConnector struct {
	conn       driver.Conn
	connectErr error

	mu        sync.Mutex
	assumeErr error // returned by ASSUME on every session, e.g. after a revoked grant
	opened    []*fakeSession
}

func (c *fakeConnector) Connect(_ context.Context) (driver.Conn, error) {
	if c.connectErr != nil {
		return nil, c.connectErr
	}
	if c.conn != nil {
		return c.conn, nil
	}
	s := &fakeSession{server: c, identity: fakeBaseLogin}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.opened = append(c.opened, s)
	return s, nil
}

func (c *fakeConnector) Driver() driver.Driver { return nil }

func (c *fakeConnector) setAssumeErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.assumeErr = err
}

func (c *fakeConnector) openedSessions() []*fakeSession {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*fakeSession(nil), c.opened...)
}

// fakeSession is a stateful stand-in for a lib/pq connection to QuestDB. ASSUME and EXIT
// change the identity that `SELECT current_user()` reports, as they do on the server, and
// ResetSession, like lib/pq's, does not restore it.
type fakeSession struct {
	server *fakeConnector

	mu       sync.Mutex
	identity string
	execs    []string
	closed   bool
}

var _ pgConn = (*fakeSession)(nil)

func (s *fakeSession) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not implemented") }
func (s *fakeSession) Begin() (driver.Tx, error)           { return nil, errors.New("not implemented") }
func (s *fakeSession) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return nil, errors.New("not implemented")
}
func (s *fakeSession) PrepareContext(context.Context, string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}
func (s *fakeSession) Ping(context.Context) error         { return nil }
func (s *fakeSession) ResetSession(context.Context) error { return nil }
func (s *fakeSession) IsValid() bool                      { return !s.isClosed() }

func (s *fakeSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *fakeSession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *fakeSession) executed() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.execs...)
}

func (s *fakeSession) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.execs = append(s.execs, query)
	if account, ok := strings.CutPrefix(query, "ASSUME SERVICE ACCOUNT "); ok {
		s.server.mu.Lock()
		err := s.server.assumeErr
		s.server.mu.Unlock()
		if err != nil {
			return nil, err
		}
		s.identity = strings.Trim(account, `"`)
	} else if strings.HasPrefix(query, "EXIT SERVICE ACCOUNT ") {
		s.identity = fakeBaseLogin
	}
	return driver.RowsAffected(0), nil
}

func (s *fakeSession) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if query != "SELECT current_user()" {
		return nil, errors.New("not implemented")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return &singleValueRows{value: s.identity}, nil
}

type singleValueRows struct {
	value string
	done  bool
}

func (r *singleValueRows) Columns() []string { return []string{"current_user"} }
func (r *singleValueRows) Close() error      { return nil }
func (r *singleValueRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = r.value
	return nil
}

// plainConn implements only driver.Conn.
type plainConn struct{ closed bool }

func (c *plainConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not implemented") }
func (c *plainConn) Close() error                        { c.closed = true; return nil }
func (c *plainConn) Begin() (driver.Tx, error)           { return nil, errors.New("not implemented") }

func TestAssumeServiceAccountConnector(t *testing.T) {
	const stmt = `ASSUME SERVICE ACCOUNT "sa_a"`

	t.Run("runs ASSUME and returns the wrapped conn", func(t *testing.T) {
		fc := &fakeConnector{}
		asc := &assumeServiceAccountConnector{base: fc, stmt: stmt}
		got, err := asc.Connect(context.Background())
		require.NoError(t, err)
		sessions := fc.openedSessions()
		require.Len(t, sessions, 1)
		wrapped, ok := got.(*assumedConn)
		require.True(t, ok, "a routed connection must be wrapped so reuse re-assumes the account")
		assert.Same(t, sessions[0], wrapped.pgConn)
		assert.Equal(t, []string{stmt}, sessions[0].executed())
		assert.False(t, sessions[0].isClosed())
	})

	t.Run("closes conn and propagates exec error", func(t *testing.T) {
		fc := &fakeConnector{assumeErr: errors.New("boom")}
		asc := &assumeServiceAccountConnector{base: fc, stmt: stmt}
		_, err := asc.Connect(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to assume service account")
		assert.Contains(t, err.Error(), "boom")
		require.Len(t, fc.openedSessions(), 1)
		assert.True(t, fc.openedSessions()[0].isClosed())
	})

	t.Run("errors when conn lacks the lib/pq interfaces", func(t *testing.T) {
		pc := &plainConn{}
		asc := &assumeServiceAccountConnector{base: &fakeConnector{conn: pc}, stmt: stmt}
		_, err := asc.Connect(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not support the interfaces required")
		assert.True(t, pc.closed)
	})

	t.Run("propagates base connect error", func(t *testing.T) {
		asc := &assumeServiceAccountConnector{base: &fakeConnector{connectErr: errors.New("dial fail")}, stmt: stmt}
		_, err := asc.Connect(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dial fail")
	})

	t.Run("closing the pool runs onClose", func(t *testing.T) {
		calls := 0
		db := sql.OpenDB(&assumeServiceAccountConnector{base: &fakeConnector{}, stmt: stmt, onClose: func() { calls++ }})
		require.NoError(t, db.Close())
		require.NoError(t, db.Close())
		assert.Equal(t, 1, calls)
	})
}

// TestAssumedConnectionReuse drives the connector wrapper through database/sql pooling: a
// session's identity can be changed by the SQL a user runs, and must not carry over to the
// next user the pool hands the same connection to.
func TestAssumedConnectionReuse(t *testing.T) {
	const account = "sa_analysts"
	ctx := context.Background()

	// newPool returns a one-connection pool that keeps its connection idle between uses, so
	// every checkout after the first reuses the same session.
	newPool := func(t *testing.T) (*sql.DB, *fakeConnector) {
		t.Helper()
		fc := &fakeConnector{}
		stmt, err := buildAssumeStatement(account)
		require.NoError(t, err)
		db := sql.OpenDB(&assumeServiceAccountConnector{base: fc, stmt: stmt})
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		t.Cleanup(func() { _ = db.Close() })
		return db, fc
	}
	currentUser := func(t *testing.T, q interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	}) string {
		t.Helper()
		var who string
		require.NoError(t, q.QueryRowContext(ctx, "SELECT current_user()").Scan(&who))
		return who
	}

	for _, tc := range []struct {
		name    string
		userSQL string
		leftAs  string
	}{
		{name: "EXIT reverts to the base login", userSQL: "EXIT SERVICE ACCOUNT " + account, leftAs: fakeBaseLogin},
		{name: "ASSUME switches to another account", userSQL: `ASSUME SERVICE ACCOUNT "sa_other"`, leftAs: "sa_other"},
	} {
		t.Run(tc.name+", then the next user of the connection gets the pool's account", func(t *testing.T) {
			db, fc := newPool(t)

			// User A changes the session identity on a pinned connection, then releases it.
			conn, err := db.Conn(ctx)
			require.NoError(t, err)
			assert.Equal(t, account, currentUser(t, conn))
			_, err = conn.ExecContext(ctx, tc.userSQL)
			require.NoError(t, err)
			assert.Equal(t, tc.leftAs, currentUser(t, conn), "the change applies to the rest of user A's session")
			require.NoError(t, conn.Close())

			// User B, mapped to the same account, borrows the same physical connection.
			assert.Equal(t, account, currentUser(t, db))
			require.Len(t, fc.openedSessions(), 1, "the identity must be restored on the reused connection, not by reconnecting")
		})
	}

	t.Run("a failed re-ASSUME discards the connection and reports why", func(t *testing.T) {
		db, fc := newPool(t)
		assert.Equal(t, account, currentUser(t, db))

		fc.setAssumeErr(errors.New("User cannot assume service account"))
		var who string
		err := db.QueryRowContext(ctx, "SELECT current_user()").Scan(&who)
		require.Error(t, err, "a connection that could not re-assume must not run the query as whatever identity it holds")
		assert.Contains(t, err.Error(), "failed to assume service account")
		assert.Contains(t, err.Error(), "User cannot assume service account")
		sessions := fc.openedSessions()
		require.Len(t, sessions, 2)
		assert.True(t, sessions[0].isClosed(), "the connection that failed to re-assume must be discarded")

		// Once ASSUME succeeds again, the pool recovers with a fresh connection.
		fc.setAssumeErr(nil)
		assert.Equal(t, account, currentUser(t, db))
	})
}

func mutateDSI(extraJSON string) *backend.DataSourceInstanceSettings {
	jsonData := `{ "server": "h", "port": 8812, "username": "u", "tlsMode": "disable"`
	if extraJSON != "" {
		jsonData += ", " + extraJSON
	}
	jsonData += " }"
	// Deliberately no DecryptedSecureJSONData: routing resolution must not require credentials.
	return &backend.DataSourceInstanceSettings{
		JSONData: []byte(jsonData),
	}
}

func makeQueries() []backend.DataQuery {
	return []backend.DataQuery{
		{RefID: "A", JSON: []byte(`{"rawSql":"select 1","format":1}`)},
		{RefID: "B", JSON: []byte(`{"rawSql":"select 2","format":0}`)},
	}
}

func TestMutateQueryData(t *testing.T) {
	h := &QuestDB{}
	ctx := context.Background()

	routingEnabled := `"serviceAccountRoutingEnabled": true, "defaultServiceAccount": "sa_default",
		"serviceAccountMappings": [{ "grafanaUser": "john", "serviceAccount": "sa_analysts" }]`

	t.Run("nil datasource settings is a no-op", func(t *testing.T) {
		req := &backend.QueryDataRequest{
			PluginContext: backend.PluginContext{User: &backend.User{Login: "john"}},
			Queries:       makeQueries(),
		}
		_, out := h.MutateQueryData(ctx, req)
		assert.JSONEq(t, `{"rawSql":"select 1","format":1}`, string(out.Queries[0].JSON))
	})

	t.Run("routing disabled leaves queries unchanged", func(t *testing.T) {
		req := &backend.QueryDataRequest{
			PluginContext: backend.PluginContext{DataSourceInstanceSettings: mutateDSI(""), User: &backend.User{Login: "john"}},
			Queries:       makeQueries(),
		}
		_, out := h.MutateQueryData(ctx, req)
		assert.JSONEq(t, `{"rawSql":"select 1","format":1}`, string(out.Queries[0].JSON))
		assert.JSONEq(t, `{"rawSql":"select 2","format":0}`, string(out.Queries[1].JSON))
	})

	t.Run("mapped user stamps connectionArgs, preserving fields", func(t *testing.T) {
		req := &backend.QueryDataRequest{
			PluginContext: backend.PluginContext{DataSourceInstanceSettings: mutateDSI(routingEnabled), User: &backend.User{Login: "john"}},
			Queries:       makeQueries(),
		}
		_, out := h.MutateQueryData(ctx, req)
		for i, raw := range []string{`"select 1"`, `"select 2"`} {
			var m map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(out.Queries[i].JSON, &m))
			assert.JSONEq(t, `{"serviceAccount":"sa_analysts"}`, string(m["connectionArgs"]))
			assert.JSONEq(t, raw, string(m["rawSql"]))
		}
	})

	t.Run("nil user resolves to default service account", func(t *testing.T) {
		req := &backend.QueryDataRequest{
			PluginContext: backend.PluginContext{DataSourceInstanceSettings: mutateDSI(routingEnabled), User: nil},
			Queries:       makeQueries(),
		}
		_, out := h.MutateQueryData(ctx, req)
		var m map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(out.Queries[0].JSON, &m))
		assert.JSONEq(t, `{"serviceAccount":"sa_default"}`, string(m["connectionArgs"]))
	})

	t.Run("unmapped user with no default leaves queries unchanged", func(t *testing.T) {
		noDefault := `"serviceAccountRoutingEnabled": true,
			"serviceAccountMappings": [{ "grafanaUser": "john", "serviceAccount": "sa_analysts" }]`
		req := &backend.QueryDataRequest{
			PluginContext: backend.PluginContext{DataSourceInstanceSettings: mutateDSI(noDefault), User: &backend.User{Login: "nobody"}},
			Queries:       makeQueries(),
		}
		_, out := h.MutateQueryData(ctx, req)
		assert.JSONEq(t, `{"rawSql":"select 1","format":1}`, string(out.Queries[0].JSON))
	})

	t.Run("blank mapping falls through to the default service account", func(t *testing.T) {
		blank := `"serviceAccountRoutingEnabled": true, "defaultServiceAccount": "sa_default",
			"serviceAccountMappings": [{ "grafanaUser": "john", "serviceAccount": "" }]`
		req := &backend.QueryDataRequest{
			PluginContext: backend.PluginContext{DataSourceInstanceSettings: mutateDSI(blank), User: &backend.User{Login: "john"}},
			Queries:       makeQueries(),
		}
		_, out := h.MutateQueryData(ctx, req)
		var m map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(out.Queries[0].JSON, &m))
		assert.JSONEq(t, `{"serviceAccount":"sa_default"}`, string(m["connectionArgs"]))
	})

	groupRouting := `"serviceAccountRoutingEnabled": true, "defaultServiceAccount": "sa_default",
		"serviceAccountGroupMappings": [
			{ "group": "Analysts", "serviceAccount": "sa_analysts" },
			{ "group": "Execs", "serviceAccount": "sa_execs" }
		]`

	stamped := func(t *testing.T, out *backend.QueryDataRequest) json.RawMessage {
		t.Helper()
		var m map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(out.Queries[0].JSON, &m))
		return m["connectionArgs"]
	}

	t.Run("group from X-Id-Token stamps the group's service account", func(t *testing.T) {
		token := makeIDToken(t, map[string]interface{}{"groups": []string{"Analysts"}})
		req := &backend.QueryDataRequest{
			PluginContext: backend.PluginContext{DataSourceInstanceSettings: mutateDSI(groupRouting), User: &backend.User{Login: "nobody"}},
			Headers:       map[string]string{"X-Id-Token": token},
			Queries:       makeQueries(),
		}
		_, out := h.MutateQueryData(ctx, req)
		assert.JSONEq(t, `{"serviceAccount":"sa_analysts"}`, string(stamped(t, out)))
	})

	t.Run("username mapping overrides the token group", func(t *testing.T) {
		both := `"serviceAccountRoutingEnabled": true,
			"serviceAccountMappings": [{ "grafanaUser": "john", "serviceAccount": "sa_user" }],
			"serviceAccountGroupMappings": [{ "group": "Analysts", "serviceAccount": "sa_analysts" }]`
		token := makeIDToken(t, map[string]interface{}{"groups": []string{"Analysts"}})
		req := &backend.QueryDataRequest{
			PluginContext: backend.PluginContext{DataSourceInstanceSettings: mutateDSI(both), User: &backend.User{Login: "john"}},
			Headers:       map[string]string{"X-Id-Token": token},
			Queries:       makeQueries(),
		}
		_, out := h.MutateQueryData(ctx, req)
		assert.JSONEq(t, `{"serviceAccount":"sa_user"}`, string(stamped(t, out)))
	})

	t.Run("custom groups claim is honored", func(t *testing.T) {
		custom := `"serviceAccountRoutingEnabled": true, "groupsClaim": "roles",
			"serviceAccountGroupMappings": [{ "group": "Execs", "serviceAccount": "sa_execs" }]`
		token := makeIDToken(t, map[string]interface{}{"roles": []string{"Execs"}})
		req := &backend.QueryDataRequest{
			PluginContext: backend.PluginContext{DataSourceInstanceSettings: mutateDSI(custom), User: &backend.User{Login: "nobody"}},
			Headers:       map[string]string{"X-Id-Token": token},
			Queries:       makeQueries(),
		}
		_, out := h.MutateQueryData(ctx, req)
		assert.JSONEq(t, `{"serviceAccount":"sa_execs"}`, string(stamped(t, out)))
	})

	t.Run("no token with group mappings falls back to the default", func(t *testing.T) {
		req := &backend.QueryDataRequest{
			PluginContext: backend.PluginContext{DataSourceInstanceSettings: mutateDSI(groupRouting), User: &backend.User{Login: "nobody"}},
			Queries:       makeQueries(), // no X-Id-Token header
		}
		_, out := h.MutateQueryData(ctx, req)
		assert.JSONEq(t, `{"serviceAccount":"sa_default"}`, string(stamped(t, out)))
	})

	t.Run("client-supplied connectionArgs is overwritten for a mapped user", func(t *testing.T) {
		// A query payload that smuggles its own service account must not survive: the
		// resolved account replaces it so the client cannot pick its own memory limit.
		req := &backend.QueryDataRequest{
			PluginContext: backend.PluginContext{DataSourceInstanceSettings: mutateDSI(routingEnabled), User: &backend.User{Login: "john"}},
			Queries: []backend.DataQuery{
				{RefID: "A", JSON: []byte(`{"rawSql":"select 1","connectionArgs":{"serviceAccount":"sa_evil"}}`)},
			},
		}
		_, out := h.MutateQueryData(ctx, req)
		assert.JSONEq(t, `{"serviceAccount":"sa_analysts"}`, string(stamped(t, out)))
	})

	t.Run("client-supplied connectionArgs is stripped when the user resolves to no account", func(t *testing.T) {
		// Unmapped user, no default: the base login is used, but the client's smuggled
		// connectionArgs must be removed rather than honored (otherwise it would select
		// an arbitrary account the base login can assume).
		noDefault := `"serviceAccountRoutingEnabled": true,
			"serviceAccountMappings": [{ "grafanaUser": "john", "serviceAccount": "sa_analysts" }]`
		req := &backend.QueryDataRequest{
			PluginContext: backend.PluginContext{DataSourceInstanceSettings: mutateDSI(noDefault), User: &backend.User{Login: "nobody"}},
			Queries: []backend.DataQuery{
				{RefID: "A", JSON: []byte(`{"rawSql":"select 1","connectionArgs":{"serviceAccount":"sa_evil"}}`)},
			},
		}
		_, out := h.MutateQueryData(ctx, req)
		var m map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(out.Queries[0].JSON, &m))
		_, ok := m["connectionArgs"]
		assert.False(t, ok, "client-supplied connectionArgs must be stripped, not honored")
		assert.JSONEq(t, `"select 1"`, string(m["rawSql"]))
	})
}

// TestMutateQueryDataConnectionArgsAsDecoded checks the connection arguments sqlds actually
// receives: the mutated query is decoded with sqlds.GetQuery, the same decoder the query path
// uses, which matches the connectionArgs field case-insensitively.
func TestMutateQueryDataConnectionArgsAsDecoded(t *testing.T) {
	h := &QuestDB{}
	ctx := context.Background()
	mapped := mutateDSI(`"serviceAccountRoutingEnabled": true, "defaultServiceAccount": "sa_default",
		"serviceAccountMappings": [{ "grafanaUser": "john", "serviceAccount": "sa_analysts" }]`)
	noDefault := mutateDSI(`"serviceAccountRoutingEnabled": true,
		"serviceAccountMappings": [{ "grafanaUser": "john", "serviceAccount": "sa_analysts" }]`)

	users := []struct {
		name string
		dsi  *backend.DataSourceInstanceSettings
		user *backend.User
		want string // "" means no connection arguments (the base login pool)
	}{
		{name: "mapped user", dsi: mapped, user: &backend.User{Login: "john"}, want: `{"serviceAccount":"sa_analysts"}`},
		{name: "unmapped user falling back to the default", dsi: mapped, user: &backend.User{Login: "nobody"}, want: `{"serviceAccount":"sa_default"}`},
		{name: "unmapped user with no default", dsi: noDefault, user: &backend.User{Login: "nobody"}, want: ""},
	}
	payloads := []struct {
		name string
		json string
	}{
		{name: "no client arguments", json: `{"rawSql":"select 1"}`},
		{name: "exact key", json: `{"rawSql":"select 1","connectionArgs":{"serviceAccount":"sa_chosen_by_client"}}`},
		{name: "lower-case key", json: `{"rawSql":"select 1","connectionargs":{"serviceAccount":"sa_chosen_by_client"}}`},
		{name: "upper-case key", json: `{"rawSql":"select 1","CONNECTIONARGS":{"serviceAccount":"sa_chosen_by_client"}}`},
		// U+017F (long s) folds to 's' under the Unicode simple folding encoding/json uses.
		{name: "Unicode case-folded key", json: `{"rawSql":"select 1","connectionArgſ":{"serviceAccount":"sa_chosen_by_client"}}`},
		{name: "key behind the exact one", json: `{"rawSql":"select 1","connectionArgs":{"serviceAccount":"sa_x"},"connectionargs":{"serviceAccount":"sa_chosen_by_client"}}`},
		{name: "empty arguments", json: `{"rawSql":"select 1","connectionargs":{}}`},
		{name: "null arguments", json: `{"rawSql":"select 1","ConnectionArgs":null}`},
	}

	for _, u := range users {
		for _, p := range payloads {
			t.Run(u.name+", "+p.name, func(t *testing.T) {
				in := backend.DataQuery{RefID: "A", JSON: []byte(p.json)}
				if p.name != "no client arguments" {
					// Guard the premise: unsanitized, sqlds would honor the client's arguments.
					q, err := sqlds.GetQuery(in, nil, false)
					require.NoError(t, err)
					require.NotEmpty(t, q.ConnectionArgs)
				}

				req := &backend.QueryDataRequest{
					PluginContext: backend.PluginContext{DataSourceInstanceSettings: u.dsi, User: u.user},
					Queries:       []backend.DataQuery{in},
				}
				_, out := h.MutateQueryData(ctx, req)
				q, err := sqlds.GetQuery(out.Queries[0], nil, false)
				require.NoError(t, err)
				assert.Equal(t, "select 1", q.RawSQL)
				if u.want == "" {
					assert.Empty(t, q.ConnectionArgs)
				} else {
					assert.JSONEq(t, u.want, string(q.ConnectionArgs))
				}
			})
		}
	}
}

func TestWithConnectionArgs(t *testing.T) {
	connArgs := json.RawMessage(`{"serviceAccount":"sa_a"}`)

	t.Run("adds connectionArgs preserving all other fields", func(t *testing.T) {
		in := json.RawMessage(`{"rawSql":"select 1","format":1,"selectedFormat":2,"meta":{"timezone":"UTC"}}`)
		var m map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(withConnectionArgs(in, connArgs), &m))
		assert.JSONEq(t, `{"serviceAccount":"sa_a"}`, string(m["connectionArgs"]))
		assert.JSONEq(t, `"select 1"`, string(m["rawSql"]))
		assert.JSONEq(t, `1`, string(m["format"]))
		assert.JSONEq(t, `2`, string(m["selectedFormat"]))
		assert.JSONEq(t, `{"timezone":"UTC"}`, string(m["meta"]))
	})

	t.Run("overwrites an existing connectionArgs", func(t *testing.T) {
		in := json.RawMessage(`{"rawSql":"select 1","connectionArgs":{"serviceAccount":"old"}}`)
		var m map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(withConnectionArgs(in, connArgs), &m))
		assert.JSONEq(t, `{"serviceAccount":"sa_a"}`, string(m["connectionArgs"]))
	})

	t.Run("returns input unchanged on malformed JSON", func(t *testing.T) {
		in := json.RawMessage(`not json`)
		assert.Equal(t, string(in), string(withConnectionArgs(in, connArgs)))
	})

	t.Run("returns input unchanged on non-object JSON", func(t *testing.T) {
		// A JSON array cannot be unmarshalled into map[string]json.RawMessage.
		in := json.RawMessage(`["a","b"]`)
		assert.Equal(t, string(in), string(withConnectionArgs(in, connArgs)))
	})

	t.Run("returns input unchanged on JSON null without panicking", func(t *testing.T) {
		// `null` is the one non-object input that unmarshals without error (into a nil map);
		// the stamp path (non-nil connArgs) must not panic on the map assignment.
		in := json.RawMessage(`null`)
		assert.Equal(t, string(in), string(withConnectionArgs(in, connArgs)))
		assert.Equal(t, string(in), string(withConnectionArgs(in, nil)))
	})

	t.Run("nil connArgs removes an existing connectionArgs", func(t *testing.T) {
		in := json.RawMessage(`{"rawSql":"select 1","connectionArgs":{"serviceAccount":"sa_evil"}}`)
		var m map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(withConnectionArgs(in, nil), &m))
		_, ok := m["connectionArgs"]
		assert.False(t, ok)
		assert.JSONEq(t, `"select 1"`, string(m["rawSql"]))
	})

	t.Run("nil connArgs leaves bytes untouched when no connectionArgs present", func(t *testing.T) {
		in := json.RawMessage(`{"rawSql":"select 1","format":1}`)
		assert.Equal(t, string(in), string(withConnectionArgs(in, nil)))
	})

	t.Run("removes every case variant of connectionArgs", func(t *testing.T) {
		in := json.RawMessage(`{"rawSql":"select 1","connectionArgs":{"serviceAccount":"a"},"connectionargs":{"serviceAccount":"b"},"CONNECTIONARGS":{}}`)

		var stamped map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(withConnectionArgs(in, connArgs), &stamped))
		assert.Equal(t, []string{"connectionArgs", "rawSql"}, slices.Sorted(maps.Keys(stamped)))
		assert.JSONEq(t, `{"serviceAccount":"sa_a"}`, string(stamped["connectionArgs"]))

		var stripped map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(withConnectionArgs(in, nil), &stripped))
		assert.Equal(t, []string{"rawSql"}, slices.Sorted(maps.Keys(stripped)))
	})
}

func TestRoutedServiceAccount(t *testing.T) {
	routing := Settings{serviceAccountConfig: serviceAccountConfig{ServiceAccountRoutingEnabled: true}}

	t.Run("no message selects the base login pool", func(t *testing.T) {
		for _, msg := range []json.RawMessage{nil, {}} {
			sa, err := routedServiceAccount(routing, msg)
			require.NoError(t, err)
			assert.Equal(t, "", sa)
		}
	})

	t.Run("routing disabled ignores the message", func(t *testing.T) {
		sa, err := routedServiceAccount(Settings{}, json.RawMessage(`{"serviceAccount":"sa_a"}`))
		require.NoError(t, err)
		assert.Equal(t, "", sa)
	})

	t.Run("returns the stamped service account", func(t *testing.T) {
		sa, err := routedServiceAccount(routing, json.RawMessage(`{"serviceAccount":"sa_a"}`))
		require.NoError(t, err)
		assert.Equal(t, "sa_a", sa)
	})

	t.Run("refuses arguments that name no service account", func(t *testing.T) {
		for _, msg := range []string{`{}`, `{"serviceAccount":""}`, `null`, `{"serviceAccount":1}`, `not json`} {
			_, err := routedServiceAccount(routing, json.RawMessage(msg))
			assert.Error(t, err, msg)
		}
	})
}

func TestConnectServiceAccountWrapping(t *testing.T) {
	// These exercise the routing branch of Connect without a live database: an invalid
	// name errors before sql.OpenDB, and OpenDB itself is lazy (no connection until used).
	baseJSON := `{"server":"h","port":8812,"username":"u","tlsMode":"disable","serviceAccountRoutingEnabled":true}`
	cfg := backend.DataSourceInstanceSettings{
		JSONData:                []byte(baseJSON),
		DecryptedSecureJSONData: map[string]string{"password": "p"},
	}

	t.Run("invalid service account name errors at connect", func(t *testing.T) {
		// ')' is in QuestDB's forbidden set; a space or '@' would be accepted (see
		// TestBuildAssumeStatement), so use a genuinely-forbidden character here.
		msg, err := json.Marshal(connectionArgs{ServiceAccount: "bad)name"})
		require.NoError(t, err)
		db, err := (&QuestDB{}).Connect(context.Background(), cfg, msg)
		require.Error(t, err)
		assert.Nil(t, db)
		assert.Contains(t, err.Error(), "invalid character")
		assert.Contains(t, err.Error(), "bad)name")
	})

	t.Run("valid service account name wires the pool without error", func(t *testing.T) {
		msg, err := json.Marshal(connectionArgs{ServiceAccount: "sa_analysts"})
		require.NoError(t, err)
		db, err := (&QuestDB{}).Connect(context.Background(), cfg, msg)
		require.NoError(t, err)
		require.NotNil(t, db)
		_ = db.Close()
	})

	t.Run("arguments without a service account are refused", func(t *testing.T) {
		db, err := (&QuestDB{}).Connect(context.Background(), cfg, json.RawMessage(`{}`))
		require.Error(t, err)
		assert.Nil(t, db)
	})
}

// TestRoutedPoolSharing covers pool ownership for routed pools: Connect must hand every caller
// asking for one account the same pool, since sqlds may call it several times for one cache key.
func TestRoutedPoolSharing(t *testing.T) {
	ctx := context.Background()
	cfg := backend.DataSourceInstanceSettings{
		JSONData: []byte(`{"server":"127.0.0.1","port":1,"username":"u","tlsMode":"disable","timeout":"1",` +
			`"serviceAccountRoutingEnabled":true,"defaultServiceAccount":"sa_a"}`),
		DecryptedSecureJSONData: map[string]string{"password": "p"},
	}
	msg := func(t *testing.T, sa string) json.RawMessage {
		t.Helper()
		m, err := json.Marshal(connectionArgs{ServiceAccount: sa})
		require.NoError(t, err)
		return m
	}
	isClosed := func(db *sql.DB) bool {
		// A closed pool fails fast without dialing; an open one fails to reach port 1.
		err := db.PingContext(ctx)
		return err != nil && err.Error() == "sql: database is closed"
	}

	t.Run("concurrent first use of an account shares one pool", func(t *testing.T) {
		h := &QuestDB{}
		const callers = 8
		dbs := make([]*sql.DB, callers)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range dbs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				db, err := h.Connect(ctx, cfg, msg(t, "sa_a"))
				assert.NoError(t, err)
				dbs[i] = db
			}()
		}
		close(start)
		wg.Wait()
		for _, db := range dbs {
			assert.Same(t, dbs[0], db)
		}

		other, err := h.Connect(ctx, cfg, msg(t, "sa_b"))
		require.NoError(t, err)
		assert.NotSame(t, dbs[0], other, "each account gets its own pool")
		_ = dbs[0].Close()
		_ = other.Close()
	})

	t.Run("a closed pool is replaced rather than handed out again", func(t *testing.T) {
		h := &QuestDB{}
		first, err := h.Connect(ctx, cfg, msg(t, "sa_a"))
		require.NoError(t, err)
		require.NoError(t, first.Close())

		second, err := h.Connect(ctx, cfg, msg(t, "sa_a"))
		require.NoError(t, err)
		defer second.Close()
		assert.NotSame(t, first, second)
		assert.False(t, isClosed(second))
	})

	t.Run("the health probe does not close the pool queries share", func(t *testing.T) {
		h := &QuestDB{}
		shared, err := h.Connect(ctx, cfg, msg(t, "sa_a"))
		require.NoError(t, err)
		defer shared.Close()

		res := h.PostCheckHealth(ctx, &backend.CheckHealthRequest{PluginContext: backend.PluginContext{DataSourceInstanceSettings: &cfg}})
		require.NotNil(t, res, "the unreachable server fails the probe")
		assert.False(t, isClosed(shared))
		again, err := h.Connect(ctx, cfg, msg(t, "sa_a"))
		require.NoError(t, err)
		assert.Same(t, shared, again)
	})

	t.Run("through sqlds, concurrent first use opens one pool and disposal closes it", func(t *testing.T) {
		// barrierDriver holds every routed Connect until all callers have arrived, so each of
		// them has provably missed the sqlds cache before any pool is stored in it.
		const callers = 8
		d := &barrierDriver{QuestDB: &QuestDB{}, start: make(chan struct{})}
		d.arrived.Add(callers)
		ds := sqlds.NewDatasource(d)
		ds.EnableMultipleConnections = true
		_, err := ds.NewDatasource(ctx, cfg)
		require.NoError(t, err)

		dbs := make([]*sql.DB, callers)
		var wg sync.WaitGroup
		for i := range dbs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				db, err := ds.GetDBFromQuery(ctx, &sqlds.Query{ConnectionArgs: msg(t, "sa_a")})
				assert.NoError(t, err)
				dbs[i] = db
			}()
		}
		d.arrived.Wait()
		close(d.start)
		wg.Wait()

		for _, db := range dbs {
			require.NotNil(t, db)
			assert.Same(t, dbs[0], db, "every caller must get the one pool sqlds keeps for the account")
		}
		assert.False(t, isClosed(dbs[0]))
		ds.Dispose()
		for _, db := range dbs {
			assert.True(t, isClosed(db), "disposal must close every pool that was handed out")
		}
	})
}

type barrierDriver struct {
	*QuestDB
	arrived sync.WaitGroup
	start   chan struct{}
}

func (d *barrierDriver) Connect(ctx context.Context, config backend.DataSourceInstanceSettings, message json.RawMessage) (*sql.DB, error) {
	if len(message) > 0 {
		d.arrived.Done()
		<-d.start
	}
	return d.QuestDB.Connect(ctx, config, message)
}

// TestValidateServiceAccountNames verifies review #2's fix: configured service-account names
// are checked at config time (so Save & Test catches a typo) rather than only at query time.
func TestValidateServiceAccountNames(t *testing.T) {
	t.Run("all valid names pass", func(t *testing.T) {
		// Includes names QuestDB allows but the old allowlist rejected (space, '@', email
		// form), locking in the "match QuestDB's denylist" decision.
		s := Settings{serviceAccountConfig: serviceAccountConfig{
			DefaultServiceAccount:       "sa_default",
			ServiceAccountMappings:      []ServiceAccountMapping{{GrafanaUser: "john", ServiceAccount: "john.doe@mail.com"}},
			ServiceAccountGroupMappings: []ServiceAccountGroupMapping{{Group: "Execs", ServiceAccount: "data team"}},
		}}
		assert.NoError(t, s.validateServiceAccountNames())
	})

	t.Run("blank names are skipped, not rejected", func(t *testing.T) {
		// A blank default/mapping/group target means "fall through", which is valid config.
		s := Settings{serviceAccountConfig: serviceAccountConfig{
			DefaultServiceAccount:       "   ",
			ServiceAccountMappings:      []ServiceAccountMapping{{GrafanaUser: "john", ServiceAccount: ""}},
			ServiceAccountGroupMappings: []ServiceAccountGroupMapping{{Group: "Execs", ServiceAccount: "  "}},
		}}
		assert.NoError(t, s.validateServiceAccountNames())
	})

	t.Run("invalid default account is rejected and named", func(t *testing.T) {
		// ')' is in QuestDB's forbidden set (a space would now be accepted); the full
		// forbidden charset is covered by TestBuildAssumeStatement.
		s := Settings{serviceAccountConfig: serviceAccountConfig{DefaultServiceAccount: "sa)oops"}}
		err := s.validateServiceAccountNames()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sa)oops")
	})

	t.Run("invalid user-mapping account is rejected and named", func(t *testing.T) {
		s := Settings{serviceAccountConfig: serviceAccountConfig{ServiceAccountMappings: []ServiceAccountMapping{{GrafanaUser: "john", ServiceAccount: "bad/name"}}}}
		err := s.validateServiceAccountNames()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bad/name")
	})

	t.Run("invalid group-mapping account is rejected and named", func(t *testing.T) {
		s := Settings{serviceAccountConfig: serviceAccountConfig{ServiceAccountGroupMappings: []ServiceAccountGroupMapping{{Group: "Execs", ServiceAccount: "sa(evil"}}}}
		err := s.validateServiceAccountNames()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sa(evil")
	})
}

// TestPostCheckHealth verifies review #1's fix: the health check exercises the routing path.
// The branches here need no live DB — routing-disabled and name-validation short-circuit
// before any connection, and the unreachable-server case proves the probe actually dials.
func TestPostCheckHealth(t *testing.T) {
	h := &QuestDB{}
	ctx := context.Background()

	healthReq := func(dsi *backend.DataSourceInstanceSettings) *backend.CheckHealthRequest {
		return &backend.CheckHealthRequest{PluginContext: backend.PluginContext{DataSourceInstanceSettings: dsi}}
	}

	t.Run("nil datasource settings is healthy", func(t *testing.T) {
		assert.Nil(t, h.PostCheckHealth(ctx, healthReq(nil)))
	})

	t.Run("routing disabled is healthy", func(t *testing.T) {
		assert.Nil(t, h.PostCheckHealth(ctx, healthReq(mutateDSI(""))))
	})

	t.Run("malformed routing block fails before any DB probe", func(t *testing.T) {
		// A provisioned type mismatch (numeric serviceAccount) unmarshals partially and would
		// otherwise drop the row, routing the user to the uncapped base login unnoticed. The
		// DSI carries no credentials, so reaching the probe would fail for an unrelated reason;
		// a parse-specific message proves we short-circuit at config time.
		dsi := mutateDSI(`"serviceAccountRoutingEnabled": true,
			"serviceAccountMappings": [{ "grafanaUser": "john", "serviceAccount": 123 }]`)
		res := h.PostCheckHealth(ctx, healthReq(dsi))
		require.NotNil(t, res)
		assert.Equal(t, backend.HealthStatusError, res.Status)
		assert.Contains(t, res.Message, "could not parse routing configuration")
	})

	t.Run("mistyped enable flag is caught even though it parses as disabled", func(t *testing.T) {
		// The enable flag itself is the type-mismatched field, so it falls back to false; an
		// enabled-first check would skip silently. The parse-error check runs first to catch it.
		dsi := mutateDSI(`"serviceAccountRoutingEnabled": "yes"`)
		res := h.PostCheckHealth(ctx, healthReq(dsi))
		require.NotNil(t, res)
		assert.Equal(t, backend.HealthStatusError, res.Status)
		assert.Contains(t, res.Message, "could not parse routing configuration")
	})

	t.Run("invalid configured name fails before any DB probe", func(t *testing.T) {
		// mutateDSI carries no password/secure data, so if validation did NOT short-circuit,
		// the probe's Connect would fail for an unrelated (missing-credentials) reason. A
		// message that names the bad account proves the failure is the config-time name check.
		// ')' is in QuestDB's forbidden set (a space would be accepted and reach the probe).
		dsi := mutateDSI(`"serviceAccountRoutingEnabled": true, "defaultServiceAccount": "bad)name"`)
		res := h.PostCheckHealth(ctx, healthReq(dsi))
		require.NotNil(t, res)
		assert.Equal(t, backend.HealthStatusError, res.Status)
		assert.Contains(t, res.Message, "bad)name")
	})

	t.Run("valid names with no default account is healthy (nothing to probe)", func(t *testing.T) {
		dsi := mutateDSI(`"serviceAccountRoutingEnabled": true,
			"serviceAccountMappings": [{ "grafanaUser": "john", "serviceAccount": "sa_analysts" }]`)
		assert.Nil(t, h.PostCheckHealth(ctx, healthReq(dsi)))
	})

	t.Run("valid default account probes the server and surfaces a connection failure", func(t *testing.T) {
		// Routing on with a valid default name, so PostCheckHealth opens a routed pool and
		// pings it. The server is unreachable (loopback port 1 → connection refused), so the
		// probe — and thus Save & Test — fails. This proves the ASSUME path is actually
		// dialed at health-check time, which is the gap review #1 reported.
		dsi := &backend.DataSourceInstanceSettings{
			JSONData: []byte(`{"server":"127.0.0.1","port":1,"username":"u","tlsMode":"disable",` +
				`"timeout":"1","serviceAccountRoutingEnabled":true,"defaultServiceAccount":"sa_default"}`),
			DecryptedSecureJSONData: map[string]string{"password": "p"},
		}
		res := h.PostCheckHealth(ctx, healthReq(dsi))
		require.NotNil(t, res)
		assert.Equal(t, backend.HealthStatusError, res.Status)
		assert.Contains(t, res.Message, "sa_default")
	})
}
