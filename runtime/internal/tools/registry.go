package tools

import (
	"fmt"
	"sync"
)

type RiskLevel string

const (
	RiskReadOnly RiskLevel = "read_only"
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
)

type Capability string

const (
	CapSystemMetricsRead   Capability = "system.metrics.read"
	CapProcessMetricsRead  Capability = "process.metrics.read"
	CapFSMetadataRead      Capability = "filesystem.metadata.read"
	CapFSContentRead       Capability = "filesystem.content.read"
	CapWorkflowExecute     Capability = "workflow.execute"
	CapAppContextRead      Capability = "application.context.read"
	CapOptimizationRequest Capability = "system.optimization.request"
)

type Handler func(args map[string]string) (string, error)

type Tool struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Risk        RiskLevel  `json:"risk"`
	Capability  Capability `json:"capability"`
	Handler     Handler    `json:"-"`
}

type Registry struct {
	mu          sync.RWMutex
	tools       map[string]Tool
	grantedCaps map[Capability]bool
	scopeCaps   map[string]map[Capability]bool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
		grantedCaps: map[Capability]bool{
			CapSystemMetricsRead:   true,
			CapProcessMetricsRead:  true,
			CapFSMetadataRead:      true,
			CapFSContentRead:       true,
			CapWorkflowExecute:     true,
			CapAppContextRead:      true,
			CapOptimizationRequest: true,
		},
		scopeCaps: make(map[string]map[Capability]bool),
	}
}

func (r *Registry) SetCapability(cap Capability, granted bool) {
	r.mu.Lock()
	r.grantedCaps[cap] = granted
	r.mu.Unlock()
}

func (r *Registry) SetScopeCapability(scope string, cap Capability, granted bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.scopeCaps[scope] == nil {
		r.scopeCaps[scope] = make(map[Capability]bool)
	}
	r.scopeCaps[scope][cap] = granted
}

func (r *Registry) RevokeAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.grantedCaps {
		r.grantedCaps[k] = false
	}
}

func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name] = t
}

func (r *Registry) Execute(name string, args map[string]string) (string, RiskLevel, error) {
	return r.ExecuteInScope("", name, args)
}

func (r *Registry) ExecuteInScope(scope, name string, args map[string]string) (string, RiskLevel, error) {
	r.mu.RLock()
	t, ok := r.tools[name]
	allowed := t.Capability == "" || r.grantedCaps[t.Capability]
	if allowed && scope != "" {
		if sc, exists := r.scopeCaps[scope]; exists {
			if capAllowed, hasOverride := sc[t.Capability]; hasOverride {
				allowed = capAllowed
			}
		}
	}
	r.mu.RUnlock()
	if !ok {
		return "", RiskReadOnly, fmt.Errorf("tool %q is not in allowlist", name)
	}
	if !allowed {
		return "", t.Risk, fmt.Errorf("capability %q is not granted for tool %q", t.Capability, name)
	}
	res, err := t.Handler(args)
	return res, t.Risk, err
}

func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}
