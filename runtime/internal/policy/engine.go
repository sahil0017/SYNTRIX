package policy

import (
	"fmt"

	"github.com/syntrix/runtime/internal/events"
	"github.com/syntrix/runtime/internal/store"
)

type Decision struct {
	Allowed         bool
	RequireApproval bool
	RiskLevel       string
	Reason          string
}

type Engine struct {
	bus *events.Bus
}

func NewEngine(bus *events.Bus) *Engine {
	return &Engine{bus: bus}
}

// ValidateAction evaluates risk and emits policy.validation.requested + policy.approval.required.
func (e *Engine) ValidateAction(corrID string, rec store.Recommendation, mode string) Decision {
	if e.bus != nil {
		e.bus.Publish(events.Event{
			Type:          events.PolicyValidationRequested,
			CorrelationID: corrID,
			Source:        "GoRuntime",
			Target:        "PolicyEngine",
			Summary:       fmt.Sprintf("Validate action '%s' (severity=%s, mode=%s)", rec.Kind, rec.Severity, mode),
		})
	}

	if mode == "OFF" {
		return Decision{
			Allowed:         false,
			RequireApproval: false,
			RiskLevel:       "blocked",
			Reason:          "SYNTRIX is in OFF mode; action blocked.",
		}
	}

	dec := Decision{
		Allowed:         true,
		RequireApproval: true,
		RiskLevel:       rec.Severity,
		Reason:          "Explicit user approval required by local security policy.",
	}

	if e.bus != nil {
		e.bus.Publish(events.Event{
			Type:          events.PolicyApprovalRequired,
			CorrelationID: corrID,
			Source:        "PolicyEngine",
			Target:        "GoRuntime",
			Summary:       fmt.Sprintf("Require user approval for '%s' (%s)", rec.Title, dec.Reason),
		})
	}

	return dec
}

// RevalidateAuthorization checks policy right before execution and emits policy.authorized or policy.denied.
func (e *Engine) RevalidateAuthorization(corrID string, recID string, mode string) bool {
	if e.bus != nil {
		e.bus.Publish(events.Event{
			Type:          events.PolicyRevalidationRequested,
			CorrelationID: corrID,
			Source:        "GoRuntime",
			Target:        "PolicyEngine",
			Summary:       fmt.Sprintf("Revalidate authorization for '%s' in mode %s", recID, mode),
		})
	}

	if mode == "OFF" {
		if e.bus != nil {
			e.bus.Publish(events.Event{
				Type:          events.PolicyDenied,
				CorrelationID: corrID,
				Source:        "PolicyEngine",
				Target:        "GoRuntime",
				Summary:       "Authorization denied: mode is OFF",
			})
		}
		return false
	}

	if e.bus != nil {
		e.bus.Publish(events.Event{
			Type:          events.PolicyAuthorized,
			CorrelationID: corrID,
			Source:        "PolicyEngine",
			Target:        "GoRuntime",
			Summary:       fmt.Sprintf("Authorized execution for '%s'", recID),
		})
	}
	return true
}
