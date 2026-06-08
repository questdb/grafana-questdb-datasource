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
	ds := sqlds.NewDatasource(&plugin.QuestDB{})
	// Per-service-account connection pools are only needed when routing is enabled, so we
	// gate this on the routing flag. Leaving it off for routing-disabled data sources keeps
	// the prior single-pool behavior, where a query carrying connectionArgs is rejected
	// rather than spawning cached pools keyed by client-supplied args. When routing is on,
	// MutateQueryData owns connectionArgs (it strips any client value), so the number of
	// distinct pools is bounded by the configured service accounts. ForwardHeaders is
	// intentionally left off so HTTP headers are not folded into the pool cache key.
	ds.EnableMultipleConnections = plugin.LoadServiceAccountSettings(settings).ServiceAccountRoutingEnabled
	return ds.NewDatasource(ctx, settings)
}
