package main

import (
	"context"
	"os"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/datasource"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/sqlds/v4"
	"github.com/questdb/grafana-questdb-datasource/pkg/plugin"
)

func main() {
	if err := datasource.Manage("questdb-questdb-datasource", newDatasource, datasource.ManageOpts{}); err != nil {
		log.DefaultLogger.Error(err.Error())
		os.Exit(1)
	}
}

func newDatasource(ctx context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	driver := &plugin.QuestDB{}
	ds := sqlds.NewDatasource(driver)
	// Per-service-account connection pools are only needed when routing is enabled, so we
	// gate this on the routing flag. Leaving it off for routing-disabled data sources keeps
	// the prior single-pool behavior, where a query carrying connectionArgs is rejected
	// rather than spawning cached pools keyed by client-supplied args. When routing is on,
	// MutateQueryData owns connectionArgs (it strips any client value), so the number of
	// distinct pools is bounded by the configured service accounts. ForwardHeaders is
	// intentionally left off so HTTP headers are not folded into the pool cache key.
	//
	// Caveat (upstream, not plugin-fixable): enabling multiple connections activates a race
	// in sqlds Connector.GetConnectionFromQuery — a non-atomic load→Connect→store on a
	// sync.Map (no LoadOrStore). Concurrent first-use of the same service account (e.g. a
	// dashboard whose panels fire together for a freshly-mapped user) can open several pools
	// and orphan all but the last; Dispose only closes pools still in the map, so the losers
	// leak their connections. The window is one-time per account per instance (subsequent
	// uses hit the cache). To shrink it, favor a small set of group-mapped accounts over many
	// per-user accounts, and set a connection max-lifetime so any orphaned idle connection is
	// eventually reaped rather than held for the life of the process.
	ds.EnableMultipleConnections = plugin.LoadServiceAccountSettings(settings).ServiceAccountRoutingEnabled
	// Base Save & Test only checks base-login connectivity. PostCheckHealth additionally
	// exercises the routing path (validates configured account names and runs a real ASSUME
	// for the default account) so routing misconfiguration fails here instead of on every
	// routed query. It is a no-op when routing is disabled.
	ds.PostCheckHealth = driver.PostCheckHealth
	return ds.NewDatasource(ctx, settings)
}
