package builder

import (
	"fmt"
	"strings"
	"sync"

	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm/github"
)

type Factory func() scm.SCMProvider

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

func (r *Registry) Build(name string) (scm.SCMProvider, error) {
	r.mu.RLock()
	factory := r.factories[strings.ToLower(strings.TrimSpace(name))]
	r.mu.RUnlock()
	if factory == nil {
		return nil, fmt.Errorf("unknown SCM provider %q", name)
	}
	return factory(), nil
}

var defaultRegistry = func() *Registry {
	r := NewRegistry()
	_ = r.Register("github", func() scm.SCMProvider { return github.New(github.Config{}) })
	return r
}()

func Register(name string, factory Factory) error { return defaultRegistry.Register(name, factory) }
func Build(name string) (scm.SCMProvider, error)  { return defaultRegistry.Build(name) }
func Default() *Registry                          { return defaultRegistry }