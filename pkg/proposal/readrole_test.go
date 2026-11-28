package proposal

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
)

func rule(groups, resources, verbs []string) rbacv1.PolicyRule {
	return rbacv1.PolicyRule{APIGroups: groups, Resources: resources, Verbs: verbs}
}

var reads = []string{"get", "list", "watch"}

// TestT25_ShippedReadRoleRulesPass: every rule of the sample kaddie-agent-read
// passes the read-role rule, one by one, so the role we ship is one the
// engine accepts as a starting point.
func TestT25_ShippedReadRoleRulesPass(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "yaml-samples", "roles", "clusterrole-kaddie-agent-read.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var role rbacv1.ClusterRole
	if err := yaml.UnmarshalStrict(b, &role); err != nil {
		t.Fatal(err)
	}
	if role.AggregationRule != nil {
		t.Error("the sample role has an aggregationRule")
	}
	if len(role.Rules) < 20 {
		t.Fatalf("read %d rules, want the whole sample role", len(role.Rules))
	}
	for i, r := range role.Rules {
		if v := readRoleViolations(r); len(v) != 0 {
			t.Errorf("rule %d (%v %v) fails: %v", i, r.APIGroups, r.Resources, v)
		}
	}
}

// TestT25_ReadRoleRuleViolations: each way a rule can fail base 5.4.4 is
// named, and a near-miss passes.
func TestT25_ReadRoleRuleViolations(t *testing.T) {
	tests := []struct {
		name string
		rule rbacv1.PolicyRule
		want string // a violation it must report; empty means the rule passes
	}{
		{"reads of pods", rule([]string{""}, []string{"pods", "pods/log"}, reads), ""},
		{"get alone", rule([]string{"apps"}, []string{"deployments/status"}, []string{"get"}), ""},
		{"kaddie.org sreproposals", rule([]string{"kaddie.org"}, []string{"sreproposals"}, reads), ""},
		{"a bare serviceaccounts grants no token", rule([]string{""}, []string{"serviceaccounts"}, reads), ""},
		{"a subresource of csr other than approval", rule([]string{"certificates.k8s.io"}, []string{"certificatesigningrequests", "certificatesigningrequests/status"}, reads), ""},
		{"a resource whose name starts like secrets", rule([]string{""}, []string{"secretsx"}, reads), ""},
		{"no verbs", rule([]string{""}, []string{"pods"}, nil), "no verbs"},
		{"create", rule([]string{""}, []string{"pods"}, []string{"get", "create"}), "verb other than get, list or watch"},
		{"wildcard verb", rule([]string{""}, []string{"pods"}, []string{"*"}), "wildcard verb"},
		{"wildcard group", rule([]string{"*"}, []string{"pods"}, reads), "wildcard API group"},
		{"wildcard resource", rule([]string{""}, []string{"*"}, reads), "wildcard resource"},
		{"wildcard subresource", rule([]string{""}, []string{"pods/*"}, reads), "wildcard resource"},
		{"resourceNames", rbacv1.PolicyRule{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: reads, ResourceNames: []string{"web"}}, "resourceNames"},
		{"nonResourceURLs", rbacv1.PolicyRule{NonResourceURLs: []string{"/healthz"}, Verbs: reads}, "nonResourceURLs"},
		{"secrets", rule([]string{""}, []string{"secrets"}, reads), "secrets"},
		{"secrets of another group", rule([]string{"example.com"}, []string{"secrets"}, reads), "secrets"},
		{"a secrets subresource", rule([]string{""}, []string{"secrets/status"}, reads), "secrets"},
		{"token", rule([]string{""}, []string{"serviceaccounts/token"}, reads), "serviceaccounts/token"},
		{"approval", rule([]string{"certificates.k8s.io"}, []string{"certificatesigningrequests/approval"}, reads), "certificatesigningrequests/approval"},
		{"exec", rule([]string{""}, []string{"pods/exec"}, reads), "exec, attach, portforward or proxy subresource"},
		{"attach", rule([]string{""}, []string{"pods/attach"}, reads), "exec, attach, portforward or proxy subresource"},
		{"portforward", rule([]string{""}, []string{"pods/portforward"}, reads), "exec, attach, portforward or proxy subresource"},
		{"proxy", rule([]string{""}, []string{"nodes/proxy"}, reads), "exec, attach, portforward or proxy subresource"},
		{"RBAC group", rule([]string{"rbac.authorization.k8s.io"}, []string{"roles"}, reads), "RBAC API group"},
		{"kaddie.org sreconfigs", rule([]string{"kaddie.org"}, []string{"sreconfigs"}, reads), "kaddie.org resource other than sreproposals"},
		{"kaddie.org sreproposals/status", rule([]string{"kaddie.org"}, []string{"sreproposals/status"}, reads), "kaddie.org resource other than sreproposals"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := readRoleViolations(tt.rule)
			switch {
			case tt.want == "" && len(got) != 0:
				t.Errorf("violations %v, want a passing rule", got)
			case tt.want != "" && !slices.Contains(got, tt.want):
				t.Errorf("violations %v, want %q among them", got, tt.want)
			}
		})
	}
}

func TestPatchPassesReadRole(t *testing.T) {
	const pass = `{"apiGroups":["cert-manager.io"],"resources":["certificates"],"verbs":["get","list","watch"]}`
	const fail = `{"apiGroups":[""],"resources":["secrets"],"verbs":["get"]}`
	tests := []struct {
		name string
		pt   v1alpha1.PatchType
		body string
		want bool
	}{
		{"append", v1alpha1.PatchTypeJSONPatch, `[{"op":"add","path":"/rules/-","value":` + pass + `}]`, true},
		{"insert", v1alpha1.PatchTypeJSONPatch, `[{"op":"add","path":"/rules/12","value":` + pass + `}]`, true},
		{"replace one", v1alpha1.PatchTypeJSONPatch, `[{"op":"replace","path":"/rules/0","value":` + pass + `}]`, true},
		{"replace all", v1alpha1.PatchTypeJSONPatch, `[{"op":"replace","path":"/rules","value":[` + pass + `,` + pass + `]}]`, true},
		{"remove one", v1alpha1.PatchTypeJSONPatch, `[{"op":"remove","path":"/rules/3"}]`, true},
		{"test then add", v1alpha1.PatchTypeJSONPatch, `[{"op":"test","path":"/rules/0","value":{}},{"op":"add","path":"/rules/-","value":` + pass + `}]`, true},
		{"merge", v1alpha1.PatchTypeMergePatch, `{"rules":[` + pass + `]}`, true},
		{"strategic", v1alpha1.PatchTypeStrategicMergePatch, `{"rules":[` + pass + `]}`, true},
		{"a failing rule", v1alpha1.PatchTypeJSONPatch, `[{"op":"add","path":"/rules/-","value":` + fail + `}]`, false},
		{"a failing rule after a passing one", v1alpha1.PatchTypeJSONPatch, `[{"op":"add","path":"/rules/-","value":` + pass + `},{"op":"add","path":"/rules/-","value":` + fail + `}]`, false},
		{"add the list", v1alpha1.PatchTypeJSONPatch, `[{"op":"add","path":"/rules","value":[` + pass + `]}]`, false},
		{"remove the list", v1alpha1.PatchTypeJSONPatch, `[{"op":"remove","path":"/rules"}]`, false},
		{"edit inside a rule", v1alpha1.PatchTypeJSONPatch, `[{"op":"add","path":"/rules/0/verbs/-","value":"get"}]`, false},
		{"move", v1alpha1.PatchTypeJSONPatch, `[{"op":"move","from":"/rules/0","path":"/rules/1"}]`, false},
		{"aggregationRule", v1alpha1.PatchTypeJSONPatch, `[{"op":"add","path":"/aggregationRule","value":{}}]`, false},
		{"a leading-zero index", v1alpha1.PatchTypeJSONPatch, `[{"op":"remove","path":"/rules/01"}]`, false},
		{"a missing value", v1alpha1.PatchTypeJSONPatch, `[{"op":"add","path":"/rules/-"}]`, false},
		{"a list where a rule belongs", v1alpha1.PatchTypeJSONPatch, `[{"op":"add","path":"/rules/-","value":[` + pass + `]}]`, false},
		{"a rule where the list belongs", v1alpha1.PatchTypeJSONPatch, `[{"op":"replace","path":"/rules","value":` + pass + `}]`, false},
		{"null patch", v1alpha1.PatchTypeJSONPatch, `null`, false},
		{"not JSON", v1alpha1.PatchTypeJSONPatch, `[{`, false},
		{"merge with metadata", v1alpha1.PatchTypeMergePatch, `{"rules":[` + pass + `],"metadata":{}}`, false},
		{"merge with a null list", v1alpha1.PatchTypeMergePatch, `{"rules":null}`, false},
		{"merge with no rules", v1alpha1.PatchTypeMergePatch, `{}`, false},
		{"strategic with a directive", v1alpha1.PatchTypeStrategicMergePatch, `{"rules":[{"$patch":"delete"}]}`, false},
		{"a rule with an unknown key", v1alpha1.PatchTypeMergePatch, `{"rules":[{"apiGroups":[""],"resources":["pods"],"verbs":["get"],"x":1}]}`, false},
		{"a rule with a key in another case", v1alpha1.PatchTypeMergePatch, `{"rules":[{"APIGroups":[""],"resources":["pods"],"verbs":["get"]}]}`, false},
		{"no patch type", "", `{"rules":[` + pass + `]}`, false},
		{"another patch type", "application/apply-patch+yaml", `{"rules":[` + pass + `]}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := patchPassesReadRole(tt.pt, tt.body); got != tt.want {
				t.Errorf("patchPassesReadRole = %v, want %v", got, tt.want)
			}
		})
	}
}
