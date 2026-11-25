//go:build e2e

package e2e

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/test/internal/testutil"
)

var update = flag.Bool("update", false, "rewrite test/e2e/golden from the cluster")

// Namespaces the permission enumeration looks at: kaddie-system, where the
// only namespaced Role lives, and an ordinary namespace.
var enumNamespaces = []string{systemNamespace, "default"}

var components = []string{"kaddie-agent", "kaddie-sessions", "kaddie-tools", "kaddie-ui", "kaddie-git-writer", "kaddie-renderer"}

// TestT1_T2_NoSecretsOrDangerousSubresources: kaddie-tools and kaddie-agent
// can't read secrets in any namespace, and can't use pods/exec, attach,
// portforward, proxy, services/proxy, nodes/proxy or serviceaccounts/token.
func TestT1_T2_NoSecretsOrDangerousSubresources(t *testing.T) {
	nsList := &corev1.NamespaceList{}
	if err := admin.List(ctx, nsList); err != nil {
		t.Fatal(err)
	}
	namespaces := []string{""} // "" = all namespaces / cluster-wide
	for _, ns := range nsList.Items {
		namespaces = append(namespaces, ns.Name)
	}
	for _, sa := range []string{"kaddie-tools", "kaddie-agent"} {
		cs := saClientset(t, sa)
		for _, ns := range namespaces {
			for _, verb := range []string{"get", "list", "watch"} {
				if can(t, cs, authorizationv1.ResourceAttributes{Namespace: ns, Verb: verb, Resource: "secrets"}) {
					t.Errorf("T1: %s may %s secrets in %q", sa, verb, ns)
				}
			}
		}
		for _, sub := range []struct{ res, sub string }{
			{"pods", "exec"}, {"pods", "attach"}, {"pods", "portforward"}, {"pods", "proxy"},
			{"services", "proxy"}, {"nodes", "proxy"}, {"serviceaccounts", "token"},
		} {
			for _, verb := range []string{"get", "create"} {
				for _, ns := range []string{"", systemNamespace, "default"} {
					if can(t, cs, authorizationv1.ResourceAttributes{Namespace: ns, Verb: verb, Resource: sub.res, Subresource: sub.sub}) {
						t.Errorf("T2: %s may %s %s/%s in %q", sa, verb, sub.res, sub.sub, ns)
					}
				}
			}
		}
	}
}

// TestT3_GT1_AgentAndToolsPermissions: kaddie-agent holds only the
// Kubernetes-default self-review permissions; kaddie-tools holds exactly the
// rules of kaddie-agent-read, its own ClusterRole and create sreproposals in
// kaddie-system, plus those defaults; neither may write a lease.
func TestT3_GT1_AgentAndToolsPermissions(t *testing.T) {
	restoreChart(t)
	for _, gitops := range []bool{false, true} {
		t.Run(fmt.Sprintf("gitops=%v", gitops), func(t *testing.T) {
			installChart(t, "--set", fmt.Sprintf("gitops.enabled=%v", gitops))
			for _, ns := range enumNamespaces {
				base := baselineSA(t, ns)
				for _, p := range base {
					if p.Group != "nonResourceURL" && !isKubernetesDefault(p) {
						t.Errorf("an unbound ServiceAccount holds %s in %s, beyond the Kubernetes defaults", p, ns)
					}
				}
				if extra := minus(rulesOf(t, saClientset(t, "kaddie-agent"), ns), base); len(extra) > 0 {
					t.Errorf("kaddie-agent holds more than the defaults in %s: %v", ns, extra)
				}

				want := expandRules(liveRules(t, "ClusterRole", "", "kaddie-agent-read"))
				want = append(want, expandRules(liveRules(t, "ClusterRole", "", "kaddie-tools"))...)
				if ns == systemNamespace {
					want = append(want, expandRules(liveRules(t, "Role", systemNamespace, "kaddie-agent-propose"))...)
				}
				got := minus(rulesOf(t, saClientset(t, "kaddie-tools"), ns), base)
				if d := diffPerms(dedupe(want), got); d != "" {
					t.Errorf("kaddie-tools in %s differs from base T3's set:\n%s", ns, d)
				}
				for _, p := range got {
					if p.Resource == "leases" && p.Verb != "get" && p.Verb != "list" && p.Verb != "watch" {
						t.Errorf("kaddie-tools may %s leases", p.Verb)
					}
				}
			}
		})
	}
}

// TestT4_GT2_GT4_HT1_EffectivePermissionsMatchGolden enumerates every
// component ServiceAccount's effective permissions (what `kubectl auth can-i
// --list` shows, minus what an unbound ServiceAccount has) with GitOps on
// and off, checks none is forbidden by base §5, checks no component can
// update another's lease, and compares the whole set with the reviewed
// golden files in test/e2e/golden (regenerate with -update).
func TestT4_GT2_GT4_HT1_EffectivePermissionsMatchGolden(t *testing.T) {
	restoreChart(t)
	for _, gitops := range []bool{false, true} {
		mode := map[bool]string{false: "gitops-off", true: "gitops-on"}[gitops]
		t.Run(mode, func(t *testing.T) {
			installChart(t, "--set", fmt.Sprintf("gitops.enabled=%v", gitops))
			for _, sa := range components {
				exists := gitops || (sa != "kaddie-git-writer" && sa != "kaddie-renderer")
				if !exists {
					if err := admin.Get(ctx, client.ObjectKey{Namespace: systemNamespace, Name: sa}, &corev1.ServiceAccount{}); !apierrors.IsNotFound(err) {
						t.Errorf("GT2: %s exists with GitOps off (err=%v)", sa, err)
					}
					continue
				}
				cs := saClientset(t, sa)
				var lines []string
				for _, ns := range enumNamespaces {
					for _, p := range minus(rulesOf(t, cs, ns), baselineSA(t, ns)) {
						if why := forbiddenPerm(p); why != "" {
							t.Errorf("T4: %s holds %s in %s: %s", sa, p, ns, why)
						}
						lines = append(lines, ns+"\t"+p.String())
					}
				}
				sort.Strings(lines)
				golden(t, fmt.Sprintf("can-i-%s-%s.txt", sa, mode), strings.Join(lines, "\n")+"\n")

				// No component can update another component's lease.
				for _, owner := range []string{"kaddie-sessions", "kaddie-git-writer", "kaddie-renderer"} {
					mayRenew := can(t, cs, authorizationv1.ResourceAttributes{Namespace: systemNamespace, Verb: "update",
						Group: "coordination.k8s.io", Resource: "leases", Name: owner + "-leader"})
					if mayRenew != (sa == owner) {
						t.Errorf("T4: %s may update lease %s-leader = %v", sa, owner, mayRenew)
					}
				}
			}
		})
	}
}

// TestT5_OperatorRoleAndBindings: kaddie-operator grants nothing outside
// kaddie.org except get/update/patch on the ClusterRole kaddie-agent-read,
// and the chart binds only the subjects listed in rbac.operators.
func TestT5_OperatorRoleAndBindings(t *testing.T) {
	opCS := humanClientset(t, "operator@example.com", strings.TrimPrefix(operatorGroup, "oidc:"))
	nobodyCS := humanClientset(t, "nobody@example.com")
	for _, ns := range enumNamespaces {
		for _, p := range minus(rulesOf(t, opCS, ns), rulesOf(t, nobodyCS, ns)) {
			okRBAC := p.Group == "rbac.authorization.k8s.io" && p.Resource == "clusterroles" && p.Name == "kaddie-agent-read" &&
				slices.Contains([]string{"get", "update", "patch"}, p.Verb)
			if p.Group != "kaddie.org" && !okRBAC {
				t.Errorf("an operator holds %s in %s, outside kaddie.org", p, ns)
			}
		}
	}

	crbs := &rbacv1.ClusterRoleBindingList{}
	if err := admin.List(ctx, crbs, client.MatchingLabels{"app.kubernetes.io/part-of": "kaddie"}); err != nil {
		t.Fatal(err)
	}
	rbs := &rbacv1.RoleBindingList{}
	if err := admin.List(ctx, rbs, client.MatchingLabels{"app.kubernetes.io/part-of": "kaddie"}); err != nil {
		t.Fatal(err)
	}
	type binding struct {
		name, role string
		subjects   []rbacv1.Subject
	}
	var all []binding
	for _, b := range crbs.Items {
		all = append(all, binding{b.Name, b.RoleRef.Name, b.Subjects})
	}
	for _, b := range rbs.Items {
		all = append(all, binding{b.Namespace + "/" + b.Name, b.RoleRef.Name, b.Subjects})
	}
	for _, b := range all {
		for _, s := range b.subjects {
			isOperator := b.role == "kaddie-operator" && s.Kind == "Group" && s.Name == operatorGroup
			isComponentSA := s.Kind == "ServiceAccount" && s.Namespace == systemNamespace && slices.Contains(components, s.Name)
			if b.role == "kaddie-operator" && !isOperator {
				t.Errorf("binding %s grants kaddie-operator to %+v, not in rbac.operators", b.name, s)
			}
			if b.role != "kaddie-operator" && !isComponentSA {
				t.Errorf("binding %s grants %s to %+v, which is no component ServiceAccount", b.name, b.role, s)
			}
		}
	}
}

// TestT42_GT3_OnlyOperatorsChangeSREConfig: kaddie-operator changes mode,
// triggersPaused and every settings field; no SRE service identity can change
// any of them; nobody may delete SREConfig.
func TestT42_GT3_OnlyOperatorsChangeSREConfig(t *testing.T) {
	restoreChart(t)
	installChart(t, "--set", "gitops.enabled=true")
	deleteSREConfig(t)
	t.Cleanup(func() { deleteSREConfig(t) })

	op := operator(t)
	if err := op.Create(ctx, &v1.SREConfig{ObjectMeta: metav1.ObjectMeta{Name: "default"}}); err != nil {
		t.Fatalf("operator creating SREConfig/default: %v", err)
	}
	patches := map[string]string{
		"mode":           `{"spec":{"mode":"Enabled","reason":"go","changedBy":"operator@example.com"}}`,
		"triggersPaused": `{"spec":{"triggersPaused":true}}`,
		"api.enabled":    `{"spec":{"api":{"enabled":false}}}`,
		"repositories":   `{"spec":{"repositories":[{"name":"r","url":"https://git.example.com/r.git","targetBranch":"main","editablePaths":["apps/**"],"pr":{"minRequiredApprovals":3}}]}}`,
	}
	cfg := &v1.SREConfig{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	for field, patch := range patches {
		if err := op.Patch(ctx, cfg.DeepCopy(), client.RawPatch(types.MergePatchType, []byte(patch))); err != nil {
			t.Errorf("operator changing %s: %v", field, err)
		}
	}
	got := getSREConfig(t)
	if got.Spec.Mode != v1.ModeEnabled || !got.Spec.TriggersPaused || *got.Spec.API.Enabled || len(got.Spec.Repositories) != 1 {
		t.Errorf("operator's changes not applied: %+v", got.Spec)
	}
	if err := op.Delete(ctx, cfg.DeepCopy()); !apierrors.IsForbidden(err) {
		t.Errorf("operator deleting SREConfig: err = %v, want Forbidden", err)
	}

	for _, sa := range components {
		c := asServiceAccount(t, sa)
		for field, patch := range patches {
			if err := c.Patch(ctx, cfg.DeepCopy(), client.RawPatch(types.MergePatchType, []byte(patch))); !apierrors.IsForbidden(err) {
				t.Errorf("%s changing %s: err = %v, want Forbidden", sa, field, err)
			}
		}
		if err := c.Delete(ctx, cfg.DeepCopy()); !apierrors.IsForbidden(err) {
			t.Errorf("%s deleting SREConfig: err = %v, want Forbidden", sa, err)
		}
	}
}

// TestHT1_SessionsReadsToolsPlusTriggers: kaddie-sessions' reads equal
// kaddie-tools' reads plus sretriggers, with GitOps on and off, before and
// after a rule is added to kaddie-agent-read (H11).
func TestHT1_SessionsReadsToolsPlusTriggers(t *testing.T) {
	restoreChart(t)
	t.Cleanup(func() { resetAgentRead(t) })
	added := rbacv1.PolicyRule{APIGroups: []string{"example.com"}, Resources: []string{"widgets"}, Verbs: []string{"get", "list", "watch"}}
	for _, gitops := range []bool{false, true} {
		t.Run(fmt.Sprintf("gitops=%v", gitops), func(t *testing.T) {
			resetAgentRead(t)
			installChart(t, "--set", fmt.Sprintf("gitops.enabled=%v", gitops))
			check := func(when string) {
				t.Helper()
				tools := reads(rulesOf(t, saClientset(t, "kaddie-tools"), "default"))
				// get on its own named lease comes from the lease template, not
				// from reading cluster state (tools' unnamed leases read covers it).
				sessions := slices.DeleteFunc(reads(rulesOf(t, saClientset(t, "kaddie-sessions"), "default")), func(p perm) bool {
					return p == perm{"coordination.k8s.io", "leases", "kaddie-sessions-leader", "get"}
				})
				want := dedupe(append(tools, perm{"kaddie.org", "sretriggers", "", "get"},
					perm{"kaddie.org", "sretriggers", "", "list"}, perm{"kaddie.org", "sretriggers", "", "watch"}))
				if d := diffPerms(want, sessions); d != "" {
					t.Errorf("%s: kaddie-sessions' reads != kaddie-tools' reads + sretriggers:\n%s", when, d)
				}
			}
			check("as installed")
			role := &rbacv1.ClusterRole{}
			if err := admin.Get(ctx, client.ObjectKey{Name: "kaddie-agent-read"}, role); err != nil {
				t.Fatal(err)
			}
			role.Rules = append(role.Rules, added)
			if err := admin.Update(ctx, role); err != nil {
				t.Fatal(err)
			}
			check("after adding a rule")
			if !slices.Contains(reads(rulesOf(t, saClientset(t, "kaddie-sessions"), "default")), perm{"example.com", "widgets", "", "watch"}) {
				t.Error("the added rule didn't reach kaddie-sessions")
			}
		})
	}
}

// --- helpers ---------------------------------------------------------------

// perm is one effective permission; Group "nonResourceURL" holds URL rules.
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

func expandRules(rules []rbacv1.PolicyRule) []perm {
	var out []perm
	for _, r := range rules {
		names := r.ResourceNames
		if len(names) == 0 {
			names = []string{""}
		}
		for _, g := range r.APIGroups {
			for _, res := range r.Resources {
				for _, n := range names {
					for _, v := range r.Verbs {
						out = append(out, perm{g, res, n, v})
					}
				}
			}
		}
		for _, u := range r.NonResourceURLs {
			for _, v := range r.Verbs {
				out = append(out, perm{"nonResourceURL", u, "", v})
			}
		}
	}
	return dedupe(out)
}

// rulesOf is `kubectl auth can-i --list -n ns` for the client's identity.
func rulesOf(t *testing.T, cs kubernetes.Interface, ns string) []perm {
	t.Helper()
	r, err := cs.AuthorizationV1().SelfSubjectRulesReviews().Create(ctx,
		&authorizationv1.SelfSubjectRulesReview{Spec: authorizationv1.SelfSubjectRulesReviewSpec{Namespace: ns}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status.Incomplete {
		t.Fatalf("rules review incomplete: %s", r.Status.EvaluationError)
	}
	var rules []rbacv1.PolicyRule
	for _, rr := range r.Status.ResourceRules {
		rules = append(rules, rbacv1.PolicyRule{Verbs: rr.Verbs, APIGroups: rr.APIGroups, Resources: rr.Resources, ResourceNames: rr.ResourceNames})
	}
	for _, nr := range r.Status.NonResourceRules {
		rules = append(rules, rbacv1.PolicyRule{Verbs: nr.Verbs, NonResourceURLs: nr.NonResourceURLs})
	}
	return expandRules(rules)
}

func can(t *testing.T, cs kubernetes.Interface, attrs authorizationv1.ResourceAttributes) bool {
	t.Helper()
	r, err := cs.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx,
		&authorizationv1.SelfSubjectAccessReview{Spec: authorizationv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &attrs}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return r.Status.Allowed
}

func saClientset(t *testing.T, sa string) kubernetes.Interface {
	t.Helper()
	return kubernetes.NewForConfigOrDie(tokenConfig(saToken(t, systemNamespace, sa)))
}

func humanClientset(t *testing.T, email string, groups ...string) kubernetes.Interface {
	t.Helper()
	return kubernetes.NewForConfigOrDie(tokenConfig(oidcToken(t, email, groups...)))
}

// baselineSA returns what an unbound ServiceAccount in another namespace
// may do in ns: the Kubernetes defaults every component inherits.
func baselineSA(t *testing.T, ns string) []perm {
	t.Helper()
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "kaddie-e2e-baseline", Namespace: "default"}}
	if err := admin.Create(ctx, sa); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatal(err)
	}
	return rulesOf(t, kubernetes.NewForConfigOrDie(tokenConfig(saToken(t, "default", sa.Name))), ns)
}

// isKubernetesDefault reports whether p is one of the permissions Kubernetes
// grants every authenticated identity: the self-review creates and, from
// about 1.33, reading the public ClusterTrustBundles.
func isKubernetesDefault(p perm) bool {
	selfReview := p.Verb == "create" && ((p.Group == "authorization.k8s.io" && strings.HasPrefix(p.Resource, "selfsubject")) ||
		(p.Group == "authentication.k8s.io" && p.Resource == "selfsubjectreviews"))
	trustBundles := p.Group == "certificates.k8s.io" && p.Resource == "clustertrustbundles" &&
		(p.Verb == "get" || p.Verb == "list" || p.Verb == "watch")
	return selfReview || trustBundles
}

func liveRules(t *testing.T, kind, ns, name string) []rbacv1.PolicyRule {
	t.Helper()
	if kind == "Role" {
		r := &rbacv1.Role{}
		if err := admin.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, r); err != nil {
			t.Fatal(err)
		}
		return r.Rules
	}
	r := &rbacv1.ClusterRole{}
	if err := admin.Get(ctx, client.ObjectKey{Name: name}, r); err != nil {
		t.Fatal(err)
	}
	return r.Rules
}

func reads(ps []perm) []perm {
	var out []perm
	for _, p := range ps {
		if p.Group != "nonResourceURL" && (p.Verb == "get" || p.Verb == "list" || p.Verb == "watch") {
			out = append(out, p)
		}
	}
	return out
}

func minus(a, b []perm) []perm {
	var out []perm
	for _, p := range a {
		if !slices.Contains(b, p) {
			out = append(out, p)
		}
	}
	return out
}

func dedupe(ps []perm) []perm {
	var out []perm
	for _, p := range ps {
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

func diffPerms(want, got []perm) string {
	var lines []string
	for _, p := range minus(want, got) {
		lines = append(lines, "  missing: "+p.String())
	}
	for _, p := range minus(got, want) {
		lines = append(lines, "  extra:   "+p.String())
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

var writeVerbs = []string{"create", "update", "patch", "delete", "deletecollection"}

// forbiddenPerm returns why base §5 forbids p for an SRE service identity.
func forbiddenPerm(p perm) string {
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

// golden compares got with test/e2e/golden/name, or rewrites it with -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := testutil.Path("test", "e2e", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (generate with: go test -tags e2e ./test/e2e -run EffectivePermissions -update)", err)
	}
	if string(want) != got {
		t.Errorf("effective permissions differ from %s:\n%s", name, lineDiff(string(want), got))
	}
}

func lineDiff(want, got string) string {
	w, g := strings.Split(strings.TrimSpace(want), "\n"), strings.Split(strings.TrimSpace(got), "\n")
	var lines []string
	for _, l := range w {
		if !slices.Contains(g, l) {
			lines = append(lines, "  - "+l)
		}
	}
	for _, l := range g {
		if !slices.Contains(w, l) {
			lines = append(lines, "  + "+l)
		}
	}
	return strings.Join(lines, "\n")
}
