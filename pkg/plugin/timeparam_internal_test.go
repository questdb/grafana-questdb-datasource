package plugin

// Internal tests for the advisory bind-capability probe — the error discrimination it
// relies on, and that it tolerates an unreachable server (Connect must succeed and
// stay silent when the probe cannot get a verdict).

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsBindRejection pins the discrimination the advisory probe relies on. The
// old-QuestDB rejection arrives in two shapes — lib/pq's client-side arity error (a
// plain error: the server's Describe reported 0 bind params) or a server-sent
// *pq.Error — and both are verdicts; transport-level failures are not.
func TestIsBindRejection(t *testing.T) {
	// The exact client-side error lib/pq's errorf produces against QuestDB <= 8.2.x.
	assert.True(t, isBindRejection(fmt.Errorf("pq: got 1 parameters but the statement requires 0")))
	assert.True(t, isBindRejection(&pq.Error{Message: "bind variable service unavailable"}))
	assert.True(t, isBindRejection(fmt.Errorf("query: %w", &pq.Error{Message: "wrapped"})))
	assert.False(t, isBindRejection(fmt.Errorf("plain error")))
	assert.False(t, isBindRejection(&net.OpError{Op: "dial", Err: fmt.Errorf("connection refused")}))
}

// TestWarnIfBindParamsUnsupportedToleratesUnreachableServer: an unreachable server
// (nothing listens on port 1) yields no verdict — the probe must return promptly and
// without panicking, since it runs inside Connect.
func TestWarnIfBindParamsUnsupportedToleratesUnreachableServer(t *testing.T) {
	db, err := sql.Open("postgres",
		"host=127.0.0.1 port=1 user=admin password=quest dbname=qdb sslmode=disable connect_timeout=2")
	require.NoError(t, err)
	defer db.Close()
	warnIfBindParamsUnsupported(context.Background(), db)
}
