package plugin

import (
	"context"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/sqlds/v4"
)

// NewDatasource is the instance factory for the QuestDB data source.
func NewDatasource(ctx context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	driver := &QuestDB{}
	return newDatasource(ctx, settings, driver, driver)
}

// newDatasource creates an instance whose sqlds driver is driver, which is qdb or wraps it; qdb
// owns the instance's routed pools. Tests pass a wrapper to pause Connect.
func newDatasource(ctx context.Context, settings backend.DataSourceInstanceSettings, driver sqlds.Driver, qdb *QuestDB) (instancemgmt.Instance, error) {
	ds := sqlds.NewDatasource(driver)
	// Per-service-account connection pools are only needed when routing is enabled, so we
	// gate this on the routing flag. Leaving it off for routing-disabled data sources keeps
	// the prior single-pool behavior, where a query carrying connectionArgs is rejected
	// rather than spawning cached pools keyed by client-supplied args. When routing is on,
	// MutateQueryData owns connectionArgs (it strips any client value), so the number of
	// distinct pools is bounded by the configured service accounts. ForwardHeaders is
	// intentionally left off so HTTP headers are not folded into the pool cache key.
	ds.EnableMultipleConnections = LoadServiceAccountSettings(settings).ServiceAccountRoutingEnabled
	// Base Save & Test only checks base-login connectivity. PostCheckHealth additionally
	// exercises the routing path (validates configured account names and runs a real ASSUME
	// for the default account) so routing misconfiguration fails here instead of on every
	// routed query. It is a no-op when routing is disabled.
	ds.PostCheckHealth = qdb.PostCheckHealth
	if _, err := ds.NewDatasource(ctx, settings); err != nil {
		return nil, err
	}
	return &datasource{SQLDatasource: ds, qdb: qdb}, nil
}

// datasource is a sqlds data source whose disposal also closes the routed pools of its driver.
type datasource struct {
	*sqlds.SQLDatasource
	qdb *QuestDB
}

// The instance manager finds these by type assertion, so the wrapper must keep serving them.
var (
	_ backend.QueryDataHandler      = (*datasource)(nil)
	_ backend.CheckHealthHandler    = (*datasource)(nil)
	_ backend.CallResourceHandler   = (*datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*datasource)(nil)
)

// Dispose closes every pool of the instance: the driver's routed pools, including one sqlds has
// not stored yet (see QuestDB.Dispose), and the pools in the sqlds cache.
func (ds *datasource) Dispose() {
	ds.qdb.Dispose()
	ds.SQLDatasource.Dispose()
}
