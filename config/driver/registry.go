package driver

import (
	"fmt"

	"github.com/welllog/golt/config/meta"
	"github.com/welllog/golt/contract"
)

// Factory creates a Driver for the given meta config.
type Factory func(meta.Config, contract.Logger) (Driver, error)

var driverFactories = make(map[string]Factory)

// RegisterDriver registers a factory for the schema. Call it from package
// init only: the registry is read without locking at runtime.
func RegisterDriver(schema string, factory Factory) {
	driverFactories[schema] = factory
}

// New creates a Driver for the schema of c. overrides, when not nil, takes
// precedence over the global registry and carries per-Configure factories,
// keeping the global registry read-only at runtime.
func New(c meta.Config, logger contract.Logger, overrides map[string]Factory) (Driver, error) {
	schema := c.SourceSchema()
	factory, ok := overrides[schema]
	if !ok {
		factory, ok = driverFactories[schema]
	}
	if !ok {
		return nil, fmt.Errorf("unknown or missing driver schema in source: %q", c.Source)
	}

	return factory(c, logger)
}
