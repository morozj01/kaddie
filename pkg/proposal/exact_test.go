package proposal_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/morozj01/kaddie/pkg/proposal"
	"github.com/morozj01/kaddie/pkg/proposal/schema"
)

const (
	liveDeployment = `{"metadata":{"name":"api"},"spec":{"replicas":2,"template":{"spec":{"containers":[{"name":"app","image":"nginx:1.27"}]}}}}`
	scaleTo3       = `{"spec":{"replicas":3}}`
)

// A finding names the exact field that changed, and never quotes values.
func TestExactFindingNamesTheChangedFieldWithoutValues(t *testing.T) {
	p := proposalOf(req("patch", "apps/v1", deployments, payments, "api", strategic, scaleTo3))
	dry := `{"metadata":{"name":"api"},"spec":{"replicas":3,"template":{"spec":{"containers":[{"name":"app","image":"registry.example.net/secret-mirror:9"}]}}}}`
	got := proposal.EvaluateExact(p, []proposal.Observed{{Request: 0, Live: json.RawMessage(liveDeployment), DryRun: json.RawMessage(dry)}}, schema.Vendored())
	if len(got) != 1 {
		t.Fatalf("findings = %+v, want one", got)
	}
	f := got[0]
	if f.Changed != "spec.template.spec.containers[0].image" || f.Source != proposal.SourceAdmission {
		t.Errorf("finding = %+v, want changed spec.template.spec.containers[0].image from Admission", f)
	}
	if strings.Contains(f.Explanation, "secret-mirror") || f.Explanation == "" {
		t.Errorf("explanation %q must be fixed text", f.Explanation)
	}
}

// Findings come in request order whatever order the observed entries arrive.
func TestExactFindingsAreInRequestOrder(t *testing.T) {
	p := proposalOf(
		req("patch", "apps/v1", deployments, payments, "api", strategic, scaleTo3),
		req("patch", "apps/v1", deployments, payments, "api", strategic, scaleTo3),
	)
	dry := `{"spec":{"replicas":3,"template":{"spec":{"containers":[{"name":"app","image":"other"}]}}}}`
	obs := func(i int) proposal.Observed {
		return proposal.Observed{Request: i, Live: json.RawMessage(liveDeployment), DryRun: json.RawMessage(dry)}
	}
	got := summary(proposal.EvaluateExact(p, []proposal.Observed{obs(1), obs(0)}, schema.Vendored()))
	want := []string{"WorkloadField@0 **.containers[*].image", "WorkloadField@1 **.containers[*].image"}
	if !slices.Equal(got, want) {
		t.Errorf("findings = %v, want %v", got, want)
	}
}

// When the observed objects can't be read, exact falls back on what the
// request may change, so it never reports less than static; a role it can't
// read fails the kaddie-agent-read exception.
func TestExactFailsSafeOnUnreadableObservations(t *testing.T) {
	image := req("patch", "apps/v1", deployments, payments, "api", strategic, `{"spec":{"template":{"spec":{"containers":[{"name":"app","image":"x"}]}}}}`)
	role := req("patch", "rbac.authorization.k8s.io/v1", "clusterroles", "", "kaddie-agent-read", jsonPatch,
		`[{"op":"add","path":"/rules/-","value":{"apiGroups":["cert-manager.io"],"resources":["certificates"],"verbs":["get"]}}]`)
	p := proposalOf(image, role)
	bad := json.RawMessage(`not json`)
	got := summary(proposal.EvaluateExact(p, []proposal.Observed{
		{Request: 0, Live: json.RawMessage(liveDeployment), DryRun: bad},
		{Request: 1, DryRun: bad},
	}, schema.Vendored()))
	want := []string{"WorkloadField@0 **.containers[*].image", "ReadRoleRule@1"}
	if !slices.Equal(got, want) {
		t.Errorf("findings = %v, want %v", got, want)
	}
}

// A proposal with no Api requests has nothing to evaluate.
func TestExactWithoutRequests(t *testing.T) {
	p := proposalOf()
	p.Spec.API = nil
	if got := proposal.EvaluateExact(p, []proposal.Observed{{Request: 0}}, schema.Vendored()); len(got) != 0 {
		t.Errorf("findings = %+v, want none", got)
	}
}
