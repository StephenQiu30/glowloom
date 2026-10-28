package media

import (
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// AssetObjects owns the configured readers used by original downloads and public works.
// Legacy is selected only for stored assets without an explicit physical location.
type AssetObjects struct {
	Legacy   *objectstorage.Client
	Routed   *mediaobjects.RoutedReadObjects
	registry *objectstorage.Registry
}

// OpenAssetObjects validates configured profiles without contacting a storage provider.
func OpenAssetObjects(cfg config.Config, legacy *objectstorage.Client) (AssetObjects, error) {
	objects := AssetObjects{Legacy: legacy}
	if cfg.ObjectStorage != nil {
		registry, err := objectstorage.OpenProfiles(*cfg.ObjectStorage)
		if err != nil {
			return AssetObjects{}, err
		}
		objects.registry, objects.Routed = registry, mediaobjects.NewRoutedReadObjects(registry)
	}
	return objects, nil
}

// Close releases only the configured profile transport owned by these readers.
func (o AssetObjects) Close() {
	if o.registry != nil {
		o.registry.Close()
	}
}
