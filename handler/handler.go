// Package handler defines the death-action handler contract and a
// self-registration registry. Concrete handlers (email, http-delete,
// gh-post, ...) live in subpackages and register themselves at init;
// the trigger layer dispatches decrypted payloads to them by name.
package handler

import (
	"context"
	"sort"
)

// Handler executes one death-action from a decrypted payload.
type Handler interface {
	// Name is the registry key an action entry references.
	Name() string
	// Run executes the action against the decrypted payload. A non-nil
	// error marks the action failed; the trigger isolates that failure
	// from the other actions.
	Run(ctx context.Context, payload []byte) error
}

var registry = map[string]Handler{}

// Register adds h under h.Name(); intended to be called from a handler
// package's init(). An empty name or a duplicate registration panics
// (programmer error, surfaced at startup).
func Register(h Handler) {
	name := h.Name()
	if name == "" {
		panic("handler: Register called with empty Name()")
	}
	if _, dup := registry[name]; dup {
		panic("handler: duplicate registration for " + name)
	}
	registry[name] = h
}

// Get returns the handler registered under name.
func Get(name string) (Handler, bool) {
	h, ok := registry[name]
	return h, ok
}

// Names returns the registered handler names, sorted, for diagnostics.
func Names() []string {
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
