package plugin

import (
	"context"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
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
}

func TestResolveServiceAccount(t *testing.T) {
	settings := Settings{
		DefaultServiceAccount: "sa_default",
		ServiceAccountMappings: []ServiceAccountMapping{
			{GrafanaUser: "john", ServiceAccount: "sa_analysts"},
			{GrafanaUser: "ceo", ServiceAccount: "sa_execs"},
		},
	}

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
		s := Settings{ServiceAccountMappings: []ServiceAccountMapping{{GrafanaUser: "john", ServiceAccount: "sa_analysts"}}}
		assert.Equal(t, "", s.resolveServiceAccount(&backend.User{Login: "nobody"}, nil))
		assert.Equal(t, "", s.resolveServiceAccount(nil, nil))
	})

	t.Run("blank mapping service account falls through to the default", func(t *testing.T) {
		s := Settings{
			DefaultServiceAccount:  "sa_default",
			ServiceAccountMappings: []ServiceAccountMapping{{GrafanaUser: "john", ServiceAccount: "   "}},
		}
		// Without a default, a blank mapping must not assume the base login implicitly.
		assert.Equal(t, "sa_default", s.resolveServiceAccount(&backend.User{Login: "john"}, nil))
		s.DefaultServiceAccount = ""
		assert.Equal(t, "", s.resolveServiceAccount(&backend.User{Login: "john"}, nil))
	})

	t.Run("a later non-blank row for the same user still matches", func(t *testing.T) {
		s := Settings{ServiceAccountMappings: []ServiceAccountMapping{
			{GrafanaUser: "john", ServiceAccount: ""},
			{GrafanaUser: "john", ServiceAccount: "sa_analysts"},
		}}
		assert.Equal(t, "sa_analysts", s.resolveServiceAccount(&backend.User{Login: "john"}, nil))
	})

	t.Run("whitespace is trimmed from resolved names", func(t *testing.T) {
		s := Settings{
			DefaultServiceAccount:  "  sa_default  ",
			ServiceAccountMappings: []ServiceAccountMapping{{GrafanaUser: "john", ServiceAccount: "  sa_analysts  "}},
		}
		assert.Equal(t, "sa_analysts", s.resolveServiceAccount(&backend.User{Login: "john"}, nil))
		assert.Equal(t, "sa_default", s.resolveServiceAccount(&backend.User{Login: "nobody"}, nil))
	})

	t.Run("whitespace-only default resolves to empty", func(t *testing.T) {
		s := Settings{DefaultServiceAccount: "   "}
		assert.Equal(t, "", s.resolveServiceAccount(nil, nil))
		assert.Equal(t, "", s.resolveServiceAccount(&backend.User{Login: "nobody"}, nil))
	})
}

func TestResolveServiceAccountGroups(t *testing.T) {
	settings := Settings{
		DefaultServiceAccount:  "sa_default",
		ServiceAccountMappings: []ServiceAccountMapping{{GrafanaUser: "john", ServiceAccount: "sa_user"}},
		ServiceAccountGroupMappings: []ServiceAccountGroupMapping{
			{Group: "Analysts", ServiceAccount: "sa_analysts"},
			{Group: "Execs", ServiceAccount: "sa_execs"},
		},
	}

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
		s := Settings{
			DefaultServiceAccount:       "sa_default",
			ServiceAccountGroupMappings: []ServiceAccountGroupMapping{{Group: "Analysts", ServiceAccount: "  "}},
		}
		assert.Equal(t, "sa_default", s.resolveServiceAccount(&backend.User{Login: "nobody"}, []string{"Analysts"}))
	})

	t.Run("blank group name matches nothing", func(t *testing.T) {
		s := Settings{ServiceAccountGroupMappings: []ServiceAccountGroupMapping{{Group: "  ", ServiceAccount: "sa_blank"}}}
		assert.Equal(t, "", s.resolveServiceAccount(&backend.User{Login: "nobody"}, []string{"", "Analysts"}))
	})

	t.Run("group names and resolved SA are trimmed on both sides", func(t *testing.T) {
		s := Settings{ServiceAccountGroupMappings: []ServiceAccountGroupMapping{{Group: "  Analysts  ", ServiceAccount: "  sa_analysts  "}}}
		assert.Equal(t, "sa_analysts", s.resolveServiceAccount(&backend.User{Login: "nobody"}, []string{"  Analysts  "}))
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

	t.Run("claim that is not a string array returns nil", func(t *testing.T) {
		assert.Nil(t, extractGroups(makeIDToken(t, map[string]interface{}{"groups": "single"}), "groups"))
		assert.Nil(t, extractGroups(makeIDToken(t, map[string]interface{}{"groups": []int{1, 2}}), "groups"))
		assert.Nil(t, extractGroups(makeIDToken(t, map[string]interface{}{"groups": map[string]interface{}{"a": 1}}), "groups"))
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
		bad := []string{
			`sa"; DROP TABLE x;--`,
			`sa name`,
			`sa;`,
			`sa"a`,
			`sa'a`,
			"sa\na",
			`"`,
			`sa)`,
		}
		for _, name := range bad {
			_, err := buildAssumeStatement(name)
			assert.Error(t, err, name)
		}
	})
}

// --- fakes for the connector wrapper ---

type fakeConnector struct {
	conn       driver.Conn
	connectErr error
}

func (c *fakeConnector) Connect(_ context.Context) (driver.Conn, error) {
	if c.connectErr != nil {
		return nil, c.connectErr
	}
	return c.conn, nil
}
func (c *fakeConnector) Driver() driver.Driver { return nil }

// execerConn implements driver.Conn and driver.ExecerContext, recording the statement.
type execerConn struct {
	lastQuery string
	closed    bool
	execErr   error
}

func (c *execerConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not implemented") }
func (c *execerConn) Close() error                        { c.closed = true; return nil }
func (c *execerConn) Begin() (driver.Tx, error)           { return nil, errors.New("not implemented") }
func (c *execerConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.lastQuery = query
	if c.execErr != nil {
		return nil, c.execErr
	}
	return driver.RowsAffected(0), nil
}

// plainConn implements only driver.Conn (no ExecerContext).
type plainConn struct{ closed bool }

func (c *plainConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not implemented") }
func (c *plainConn) Close() error                        { c.closed = true; return nil }
func (c *plainConn) Begin() (driver.Tx, error)           { return nil, errors.New("not implemented") }

func TestAssumeServiceAccountConnector(t *testing.T) {
	const stmt = `ASSUME SERVICE ACCOUNT "sa_a"`

	t.Run("runs ASSUME once and returns the conn", func(t *testing.T) {
		fc := &execerConn{}
		asc := &assumeServiceAccountConnector{base: &fakeConnector{conn: fc}, stmt: stmt}
		got, err := asc.Connect(context.Background())
		require.NoError(t, err)
		assert.Same(t, fc, got)
		assert.Equal(t, stmt, fc.lastQuery)
		assert.False(t, fc.closed)
	})

	t.Run("closes conn and propagates exec error", func(t *testing.T) {
		fc := &execerConn{execErr: errors.New("boom")}
		asc := &assumeServiceAccountConnector{base: &fakeConnector{conn: fc}, stmt: stmt}
		_, err := asc.Connect(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to assume service account")
		assert.True(t, fc.closed)
	})

	t.Run("errors when conn lacks ExecContext", func(t *testing.T) {
		pc := &plainConn{}
		asc := &assumeServiceAccountConnector{base: &fakeConnector{conn: pc}, stmt: stmt}
		_, err := asc.Connect(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not support ExecContext")
		assert.True(t, pc.closed)
	})

	t.Run("propagates base connect error", func(t *testing.T) {
		asc := &assumeServiceAccountConnector{base: &fakeConnector{connectErr: errors.New("dial fail")}, stmt: stmt}
		_, err := asc.Connect(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dial fail")
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
		msg, err := json.Marshal(connectionArgs{ServiceAccount: "bad name!"})
		require.NoError(t, err)
		db, err := (&QuestDB{}).Connect(context.Background(), cfg, msg)
		require.Error(t, err)
		assert.Nil(t, db)
		assert.Contains(t, err.Error(), "invalid service account name")
	})

	t.Run("valid service account name wires the pool without error", func(t *testing.T) {
		msg, err := json.Marshal(connectionArgs{ServiceAccount: "sa_analysts"})
		require.NoError(t, err)
		db, err := (&QuestDB{}).Connect(context.Background(), cfg, msg)
		require.NoError(t, err)
		require.NotNil(t, db)
		_ = db.Close()
	})
}
