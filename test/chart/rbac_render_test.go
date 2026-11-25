package chart

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

// ruleTemplates are the parameterless rule templates of base §5, defined in
// charts/kaddie/templates/_rbac_rules.tpl.
var ruleTemplates = []string{
	"agent-read-allowlist", "agent-read-extra", "config-read", "proposal-read", "trigger-read",
	"proposal-create", "proposal-prune", "proposal-status-write", "trigger-status-write",
	"config-create", "config-update", "trigger-manage", "read-scope-edit", "session-use",
	"session-delete", "token-review", "access-review",
}

var leases = []string{"kaddie-sessions-leader", "kaddie-git-writer-leader", "kaddie-renderer-leader"}

// serviceRoles are the role objects bound to SRE service identities.
var serviceRoles = []string{"kaddie-agent-read", "kaddie-agent-propose", "kaddie-sessions", "kaddie-tools", "kaddie-git-writer", "kaddie-renderer"}

// TestT4_EveryRuleFromATemplate: rendering with GitOps on and off, every rule
// of every Role and ClusterRole equals a rule some named template renders
// (T4 static part, base §13).
func TestT4_EveryRuleFromATemplate(t *testing.T) {
	gitopsModes(t, func(t *testing.T, _ bool, args []string) {
		args = append(args, "--set", "agentRead.extraResources[0].group=cert-manager.io",
			"--set", "agentRead.extraResources[0].resource=certificates")
		catalog := ruleCatalog(t, args)
		for _, role := range ofKind(render(t, chartDir, args...), "Role", "ClusterRole") {
			for i, r := range rules(t, role) {
				if !slices.ContainsFunc(catalog, func(c rbacv1.PolicyRule) bool { return reflect.DeepEqual(c, r) }) {
					t.Errorf("%s %s rule %d %+v comes from no rule template", role.GetKind(), role.GetName(), i, r)
				}
			}
		}
	})
}

// TestT4_NoWildcardsOrAggregation: no role uses aggregationRule or any "*".
func TestT4_NoWildcardsOrAggregation(t *testing.T) {
	gitopsModes(t, func(t *testing.T, _ bool, args []string) {
		for _, role := range ofKind(render(t, chartDir, args...), "Role", "ClusterRole") {
			if _, ok := role.Object["aggregationRule"]; ok {
				t.Errorf("%s uses aggregationRule", role.GetName())
			}
			for _, p := range expand(rules(t, role)) {
				if strings.Contains(p.String(), "*") {
					t.Errorf("%s grants a wildcard: %s", role.GetName(), p)
				}
			}
		}
	})
}

// TestT4_NoForbiddenPermission checks every role bound to an SRE service
// identity against base §5's never-grant list, and that each lease rule
// names only the component's own lease.
func TestT4_NoForbiddenPermission(t *testing.T) {
	gitopsModes(t, func(t *testing.T, _ bool, args []string) {
		objs := render(t, chartDir, args...)
		for _, name := range serviceRoles {
			role := find(objs, "ClusterRole", name)
			if role == nil {
				role = find(objs, "Role", name)
			}
			if role == nil {
				continue // GitOps-only roles with GitOps off
			}
			for _, p := range expand(rules(t, role)) {
				if why := forbidden(p); why != "" {
					t.Errorf("%s grants %s: %s", name, p, why)
				}
				if p.Group == "coordination.k8s.io" && p.Resource == "leases" && (p.Verb == "update" || p.Verb == "patch" || p.Verb == "delete") {
					if p.Name != name+"-leader" {
						t.Errorf("%s may %s lease %q, not only its own", name, p.Verb, p.Name)
					}
				}
			}
		}
	})
}

// TestT5_OperatorRole: kaddie-operator grants nothing outside kaddie.org
// except get/update/patch on the ClusterRole kaddie-agent-read, and the chart
// binds only the subjects in rbac.operators.
func TestT5_OperatorRole(t *testing.T) {
	op := find(render(t, chartDir), "ClusterRole", "kaddie-operator")
	for _, p := range expand(rules(t, op)) {
		if p.Group == "kaddie.org" {
			continue
		}
		if p.Group == "rbac.authorization.k8s.io" && p.Resource == "clusterroles" && p.Name == "kaddie-agent-read" && slices.Contains([]string{"get", "update", "patch"}, p.Verb) {
			continue
		}
		t.Errorf("kaddie-operator grants %s outside kaddie.org", p)
	}

	if b := find(render(t, chartDir), "ClusterRoleBinding", "kaddie-operator"); b != nil {
		t.Error("default values bind kaddie-operator; rbac.operators is empty")
	}
	b := find(render(t, chartDir,
		"--set", "rbac.operators[0].kind=Group", "--set", "rbac.operators[0].name=oncall-sre",
		"--set", "rbac.operators[1].kind=User", "--set", "rbac.operators[1].name=alice@example.com",
	), "ClusterRoleBinding", "kaddie-operator")
	if b == nil {
		t.Fatal("rbac.operators set but no kaddie-operator binding rendered")
	}
	got := subjects(t, b)
	want := []rbacv1.Subject{
		{Kind: "Group", APIGroup: "rbac.authorization.k8s.io", Name: "oncall-sre"},
		{Kind: "User", APIGroup: "rbac.authorization.k8s.io", Name: "alice@example.com"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("operator subjects = %+v, want %+v", got, want)
	}
}

// TestRBACMatchesSamples compares every rendered role's effective
// permissions and every binding with yaml-samples/roles and rolebindings,
// apart from the samples' illustrative extras (T1.3).
func TestRBACMatchesSamples(t *testing.T) {
	gitopsModes(t, func(t *testing.T, gitops bool, args []string) {
		args = append(args, "--set", "rbac.operators[0].kind=Group", "--set", "rbac.operators[0].name=oncall-sre")
		objs := render(t, chartDir, args...)

		roleFiles, _ := filepath.Glob(filepath.Join(sampleDir(), "roles", "*.yaml"))
		for _, f := range roleFiles {
			want := sample(t, "roles", filepath.Base(f))
			got := find(objs, want.GetKind(), want.GetName())
			gitopsOnly := want.GetName() == "kaddie-git-writer" || want.GetName() == "kaddie-renderer"
			if !gitops && gitopsOnly {
				if got != nil {
					t.Errorf("%s rendered with GitOps off", want.GetName())
				}
				continue
			}
			if got == nil {
				t.Errorf("%s %s not rendered", want.GetKind(), want.GetName())
				continue
			}
			exp := expand(rules(t, want))
			// Illustrative extra: the cert-manager rule shown as added later.
			exp = slices.DeleteFunc(exp, func(p perm) bool { return p.Group == "cert-manager.io" })
			if !gitops {
				exp = slices.DeleteFunc(exp, func(p perm) bool { return p.Group == "argoproj.io" })
			}
			if diff := setDiff(exp, expand(rules(t, got))); diff != "" {
				t.Errorf("%s differs from its sample:\n%s", want.GetName(), diff)
			}
			for k, v := range want.GetAnnotations() {
				if got.GetAnnotations()[k] != v {
					t.Errorf("%s annotation %s = %q, want %q", want.GetName(), k, got.GetAnnotations()[k], v)
				}
			}
		}

		bindingFiles, _ := filepath.Glob(filepath.Join(sampleDir(), "rolebindings", "*.yaml"))
		for _, f := range bindingFiles {
			want := sample(t, "rolebindings", filepath.Base(f))
			got := find(objs, want.GetKind(), want.GetName())
			if !gitops && (want.GetName() == "kaddie-git-writer" || want.GetName() == "kaddie-renderer") {
				if got != nil {
					t.Errorf("binding %s rendered with GitOps off", want.GetName())
				}
				continue
			}
			if got == nil {
				t.Errorf("%s %s not rendered", want.GetKind(), want.GetName())
				continue
			}
			if got.GetNamespace() != want.GetNamespace() {
				t.Errorf("binding %s namespace = %q, want %q", want.GetName(), got.GetNamespace(), want.GetNamespace())
			}
			gr, _, _ := unstructured.NestedStringMap(got.Object, "roleRef")
			wr, _, _ := unstructured.NestedStringMap(want.Object, "roleRef")
			if !reflect.DeepEqual(gr, wr) {
				t.Errorf("binding %s roleRef = %v, want %v", want.GetName(), gr, wr)
			}
			if gs, ws := subjects(t, got), subjects(t, want); !reflect.DeepEqual(gs, ws) {
				t.Errorf("binding %s subjects = %+v, want %+v", want.GetName(), gs, ws)
			}
		}

		// Nothing beyond the samples: 7 role objects with GitOps on, 5 off.
		wantRoles := 5
		if gitops {
			wantRoles = 7
		}
		if n := len(ofKind(objs, "Role", "ClusterRole")); n != wantRoles {
			t.Errorf("rendered %d role objects, want %d", n, wantRoles)
		}
	})
}

// TestT27_AgentReadKeptByHelm checks the static half of T27: in cluster
// ownership mode the role carries helm.sh/resource-policy: keep, renders the
// installer's extraResources, and refuses extras the read-role rule forbids.
func TestT27_AgentReadKeptByHelm(t *testing.T) {
	role := find(render(t, chartDir, "--set", "agentRead.extraResources[0].group=cert-manager.io",
		"--set", "agentRead.extraResources[0].resource=certificates"), "ClusterRole", "kaddie-agent-read")
	if role.GetAnnotations()["helm.sh/resource-policy"] != "keep" {
		t.Error("kaddie-agent-read lacks helm.sh/resource-policy: keep")
	}
	if !slices.Contains(expand(rules(t, role)), perm{"cert-manager.io", "certificates", "", "watch"}) {
		t.Error("agentRead.extraResources not rendered into kaddie-agent-read")
	}

	for _, bad := range [][2]string{
		{"", "secrets"}, {"", "pods/exec"}, {"", "pods/portforward"}, {"", "serviceaccounts/token"},
		{"rbac.authorization.k8s.io", "roles"}, {"kaddie.org", "sreconfigs"},
		{"certificates.k8s.io", "certificatesigningrequests/approval"},
	} {
		_, err := helmTemplate(chartDir, "--set-string", "agentRead.extraResources[0].group="+bad[0],
			"--set", "agentRead.extraResources[0].resource="+bad[1])
		if err == nil || !strings.Contains(err.Error(), "read-role rule") {
			t.Errorf("extraResources %s/%s: err = %v, want a read-role rule refusal", bad[0], bad[1], err)
		}
	}
	if _, err := helmTemplate(chartDir, "--set", "agentRead.ownership=git"); err == nil {
		t.Error("agentRead.ownership=git rendered; git ownership mode isn't implemented yet (T7.2)")
	}
}

// --- helpers ---------------------------------------------------------------

func sampleDir() string {
	return filepath.Join(chartDir, "..", "..", "test", "fixtures", "yaml-samples")
}

// perm is one effective permission: group, resource, resourceName ("" for
// any) and verb.
type perm struct{ Group, Resource, Name, Verb string }

func (p perm) String() string {
	g := p.Group
	if g == "" {
		g = "core"
	}
	s := fmt.Sprintf("%s %s/%s", p.Verb, g, p.Resource)
	if p.Name != "" {
		s += "[" + p.Name + "]"
	}
	return s
}

func expand(rs []rbacv1.PolicyRule) []perm {
	var out []perm
	for _, r := range rs {
		names := r.ResourceNames
		if len(names) == 0 {
			names = []string{""}
		}
		for _, g := range r.APIGroups {
			for _, res := range r.Resources {
				for _, n := range names {
					for _, v := range r.Verbs {
						p := perm{g, res, n, v}
						if !slices.Contains(out, p) {
							out = append(out, p)
						}
					}
				}
			}
		}
		for _, u := range r.NonResourceURLs {
			out = append(out, perm{"nonResourceURL", u, "", strings.Join(r.Verbs, ",")})
		}
	}
	return out
}

func setDiff(want, got []perm) string {
	var lines []string
	for _, p := range want {
		if !slices.Contains(got, p) {
			lines = append(lines, "  missing: "+p.String())
		}
	}
	for _, p := range got {
		if !slices.Contains(want, p) {
			lines = append(lines, "  extra:   "+p.String())
		}
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

var writeVerbs = []string{"create", "update", "patch", "delete", "deletecollection"}

// forbidden returns why base §5 forbids p for an SRE service identity, or "".
func forbidden(p perm) string {
	switch {
	case p.Resource == "secrets":
		return "secrets, any verb"
	case slices.Contains([]string{"pods/exec", "pods/attach", "pods/portforward", "pods/proxy", "services/proxy", "nodes/proxy", "serviceaccounts/token", "certificatesigningrequests/approval"}, p.Resource):
		return "a forbidden subresource"
	case slices.Contains([]string{"impersonate", "escalate", "bind"}, p.Verb):
		return "a forbidden verb"
	case slices.Contains(writeVerbs, p.Verb) && slices.Contains([]string{"rbac.authorization.k8s.io", "admissionregistration.k8s.io", "apiextensions.k8s.io"}, p.Group):
		return "a write on RBAC, admission or CRDs"
	case slices.Contains(writeVerbs, p.Verb) && p.Resource == "sreconfigs":
		return "a write on sreconfigs"
	case p.Group == "*" || p.Resource == "*" || p.Verb == "*":
		return "a wildcard"
	}
	return ""
}

func rules(t *testing.T, role *unstructured.Unstructured) []rbacv1.PolicyRule {
	t.Helper()
	var cr rbacv1.ClusterRole
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(role.Object, &cr); err != nil {
		t.Fatalf("%s: %v", role.GetName(), err)
	}
	return cr.Rules
}

func subjects(t *testing.T, b *unstructured.Unstructured) []rbacv1.Subject {
	t.Helper()
	var crb rbacv1.ClusterRoleBinding
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(b.Object, &crb); err != nil {
		t.Fatalf("%s: %v", b.GetName(), err)
	}
	return crb.Subjects
}

// ruleCatalog renders every rule template on its own, by copying the chart
// to a temporary directory and adding a template that emits each one into a
// ConfigMap, and returns every rule they produce.
func ruleCatalog(t *testing.T, args []string) []rbacv1.PolicyRule {
	t.Helper()
	dir := t.TempDir()
	copyDir(t, chartDir, dir)

	var b strings.Builder
	b.WriteString("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: zz-rule-catalog\ndata:\n")
	for _, name := range ruleTemplates {
		fmt.Fprintf(&b, "  %s: |\n    {{- include \"kaddie.rules.%s\" . | nindent 4 }}\n", name, name)
	}
	for _, l := range leases {
		fmt.Fprintf(&b, "  lease-%s: |\n    {{- include \"kaddie.rules.lease\" %q | nindent 4 }}\n", l, l)
	}
	b.WriteString("{{- range .Values.gitops.reconcilerAdapters }}\n  reconciler-read-{{ . }}: |\n    {{- include \"kaddie.rules.reconciler-read\" . | nindent 4 }}\n{{- end }}\n")
	if err := os.WriteFile(filepath.Join(dir, "templates", "zz_catalog.yaml"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	cm := find(render(t, dir, args...), "ConfigMap", "zz-rule-catalog")
	if cm == nil {
		t.Fatal("catalog not rendered")
	}
	data, _, _ := unstructured.NestedStringMap(cm.Object, "data")
	var all []rbacv1.PolicyRule
	for name, text := range data {
		var rs []rbacv1.PolicyRule
		if err := yaml.UnmarshalStrict([]byte(text), &rs); err != nil {
			t.Fatalf("rule template %s: %v\n%s", name, err, text)
		}
		all = append(all, rs...)
	}
	return all
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}
