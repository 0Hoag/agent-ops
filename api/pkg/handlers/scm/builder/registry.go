package builder

import (
	"fmt"
	"strings"
	"sync"

	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
)

type Factory func(scm.Config) scm.SCMProvider

type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

func NewRegistry() *Registry { return &Registry{factories: make(map[string]Factory)} }

func (r *Registry) Register(name string, factory Factory) error {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || factory == nil {
		return fmt.Errorf("provider name and factory are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("provider %q is already registered", name)
	}
	r.factories[name] = factory
	return nil
}

func (r *Registry) Build(name string, config scm.Config) (scm.SCMProvider, error) {
	r.mu.RLock()
	factory := r.factories[strings.ToLower(strings.TrimSpace(name))]
	r.mu.RUnlock()
	if factory == nil {
		return nil, fmt.Errorf("unknown SCM provider %q", name)
	}
	return factory(config), nil
}

var defaultRegistry = NewRegistry()

func Register(name string, factory Factory) error { return defaultRegistry.Register(name, factory) }
func Build(name string, config scm.Config) (scm.SCMProvider, error) {
	return defaultRegistry.Build(name, config)
}
func Default() *Registry { return defaultRegistry }
