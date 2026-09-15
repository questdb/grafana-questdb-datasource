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
	// sqlds Connector.GetConnectionFromQuery creates pools with a non-atomic
	// load→Connect→store, so concurrent first use of an account calls Connect several times.
	// driver.Connect returns one shared pool per account to all of them, so no pool is
	// orphaned outside the sqlds cache that Dispose closes.
	ds.EnableMultipleConnections = plugin.LoadServiceAccountSettings(settings).ServiceAccountRoutingEnabled
	// Base Save & Test only checks base-login connectivity. PostCheckHealth additionally
	// exercises the routing path (validates configured account names and runs a real ASSUME
	// for the default account) so routing misconfiguration fails here instead of on every
	// routed query. It is a no-op when routing is disabled.
	ds.PostCheckHealth = driver.PostCheckHealth
	return ds.NewDatasource(ctx, settings)
}
