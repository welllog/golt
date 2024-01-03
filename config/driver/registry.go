package driver

import (
	"errors"

	"github.com/welllog/golt/config/meta"
	"github.com/welllog/golt/contract"
)

var driverFactories = make(map[string]func(meta.Config, contract.Logger) (Driver, error))

func RegisterDriver(schema string, factory func(meta.Config, contract.Logger) (Driver, error)) {
	driverFactories[schema] = factory
}

func New(config meta.Config, logger contract.Logger) (Driver, error) {
	schema := config.SourceSchema()
	factory, ok := driverFactories[schema]
	if !ok {
		return nil, errors.New("unknown driver schema: " + schema)
	}

	return factory(config, logger)
}
