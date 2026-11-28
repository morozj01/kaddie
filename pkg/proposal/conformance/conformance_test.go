package conformance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/pkg/proposal"
	"github.com/morozj01/kaddie/pkg/proposal/conformance"
)

func fixed(static, exact []proposal.Finding) conformance.Engine {
	return conformance.Engine{
		Static: func(*v1alpha1.SREProposal) []proposal.Finding { return static },
		Exact:  func(*v1alpha1.SREProposal, []proposal.Observed) []proposal.Finding { return exact },
	}
}

func finding(reason proposal.Reason, request int, field string, source proposal.Source) proposal.Finding {
	return proposal.Finding{Reason: reason, Request: request, Field: field, Source: source, Explanation: "text that is never compared"}
}

// TestT16_CheckReportsMissingAndExtraFindings is the control for the whole
// corpus: a comparison that can't fail would let every vector pass.
func TestT16_CheckReportsMissingAndExtraFindings(t *testing.T) {
	v := conformance.Vector{Static: []conformance.Expect{
		{Reason: proposal.Secret, Request: 0},
		{Reason: proposal.RBAC, Request: 2},
	}}
	e := fixed([]proposal.Finding{finding(proposal.Secret, 0, "", ""), finding(proposal.ReconcilerKind, 1, "", "")}, nil)

	got := conformance.Check(v, e)
	want := []string{
		"static: missing RBAC on request 2",
		"static: unexpected ReconcilerKind on request 1",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("differences:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestT16_CheckPassesAConformingEngine(t *testing.T) {
	v := conformance.Vector{Static: []conformance.Expect{
		{Reason: proposal.Secret, Request: 0},
		{Reason: proposal.ProposalNamespace, Request: proposal.NoRequest},
	}}
	e := fixed([]proposal.Finding{
		finding(proposal.ProposalNamespace, proposal.NoRequest, "", ""),
		finding(proposal.Secret, 0, "", ""),
		finding(proposal.Secret, 0, "", ""), // a repeated finding is the same member of the set
	}, nil)
	if got := conformance.Check(v, e); len(got) != 0 {
		t.Errorf("differences %v, want none", got)
	}
	if got := conformance.Check(conformance.Vector{}, fixed(nil, nil)); len(got) != 0 {
		t.Errorf("an empty vector and an empty engine differ: %v", got)
	}
}

func TestT16_CheckComparesFieldAndSource(t *testing.T) {
	v := conformance.Vector{
		Static:   []conformance.Expect{{Reason: proposal.WorkloadField, Request: 0, Field: "**.volumes"}},
		Observed: []proposal.Observed{{Request: 0}},
		Exact:    []conformance.Expect{{Reason: proposal.WorkloadField, Request: 0, Field: "**.volumes", Source: proposal.SourceRequest}},
	}
	e := fixed(
		[]proposal.Finding{finding(proposal.WorkloadField, 0, "**.hostNetwork", "")},
		[]proposal.Finding{finding(proposal.WorkloadField, 0, "**.volumes", proposal.SourceAdmission)},
	)
	got := strings.Join(conformance.Check(v, e), "\n")
	for _, want := range []string{
		"static: missing WorkloadField on request 0 field **.volumes",
		"static: unexpected WorkloadField on request 0 field **.hostNetwork",
		"exact: missing WorkloadField on request 0 field **.volumes source Request",
		"exact: unexpected WorkloadField on request 0 field **.volumes source Admission",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("differences lack %q:\n%s", want, got)
		}
	}
}

func TestCheck_ExactIsCheckedOnlyWithObservedRequests(t *testing.T) {
	stray := []proposal.Finding{finding(proposal.Secret, 0, "", proposal.SourceRequest)}
	if got := conformance.Check(conformance.Vector{}, fixed(nil, stray)); len(got) != 0 {
		t.Errorf("exact findings were compared without observed requests: %v", got)
	}
	v := conformance.Vector{Observed: []proposal.Observed{{Request: 0}}}
	if got := conformance.Check(v, fixed(nil, stray)); len(got) != 1 || !strings.HasPrefix(got[0], "exact: unexpected Secret") {
		t.Errorf("differences %v, want the unexpected exact finding", got)
	}
	noExact := conformance.Engine{Static: func(*v1alpha1.SREProposal) []proposal.Finding { return nil }}
	if got := conformance.Check(v, noExact); len(got) != 1 {
		t.Errorf("an engine with no exact evaluation passed observed requests: %v", got)
	}
	expects := conformance.Vector{Exact: []conformance.Expect{{Reason: proposal.Secret, Request: 0}}}
	if got := conformance.Check(expects, fixed(nil, nil)); len(got) != 1 {
		t.Errorf("exact findings with nothing observed passed: %v", got)
	}
}

func writeVectors(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoad_ReadsEveryYAMLFileInNameOrderAndDefaultsTheNamespace(t *testing.T) {
	dir := writeVectors(t, map[string]string{
		"b.yaml": "- name: second\n  proposal:\n    metadata: {namespace: payments}\n  static: [{reason: Secret, request: 0}]\n",
		"a.yaml": "- name: first\n  tests: [T15]\n  proposal: {}\n  static: []\n- name: third\n  proposal: {}\n",
		"c.yml":  "- name: ignored\n",
		"d.txt":  "not yaml at all: [",
	})
	got, err := conformance.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names, namespaces []string
	for _, v := range got {
		names = append(names, v.Name)
		namespaces = append(namespaces, v.Proposal.Namespace)
	}
	if strings.Join(names, ",") != "first,third,second" {
		t.Errorf("vectors %v, want first,third,second", names)
	}
	if strings.Join(namespaces, ",") != "kaddie-system,kaddie-system,payments" {
		t.Errorf("namespaces %v, want kaddie-system twice then payments", namespaces)
	}
	if got[2].Static[0].Reason != proposal.Secret || got[0].Tests[0] != "T15" {
		t.Errorf("fields not read: %+v", got)
	}
}

func TestLoad_RejectsWhatWouldWeakenAVector(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"an unknown field":    {"a.yaml": "- name: x\n  statik: []\n"},
		"an unnamed vector":   {"a.yaml": "- proposal: {}\n"},
		"a repeated name":     {"a.yaml": "- name: x\n", "b.yaml": "- name: x\n"},
		"not a list":          {"a.yaml": "name: x\n"},
		"a malformed finding": {"a.yaml": "- name: x\n  static: [{reason: Secret, requests: 0}]\n"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := conformance.Load(writeVectors(t, files)); err == nil {
				t.Error("Load accepted it")
			}
		})
	}
}
