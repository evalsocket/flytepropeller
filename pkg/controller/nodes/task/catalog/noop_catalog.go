package catalog

import (
	"context"

	"github.com/lyft/flyteidl/gen/pb-go/flyteidl/core"
	"github.com/lyft/flyteplugins/go/tasks/pluginmachinery/catalog"
	"github.com/lyft/flyteplugins/go/tasks/pluginmachinery/io"
)

var disabledStatus = catalog.NewStatus(core.CatalogCacheStatus_CACHE_DISABLED, nil)

type NOOPCatalog struct {
}

func (n NOOPCatalog) Get(_ context.Context, _ catalog.Key) (catalog.Entry, error) {
	return catalog.NewCatalogEntry(nil, disabledStatus), nil
}

func (n NOOPCatalog) Put(_ context.Context, _ catalog.Key, _ io.OutputReader, _ catalog.Metadata) (catalog.Status, error) {
	return disabledStatus, nil
}
