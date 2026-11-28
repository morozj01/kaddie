package proposal

import (
	"time"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
)

const (
	haltedExplanation          = "Kaddie is halted, or its SREConfig is missing or not in mode Enabled. Nothing may be executed until an operator sets mode Enabled."
	executionPausedExplanation = "Execution is paused. The agent may still propose, but nothing may be executed until an operator sets mode Enabled."
	notOpenExplanation         = "The proposal already has an outcome, so it can't be acted on again."
	expiredExplanation         = "The proposal has expired and can't be acted on. Ask the agent for a new one."
)

// RefusalReason is a stable code for a failed acting precondition.
type RefusalReason string

const (
	// Halted: SREConfig/default is missing, unreadable, or in mode Halted.
	Halted RefusalReason = "Halted"
	// ExecutionPaused: mode is ExecutionPaused.
	ExecutionPaused RefusalReason = "ExecutionPaused"
	// NotOpen: the proposal already has an outcome.
	NotOpen RefusalReason = "NotOpen"
	// Expired: an expiry is configured and has passed.
	Expired RefusalReason = "Expired"
)

// Refusal is one failed acting precondition.
type Refusal struct {
	Reason      RefusalReason
	Explanation string
}

// CanAct reports every acting precondition the proposal fails (base §7.2):
// mode Enabled (a nil cfg is a missing SREConfig, so Halted), open, and
// unexpired, where expiry is proposals.expirySeconds and zero never expires.
// An empty result means a human may act. Risk findings are deliberately not
// an input: they change how a proposal is shown, never whether it can be
// acted on.
func CanAct(cfg *v1alpha1.SREConfig, p *v1alpha1.SREProposal, expiry time.Duration, now time.Time) []Refusal {
	var out []Refusal
	switch {
	case cfg == nil:
		out = append(out, Refusal{Reason: Halted, Explanation: haltedExplanation})
	case cfg.Spec.Mode == v1alpha1.ModeEnabled:
	case cfg.Spec.Mode == v1alpha1.ModeExecutionPaused:
		out = append(out, Refusal{Reason: ExecutionPaused, Explanation: executionPausedExplanation})
	default:
		// Halted, and any mode not recognised, which can't be read as
		// permission.
		out = append(out, Refusal{Reason: Halted, Explanation: haltedExplanation})
	}
	if !isOpen(p) {
		out = append(out, Refusal{Reason: NotOpen, Explanation: notOpenExplanation})
	}
	if expiry > 0 && !now.Before(p.CreationTimestamp.Add(expiry)) {
		out = append(out, Refusal{Reason: Expired, Explanation: expiredExplanation})
	}
	return out
}

// isOpen reports whether the proposal has no outcome yet (base §7.2). An Api
// proposal is open until kaddie-ui writes status.execution.phase. A Git
// proposal is closed by a Dismissed approval, a push failure, or a PR whose
// state is anything but Open, so an empty or unknown state can't be read as
// permission.
func isOpen(p *v1alpha1.SREProposal) bool {
	st := p.Status
	switch p.Spec.Type {
	case v1alpha1.ProposalTypeGit:
		return (st.Approval == nil || st.Approval.Phase != v1alpha1.ApprovalPhaseDismissed) &&
			st.PushFailure == nil &&
			(st.PR == nil || st.PR.State == v1alpha1.PRStateOpen)
	default:
		return st.Execution == nil || st.Execution.Phase == ""
	}
}
