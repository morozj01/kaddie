// Package conformance loads the shared rule-engine vectors under
// test/conformance/proposal and checks an engine against them, so every
// consumer of pkg/proposal (kaddie-tools, kaddie-ui, any port) proves it
// evaluates proposals identically (base §6.5, T16).
package conformance

import (
	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/pkg/proposal"
)

// Vector is one case: a proposal, optionally what an executor observed for
// its requests, and the findings static and exact evaluation must return.
// Each file under test/conformance/proposal is a YAML list of vectors.
type Vector struct {
	Name string `json:"name"`
	// Tests lists the spec test IDs the vector covers, such as T15.
	Tests []string `json:"tests,omitempty"`
	// Proposal is the SREProposal under test. Its namespace defaults to
	// kaddie-system.
	Proposal v1alpha1.SREProposal `json:"proposal"`
	// Observed feeds exact evaluation; with none, Exact isn't checked.
	Observed []proposal.Observed `json:"observed,omitempty"`
	// Static and Exact are the expected findings, compared as sets on
	// reason, request, field and source; explanations aren't compared.
	Static []Expect `json:"static"`
	Exact  []Expect `json:"exact,omitempty"`
}

// Expect is one expected finding.
type Expect struct {
	Reason  proposal.Reason `json:"reason"`
	Request int             `json:"request"`
	Field   string          `json:"field,omitempty"`
	Source  proposal.Source `json:"source,omitempty"`
}

// Engine is what a consumer exposes for checking: its static and exact
// evaluation, however it wraps pkg/proposal.
type Engine struct {
	Static func(*v1alpha1.SREProposal) []proposal.Finding
	Exact  func(*v1alpha1.SREProposal, []proposal.Observed) []proposal.Finding
}

// Load reads every vector file in dir.
func Load(dir string) ([]Vector, error) {
	panic("unimplemented")
}

// Check returns one line per difference between what e returns for v and
// what v expects; none means e conforms.
func Check(v Vector, e Engine) []string {
	panic("unimplemented")
}
