package dns

import "context"

type Manager interface {
	Apply(ctx context.Context, servers []string, cleanup Cleanup) error
}

type Cleanup interface {
	Add(name string, fn func() error)
}
