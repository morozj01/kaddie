// Package conformance loads the shared rule-engine vectors under
// test/conformance/proposal and checks an engine against them, so every
// consumer of pkg/proposal (kaddie-tools, kaddie-ui, any port) proves it
// evaluates proposals identically (base §6.5, T16).
package conformance

import (
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/pkg/proposal"
)

// defaultNamespace is the namespace of a vector whose proposal names none.
const defaultNamespace = "kaddie-system"

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

// Load reads every *.yaml file in dir, in name order. Each file is a YAML
// list of vectors. A field no vector type has, an unnamed vector and a
// repeated name are errors, so a typo can't silently weaken a vector.
func Load(dir string) ([]Vector, error) {
	fsys := os.DirFS(dir)
	files, err := fs.Glob(fsys, "*.yaml")
	if err != nil {
		return nil, err
	}
	slices.Sort(files)
	var out []Vector
	names := map[string]string{}
	for _, f := range files {
		b, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		var vs []Vector
		if err := yaml.UnmarshalStrict(b, &vs); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		for _, v := range vs {
			switch prev, dup := names[v.Name]; {
			case v.Name == "":
				return nil, fmt.Errorf("%s: a vector has no name", f)
			case dup:
				return nil, fmt.Errorf("%s: vector %q is already in %s", f, v.Name, prev)
			}
			names[v.Name] = f
			if v.Proposal.Namespace == "" {
				v.Proposal.Namespace = defaultNamespace
			}
			out = append(out, v)
		}
	}
	return out, nil
}

// Check returns one line per difference between what e returns for v and
// what v expects; none means e conforms. Findings are compared as sets on
// reason, request, field and source. Exact findings are compared only when
// the vector has observed requests.
func Check(v Vector, e Engine) []string {
	diffs := compare("static", v.Static, e.Static(&v.Proposal))
	switch {
	case len(v.Observed) > 0 && e.Exact == nil:
		diffs = append(diffs, "exact: the vector has observed requests and the engine has no exact evaluation")
	case len(v.Observed) > 0:
		diffs = append(diffs, compare("exact", v.Exact, e.Exact(&v.Proposal, v.Observed))...)
	case len(v.Exact) > 0:
		diffs = append(diffs, "exact: the vector expects findings but has no observed requests")
	}
	return diffs
}

// compare returns a line for each expected finding missing from got and each
// found one the vector doesn't expect, sorted within each kind.
func compare(kind string, want []Expect, got []proposal.Finding) []string {
	wantKeys := map[Expect]bool{}
	for _, w := range want {
		wantKeys[w] = true
	}
	gotKeys := map[Expect]bool{}
	for _, g := range got {
		gotKeys[Expect{Reason: g.Reason, Request: g.Request, Field: g.Field, Source: g.Source}] = true
	}
	var out []string
	for _, side := range []struct {
		label     string
		have, not map[Expect]bool
	}{{"missing", wantKeys, gotKeys}, {"unexpected", gotKeys, wantKeys}} {
		var lines []string
		for k := range side.have {
			if !side.not[k] {
				lines = append(lines, fmt.Sprintf("%s: %s %s", kind, side.label, describe(k)))
			}
		}
		slices.Sort(lines)
		out = append(out, lines...)
	}
	return out
}

// describe renders one finding key.
func describe(k Expect) string {
	var b strings.Builder
	b.WriteString(string(k.Reason))
	if k.Request == proposal.NoRequest {
		b.WriteString(" on the proposal")
	} else {
		fmt.Fprintf(&b, " on request %d", k.Request)
	}
	if k.Field != "" {
		fmt.Fprintf(&b, " field %s", k.Field)
	}
	if k.Source != "" {
		fmt.Fprintf(&b, " source %s", k.Source)
	}
	return b.String()
}
