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
	// Enables per-service-account connection pools for service-account routing. Safe to
	// set unconditionally: when routing is off (or a query carries no connectionArgs),
	// sqlds returns the default pool, so behavior is identical to before. ForwardHeaders
	// is intentionally left off so HTTP headers are not folded into the pool cache key.
	ds.EnableMultipleConnections = true
	return ds.NewDatasource(ctx, settings)
}
