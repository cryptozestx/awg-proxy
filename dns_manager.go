package main

import "context"

type DNSManager interface {
	Apply(ctx context.Context, servers []string, cleanup *CleanupStack) error
}
