package supervisor

import "context"

// noop is the wave-0 placeholder; every instance reports stopped.
type noop struct{ kind string }

func (n *noop) Start(context.Context, string) error   { return nil }
func (n *noop) Stop(context.Context, string) error    { return nil }
func (n *noop) Restart(context.Context, string) error { return nil }
func (n *noop) Status(context.Context, string) (Status, error) {
	return Status{State: StateStopped}, nil
}
func (n *noop) SetAutostart(context.Context, string, bool) error { return nil }
func (n *noop) Kind() string                                     { return n.kind }
