package driver

import (
	"errors"

	"github.com/welllog/golt/config/meta"
)

var driverFactories = make(map[string]func(config meta.Config) (Driver, error))

func RegisterDriver(schema string, factory func(config meta.Config) (Driver, error)) {
	driverFactories[schema] = factory
}

func New(config meta.Config) (Driver, error) {
	schema := config.SourceSchema()
	factory, ok := driverFactories[schema]
	if !ok {
		return nil, errors.New("unknown driver schema: " + schema)
	}

	return factory(config)
}
