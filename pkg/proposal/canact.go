package proposal

import (
	"time"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
)

// Fixed explanations of each refusal.
const (
	haltedExplanation          = "Kaddie is halted, or its SREConfig is missing or not in mode Enabled. Nothing may be executed until an operator sets mode Enabled."
	executionPausedExplanation = "Execution is paused. The agent may still propose, but nothing may be executed until an operator sets mode Enabled."
	notOpenExplanation         = "The proposal already has an outcome, so it can't be acted on again."
	expiredExplanation         = "The proposal has expired and can't be acted on. Ask the agent for a new one."
)

const (
	approvalDismissed v1alpha1.ApprovalPhase = "Dismissed"
	prMerged          v1alpha1.PRState       = "Merged"
	prClosed          v1alpha1.PRState       = "Closed"
)

// canAct is CanAct. Any mode other than the three known ones fails closed as
// Halted, since an unrecognised mode can't be read as permission.
func canAct(cfg *v1alpha1.SREConfig, p *v1alpha1.SREProposal, expiry time.Duration, now time.Time) []Refusal {
	var out []Refusal
	switch {
	case cfg == nil:
		out = append(out, Refusal{Reason: Halted, Explanation: haltedExplanation})
	case cfg.Spec.Mode == v1alpha1.ModeEnabled:
	case cfg.Spec.Mode == v1alpha1.ModeExecutionPaused:
		out = append(out, Refusal{Reason: ExecutionPaused, Explanation: executionPausedExplanation})
	default:
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
// proposal is closed by a Dismissed approval, a push failure, or a PR that
// was merged or closed.
func isOpen(p *v1alpha1.SREProposal) bool {
	st := p.Status
	switch p.Spec.Type {
	case v1alpha1.ProposalTypeGit:
		return (st.Approval == nil || st.Approval.Phase != approvalDismissed) &&
			st.PushFailure == nil &&
			(st.PR == nil || (st.PR.State != prMerged && st.PR.State != prClosed))
	default:
		return st.Execution == nil || st.Execution.Phase == ""
	}
}
