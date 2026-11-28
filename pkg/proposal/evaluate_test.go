package proposal_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/pkg/proposal"
	"github.com/morozj01/kaddie/pkg/proposal/schema"
)

const (
	mergePatch  v1alpha1.PatchType = "application/merge-patch+json"
	strategic   v1alpha1.PatchType = "application/strategic-merge-patch+json"
	jsonPatch   v1alpha1.PatchType = "application/json-patch+json"
	payments                       = "payments"
	deployments                    = "deployments"
)

func req(verb v1alpha1.RequestVerb, apiVersion, resource, namespace, name string, pt v1alpha1.PatchType, body string) v1alpha1.APIRequest {
	return v1alpha1.APIRequest{RequestFields: v1alpha1.RequestFields{
		Verb: verb, APIVersion: apiVersion, Resource: resource, Namespace: namespace, Name: name, PatchType: pt, Body: body,
	}}
}

// proposalOf builds a valid Api proposal in kaddie-system.
func proposalOf(requests ...v1alpha1.APIRequest) *v1alpha1.SREProposal {
	return &v1alpha1.SREProposal{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "kaddie-system"},
		Spec: v1alpha1.SREProposalSpec{
			Type:          v1alpha1.ProposalTypeAPI,
			API:           &v1alpha1.APIProposal{Requests: requests},
			Justification: "j",
		},
	}
}

// summary renders findings as "Reason@request[ field]", in order.
func summary(findings []proposal.Finding) []string {
	var out []string
	for _, f := range findings {
		s := fmt.Sprintf("%s@%d", f.Reason, f.Request)
		if f.Field != "" {
			s += " " + f.Field
		}
		out = append(out, s)
	}
	return out
}

func evaluate(p *v1alpha1.SREProposal) []proposal.Finding {
	return proposal.EvaluateStatic(p, schema.Vendored())
}

func TestEvaluateStatic_OrdersProposalLevelFirstThenByRequestHygieneBeforeCatalogue(t *testing.T) {
	p := proposalOf(
		req("create", "v1", "configmaps", "", "", "", `{"metadata":{"name":"c","namespace":"kaddie-system"},"data":{"a":"<redacted:x>\u200b"}}`),
		req("delete", "v1", "secrets", payments, "s", "", ""),
		req("delete", "kaddie.org/v1alpha1", "sreconfigs", "", "default", "", ""),
	)
	p.Namespace = "default"
	for range 21 {
		p.Spec.Evidence = append(p.Spec.Evidence, v1alpha1.EvidenceRef{Kind: "Pod", Name: "p"})
	}
	want := []string{
		"ProposalNamespace@-1", "Malformed@-1",
		"RedactedPlaceholder@0", "InvisibleCharacter@0", "KaddieNamespace@0",
		"Secret@1",
		"KaddieGroup@2",
	}
	if diff := cmp.Diff(want, summary(evaluate(p))); diff != "" {
		t.Errorf("findings (-want +got):\n%s", diff)
	}
}

func TestEvaluateStatic_NoFindingsForABenignProposal(t *testing.T) {
	p := proposalOf(req("patch", "apps/v1", deployments, payments, "api", mergePatch, `{"spec":{"replicas":3}}`))
	if got := evaluate(p); len(got) != 0 {
		t.Errorf("findings %v, want none", summary(got))
	}
}

func TestEvaluateStatic_OneFindingPerRulePathAndOneHygieneFindingPerReason(t *testing.T) {
	// Two containers' images change, and the body holds two placeholders and
	// two zero-width spaces.
	body := `{"metadata":{"annotations":{"a":"<redacted:1>\u200b","b":"<redacted:2>\u200b"}},` +
		`"spec":{"template":{"spec":{"containers":[{"name":"a","image":"x"},{"name":"b","image":"y"}]}}}}`
	got := summary(evaluate(proposalOf(req("patch", "apps/v1", deployments, payments, "api", strategic, body))))
	want := []string{"RedactedPlaceholder@0", "InvisibleCharacter@0", "WorkloadField@0 **.containers[*].image"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("findings (-want +got):\n%s", diff)
	}
}

func TestEvaluateStatic_ChangedNamesTheChangeThatMatched(t *testing.T) {
	tests := []struct {
		name      string
		pt        v1alpha1.PatchType
		body      string
		field     string
		wantField string
	}{
		{"a leaf", strategic, `{"spec":{"template":{"spec":{"serviceAccountName":"admin"}}}}`, "**.serviceAccountName", "spec.template.spec.serviceAccountName"},
		{"a JSON Patch element", jsonPatch, `[{"op":"replace","path":"/spec/template/spec/containers/0/image","value":"x"}]`, "**.containers[*].image", "spec.template.spec.containers[0].image"},
		{"a descendant of the rule path", jsonPatch, `[{"op":"replace","path":"/spec/template/spec/containers/0/env/0/value","value":"x"}]`, "**.containers[*].env", "spec.template.spec.containers[0].env[0].value"},
		{"a replaced list", mergePatch, `{"spec":{"template":{"spec":{"containers":[{"name":"a"}]}}}}`, "**.containers[*].image", "spec.template.spec.containers"},
		{"a replaced template", jsonPatch, `[{"op":"replace","path":"/spec/template","value":{}}]`, "**.volumes", "spec.template"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var found []string
			for _, f := range evaluate(proposalOf(req("patch", "apps/v1", deployments, payments, "api", tt.pt, tt.body))) {
				if f.Field == tt.field {
					found = append(found, f.Changed)
				}
			}
			if diff := cmp.Diff([]string{tt.wantField}, found); diff != "" {
				t.Errorf("Changed for %s (-want +got):\n%s", tt.field, diff)
			}
		})
	}
}

func TestEvaluateStatic_FindingsCarryNoBodyContent(t *testing.T) {
	const canary = "CANARY-7f3a91"
	p := proposalOf(
		req("create", "v1", "secrets", payments, "", "", `{"metadata":{"name":"s"},"stringData":{"k":"`+canary+`"}}`),
		req("patch", "apps/v1", deployments, payments, "api", strategic,
			`{"spec":{"template":{"spec":{"containers":[{"name":"a","image":"`+canary+`","env":[{"name":"A","value":"`+canary+`"}]}]}}}}`),
		req("create", "v1", "configmaps", payments, "", "", `{"data":{"a":"<redacted:`+canary+`>\u200b"}}`),
		req("create", "v1", "configmaps", payments, "", "", `{"data":"`+canary+`"`),
	)
	findings := evaluate(p)
	if len(findings) < 5 {
		t.Fatalf("findings %v, want the proposal to be flagged several ways", summary(findings))
	}
	b, err := json.Marshal(findings)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), canary) {
		t.Errorf("a finding carries body content: %s", b)
	}
}

func TestEvaluateStatic_HygieneExplanations(t *testing.T) {
	var seen bool
	for _, f := range evaluate(proposalOf(req("create", "v1", "configmaps", payments, "", "", `{"data":{"a":"<redacted:x>"}}`))) {
		if f.Reason == proposal.RedactedPlaceholder {
			seen = true
			if !strings.Contains(f.Explanation, "placeholder text would be written in place of the real value") {
				t.Errorf("RedactedPlaceholder explanation %q doesn't say the placeholder text would be written", f.Explanation)
			}
		}
	}
	if !seen {
		t.Error("no RedactedPlaceholder finding")
	}
}

func TestEvaluateStatic_EveryFindingExplainsItself(t *testing.T) {
	p := proposalOf(
		req("delete", "v1", "secrets", payments, "s", "", ""),
		req("patch", "rbac.authorization.k8s.io/v1", "clusterroles", "", "kaddie-agent-read", jsonPatch, `[{"op":"add","path":"/rules/-","value":{"apiGroups":[""],"resources":["secrets"],"verbs":["get"]}}]`),
		req("create", "v1", "configmaps", payments, "", "", `{`),
		req("update", "v1", "configmaps", payments, "c", "", `{}`),
	)
	p.Namespace = "default"
	findings := evaluate(p)
	if len(findings) < 5 {
		t.Fatalf("findings %v, want at least five", summary(findings))
	}
	for _, f := range findings {
		if strings.TrimSpace(f.Explanation) == "" {
			t.Errorf("%s has no explanation", f.Reason)
		}
	}
}

// TestMalformed_Limits covers the limits whose inputs are too large to write
// into a vector.
func TestMalformed_Limits(t *testing.T) {
	cm := func(body string) v1alpha1.APIRequest {
		return req("create", "v1", "configmaps", payments, "", "", body)
	}
	bodyOf := func(n int) string {
		const overhead = len(`{"data":{"a":""}}`)
		return `{"data":{"a":"` + strings.Repeat("x", n-overhead) + `"}}`
	}
	runes := func(n int) string { return strings.Repeat("é", n) } // two bytes each: characters, not bytes, are counted
	many := func(n int) []v1alpha1.APIRequest {
		var rs []v1alpha1.APIRequest
		for range n {
			rs = append(rs, cm(`{}`))
		}
		return rs
	}
	withText := func(set func(*v1alpha1.SREProposalSpec)) func() *v1alpha1.SREProposal {
		return func() *v1alpha1.SREProposal {
			p := proposalOf(cm(`{}`))
			set(&p.Spec)
			return p
		}
	}
	tests := []struct {
		name  string
		build func() *v1alpha1.SREProposal
		want  []string
	}{
		{"a 65536-byte body", func() *v1alpha1.SREProposal { return proposalOf(cm(bodyOf(65536))) }, nil},
		{"a 65537-byte body", func() *v1alpha1.SREProposal { return proposalOf(cm(bodyOf(65537))) }, []string{"Malformed@0"}},
		{"20 requests", func() *v1alpha1.SREProposal { return proposalOf(many(20)...) }, nil},
		{"21 requests", func() *v1alpha1.SREProposal { return proposalOf(many(21)...) }, []string{"Malformed@-1"}},
		{"a 4000-character justification", withText(func(s *v1alpha1.SREProposalSpec) { s.Justification = runes(4000) }), nil},
		{"a 4001-character justification", withText(func(s *v1alpha1.SREProposalSpec) { s.Justification = runes(4001) }), []string{"Malformed@-1"}},
		{"a 4000-character risk explanation", withText(func(s *v1alpha1.SREProposalSpec) { s.RiskExplanation = runes(4000) }), nil},
		{"a 4001-character risk explanation", withText(func(s *v1alpha1.SREProposalSpec) { s.RiskExplanation = runes(4001) }), []string{"Malformed@-1"}},
		{"20 evidence references", withText(func(s *v1alpha1.SREProposalSpec) { s.Evidence = make([]v1alpha1.EvidenceRef, 20) }), nil},
		{"21 evidence references", withText(func(s *v1alpha1.SREProposalSpec) { s.Evidence = make([]v1alpha1.EvidenceRef, 21) }), []string{"Malformed@-1"}},
		{"a spec of five 64000-byte bodies, over 256 KiB", func() *v1alpha1.SREProposal {
			return proposalOf(cm(bodyOf(64000)), cm(bodyOf(64000)), cm(bodyOf(64000)), cm(bodyOf(64000)), cm(bodyOf(64000)))
		}, []string{"Malformed@-1"}},
		{"a spec of three 64000-byte bodies", func() *v1alpha1.SREProposal {
			return proposalOf(cm(bodyOf(64000)), cm(bodyOf(64000)), cm(bodyOf(64000)))
		}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, summary(evaluate(tt.build()))); diff != "" {
				t.Errorf("findings (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRenderPolicy(t *testing.T) {
	want := []string{
		"System integrity: ", "System integrity: ", "System integrity: ", "System integrity: ", "System integrity: ",
		"System integrity: ", "System integrity: ", "System integrity: ", "System integrity: ",
		"Sensitive data: ", "Sensitive data: ",
		"What code runs, and as whom: ",
		"Deployment mechanism: ",
		"Previewability: ", "Previewability: ",
	}
	got := proposal.RenderPolicy()
	if !strings.HasSuffix(got, "\n") || strings.HasSuffix(got, "\n\n") {
		t.Errorf("policy %q doesn't end in exactly one newline", got)
	}
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != len(want) {
		t.Fatalf("%d lines, want %d:\n%s", len(lines), len(want), got)
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, want[i]) || len(line) == len(want[i]) {
			t.Errorf("line %d is %q, want %q followed by a summary", i, line, want[i])
		}
	}
	for i, p := range proposal.Protections() {
		if lines[i] != p.Group+": "+p.Summary {
			t.Errorf("line %d is %q, want the protection %s", i, lines[i], p.Reason)
		}
	}
	if again := proposal.RenderPolicy(); again != got {
		t.Error("RenderPolicy is not deterministic")
	}
	if !strings.Contains(got, "kaddie-agent-read") {
		t.Error("the RBAC line doesn't tell the model about the kaddie-agent-read exception")
	}
}

func TestProtections_AreTheCatalogueOfBase6_3InOrder(t *testing.T) {
	want := []proposal.Reason{
		proposal.KaddieGroup, proposal.KaddieNamespace, proposal.SystemNamespace, proposal.RBAC, proposal.AdmissionControl,
		proposal.CustomResourceDefinition, proposal.APIService, proposal.ServiceAccountToken, proposal.CSRApproval,
		proposal.Secret, proposal.SecretDeliveryKind, proposal.WorkloadField, proposal.ReconcilerKind,
		proposal.DeleteCollection, proposal.ConnectSubresource,
	}
	var got []proposal.Reason
	for _, p := range proposal.Protections() {
		got = append(got, p.Reason)
		if p.Group == "" || p.Summary == "" || p.Explanation == "" || len(p.Rules) == 0 {
			t.Errorf("%s is missing its group, summary, explanation or rules", p.Reason)
		}
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("protections (-want +got):\n%s", diff)
	}
	if proposal.ProtectionsVersion != "1" {
		t.Errorf("ProtectionsVersion = %q, want 1", proposal.ProtectionsVersion)
	}
}

// TestProtections_NothingCanChangeTheCatalogue: the catalogue is compiled in.
// Protections returns a copy, so a caller that edits it changes nothing, and
// no Helm value names it.
func TestProtections_NothingCanChangeTheCatalogue(t *testing.T) {
	pristine := proposal.Protections()
	policy := proposal.RenderPolicy()

	got := proposal.Protections()
	for i := range got {
		got[i].Reason, got[i].Group, got[i].Summary, got[i].Explanation = "X", "X", "X", "X"
		for j := range got[i].Rules {
			r := &got[i].Rules[j]
			clear(r.APIGroups)
			clear(r.Resources)
			clear(r.Verbs)
			clear(r.Namespaces)
			clear(r.Names)
			for k := range r.Fields {
				r.Fields[k].AnyDepth = false
				clear(r.Fields[k].Steps)
			}
		}
		got[i].Rules = nil
	}

	if diff := cmp.Diff(pristine, proposal.Protections()); diff != "" {
		t.Errorf("mutating the copy changed the catalogue (-before +after):\n%s", diff)
	}
	if proposal.RenderPolicy() != policy {
		t.Error("mutating the copy changed the policy summary")
	}
	flagged := evaluate(proposalOf(req("delete", "v1", "secrets", payments, "s", "", "")))
	if len(flagged) != 1 || flagged[0].Reason != proposal.Secret {
		t.Errorf("a Secret request is %v after mutating the copy, want Secret", summary(flagged))
	}

	for _, f := range []string{"charts/kaddie/values.yaml", "charts/kaddie/values.schema.json"} {
		b, err := os.ReadFile(filepath.Join("../..", f))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(b)), "protections") {
			t.Errorf("%s has an input for the protections", f)
		}
	}
}

// TestSampleProposalsAreNotHighRisk runs the shipped samples themselves, not
// the copies in the vectors.
func TestSampleProposalsAreNotHighRisk(t *testing.T) {
	for _, name := range []string{"sreproposal-api.yaml", "sreproposal-api-read.yaml"} {
		b, err := os.ReadFile(filepath.Join("../../test/fixtures/yaml-samples/crds", name))
		if err != nil {
			t.Fatal(err)
		}
		var p v1alpha1.SREProposal
		if err := yaml.UnmarshalStrict(b, &p); err != nil {
			t.Fatal(err)
		}
		if got := evaluate(&p); len(got) != 0 {
			t.Errorf("%s: findings %v, want none", name, summary(got))
		}
	}
}
