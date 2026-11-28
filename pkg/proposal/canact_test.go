package proposal_test

import (
	"slices"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/pkg/proposal"
)

func configIn(mode v1alpha1.Mode) *v1alpha1.SREConfig {
	return &v1alpha1.SREConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
		Spec:       v1alpha1.SREConfigSpec{Mode: mode},
	}
}

func reasons(rs []proposal.Refusal) []proposal.RefusalReason {
	var out []proposal.RefusalReason
	for _, r := range rs {
		out = append(out, r.Reason)
	}
	return out
}

// created is the creationTimestamp of the proposals below.
var created = time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

func apiProposal(phase v1alpha1.ExecutionPhase) *v1alpha1.SREProposal {
	p := proposalOf(req("patch", "apps/v1", "deployments", "payments", "api", mergePatch, `{}`))
	p.CreationTimestamp = metav1.NewTime(created)
	if phase != "" {
		p.Status.Execution = &v1alpha1.ExecutionStatus{Phase: phase}
	}
	return p
}

func gitProposal(mutate func(*v1alpha1.SREProposalStatus)) *v1alpha1.SREProposal {
	p := &v1alpha1.SREProposal{
		ObjectMeta: metav1.ObjectMeta{Name: "g", Namespace: "kaddie-system", CreationTimestamp: metav1.NewTime(created)},
		Spec:       v1alpha1.SREProposalSpec{Type: v1alpha1.ProposalTypeGit, Justification: "j"},
	}
	mutate(&p.Status)
	return p
}

// T45: with proposals.expirySeconds unset a proposal never expires; with it
// set it expires exactly at its creation time plus the expiry, whatever the
// spec says. (The retention loop and the executor's use of it: T4.8, T6.6.)
func TestT45_CanActExpiry(t *testing.T) {
	const expiry = time.Hour
	tests := []struct {
		name   string
		expiry time.Duration
		now    time.Time
		want   []proposal.RefusalReason
	}{
		{"unset expiry never expires, a day later", 0, created.Add(24 * time.Hour), nil},
		{"unset expiry never expires, a century later", 0, created.AddDate(100, 0, 0), nil},
		{"a negative expiry is unset", -time.Hour, created.AddDate(1, 0, 0), nil},
		{"one nanosecond before the expiry", expiry, created.Add(expiry - time.Nanosecond), nil},
		{"exactly at the expiry", expiry, created.Add(expiry), []proposal.RefusalReason{proposal.Expired}},
		{"after the expiry", expiry, created.Add(expiry + time.Second), []proposal.RefusalReason{proposal.Expired}},
		{"before the proposal was created", expiry, created.Add(-time.Hour), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reasons(proposal.CanAct(configIn(v1alpha1.ModeEnabled), apiProposal(""), tt.expiry, tt.now))
			if !slices.Equal(got, tt.want) {
				t.Errorf("CanAct = %v, want %v", got, tt.want)
			}
		})
	}
}

// The expiry is measured from creationTimestamp alone: no field of the spec
// moves it.
func TestT45_ExpiryIgnoresTheSpec(t *testing.T) {
	p := apiProposal("")
	p.Spec.Justification = "expires in a year, honest"
	p.Spec.RiskExplanation = "expiry: 8760h"
	got := reasons(proposal.CanAct(configIn(v1alpha1.ModeEnabled), p, time.Hour, created.Add(2*time.Hour)))
	if want := []proposal.RefusalReason{proposal.Expired}; !slices.Equal(got, want) {
		t.Errorf("CanAct = %v, want %v", got, want)
	}
}

// Base 7.2: the mode must be Enabled. A missing SREConfig, or a mode this
// version doesn't know, fails closed as Halted.
func TestCanActMode(t *testing.T) {
	tests := []struct {
		name string
		cfg  *v1alpha1.SREConfig
		want []proposal.RefusalReason
	}{
		{"Enabled", configIn(v1alpha1.ModeEnabled), nil},
		{"Halted", configIn(v1alpha1.ModeHalted), []proposal.RefusalReason{proposal.Halted}},
		{"ExecutionPaused", configIn(v1alpha1.ModeExecutionPaused), []proposal.RefusalReason{proposal.ExecutionPaused}},
		{"missing config", nil, []proposal.RefusalReason{proposal.Halted}},
		{"empty mode", configIn(""), []proposal.RefusalReason{proposal.Halted}},
		{"unknown mode", configIn("Resumed"), []proposal.RefusalReason{proposal.Halted}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reasons(proposal.CanAct(tt.cfg, apiProposal(""), 0, created))
			if !slices.Equal(got, tt.want) {
				t.Errorf("CanAct = %v, want %v", got, tt.want)
			}
		})
	}
}

// Base 7.2: a proposal is open until it has an outcome.
func TestCanActOpen(t *testing.T) {
	notOpen := []proposal.RefusalReason{proposal.NotOpen}
	tests := []struct {
		name string
		p    *v1alpha1.SREProposal
		want []proposal.RefusalReason
	}{
		{"Api with no execution", apiProposal(""), nil},
		{"Api with an execution that has no phase", func() *v1alpha1.SREProposal {
			p := apiProposal("")
			p.Status.Execution = &v1alpha1.ExecutionStatus{}
			return p
		}(), nil},
		{"Api executed", apiProposal("Executed"), notOpen},
		{"Api partially executed", apiProposal("PartiallyExecuted"), notOpen},
		{"Api failed", apiProposal("Failed"), notOpen},
		{"Api dismissed", apiProposal("Dismissed"), notOpen},
		{"Git with no status", gitProposal(func(*v1alpha1.SREProposalStatus) {}), nil},
		{"Git ready for PR", gitProposal(func(s *v1alpha1.SREProposalStatus) {
			s.Approval = &v1alpha1.ApprovalStatus{Phase: "ReadyForPR"}
		}), nil},
		{"Git dismissed", gitProposal(func(s *v1alpha1.SREProposalStatus) {
			s.Approval = &v1alpha1.ApprovalStatus{Phase: "Dismissed"}
		}), notOpen},
		{"Git push failed", gitProposal(func(s *v1alpha1.SREProposalStatus) {
			s.PushFailure = &v1alpha1.PushFailure{Reason: "BaseTooOld"}
		}), notOpen},
		{"Git PR open", gitProposal(func(s *v1alpha1.SREProposalStatus) { s.PR = &v1alpha1.PRStatus{State: "Open"} }), nil},
		{"Git PR merged", gitProposal(func(s *v1alpha1.SREProposalStatus) { s.PR = &v1alpha1.PRStatus{State: "Merged"} }), notOpen},
		{"Git PR closed", gitProposal(func(s *v1alpha1.SREProposalStatus) { s.PR = &v1alpha1.PRStatus{State: "Closed"} }), notOpen},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reasons(proposal.CanAct(configIn(v1alpha1.ModeEnabled), tt.p, 0, created))
			if !slices.Equal(got, tt.want) {
				t.Errorf("CanAct = %v, want %v", got, tt.want)
			}
		})
	}
}

// Every failing precondition is reported, mode first, then open, then expiry,
// each with an explanation; risk findings are not an input.
func TestCanActReportsEveryFailure(t *testing.T) {
	p := apiProposal("Executed")
	got := proposal.CanAct(configIn(v1alpha1.ModeHalted), p, time.Hour, created.Add(2*time.Hour))
	want := []proposal.RefusalReason{proposal.Halted, proposal.NotOpen, proposal.Expired}
	if !slices.Equal(reasons(got), want) {
		t.Fatalf("CanAct = %v, want %v", reasons(got), want)
	}
	for _, r := range got {
		if r.Explanation == "" {
			t.Errorf("%s has no explanation", r.Reason)
		}
	}
	paused := reasons(proposal.CanAct(configIn(v1alpha1.ModeExecutionPaused), p, time.Hour, created.Add(2*time.Hour)))
	if want := []proposal.RefusalReason{proposal.ExecutionPaused, proposal.NotOpen, proposal.Expired}; !slices.Equal(paused, want) {
		t.Errorf("paused CanAct = %v, want %v", paused, want)
	}
}

// A high-risk proposal can be acted on: risk only changes how it is shown.
func TestCanActIgnoresRisk(t *testing.T) {
	p := apiProposal("")
	p.Namespace = "elsewhere"
	p.Spec.API.Requests[0] = req("create", "v1", "secrets", "payments", "", "", `{"metadata":{"name":"s"}}`)
	if len(evaluate(p)) == 0 {
		t.Fatal("the proposal should be high risk")
	}
	if got := proposal.CanAct(configIn(v1alpha1.ModeEnabled), p, 0, created); len(got) != 0 {
		t.Errorf("CanAct = %v, want none", got)
	}
}
