package core

import (
	"context"
	"github.com/eco-digit/energyprofiler/internal/types"
)

type Benchmark interface {
	Name() string
	Run(ctx context.Context) (*types.ResourceProfile, error)
	Validate() error
}

type PowerReader interface {
	ReadPower(resourceType string) (float64, error)
	ReadSystemPower() (float64, error)
}

type LoadGenerator interface {
	Start(loadLevel int) error
	Stop() error
}

type UtilizationGetter func() (float64, error)
