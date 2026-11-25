//go:build e2e

package e2e

import (
	"strconv"
	"strings"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/test/internal/testutil"
)

// TestT53_InstallsOnMinimumVersion: the chart is installed on this cluster,
// which runs 1.28 or later; the render-time refusal for 1.27 is in
// test/chart (TestT53_VersionGate).
func TestT53_InstallsOnMinimumVersion(t *testing.T) {
	v, err := clientset.Discovery().ServerVersion()
	if err != nil {
		t.Fatal(err)
	}
	if !atLeast(v, 1, 28) {
		t.Fatalf("cluster runs %s.%s, below the minimum 1.28", v.Major, v.Minor)
	}
	if out := helm(t, "status", releaseName, "--namespace", releaseNamespace); !strings.Contains(out, "STATUS: deployed") {
		t.Fatalf("release not deployed:\n%s", out)
	}
}

// TestT6_ProposalQuota is base T6's quota part: kaddie-system rejects
// SREProposal creates beyond its object quota.
func TestT6_ProposalQuota(t *testing.T) {
	restoreChart(t)
	installChart(t, "--set", "proposals.quota=3")
	deleteAllProposals(t)
	t.Cleanup(func() { deleteAllProposals(t) })

	tools := asServiceAccount(t, "kaddie-tools")
	for i := range 3 {
		if err := tools.Create(ctx, newAPIProposal()); err != nil {
			t.Fatalf("proposal %d of 3 rejected: %v", i+1, err)
		}
	}
	err := tools.Create(ctx, newAPIProposal())
	if err == nil {
		t.Fatal("a fourth proposal was admitted past a quota of 3")
	}
	if !apierrors.IsForbidden(err) || !strings.Contains(err.Error(), "exceeded quota") {
		t.Fatalf("fourth proposal: %v, want a quota rejection", err)
	}
}

// TestT7_GT5_CreateNeedsRBAC is base T7 (and GT5's T7 part): creating an
// SREProposal as an identity without create on sreproposals is denied; only
// kaddie-tools holds it, and only in kaddie-system.
func TestT7_GT5_CreateNeedsRBAC(t *testing.T) {
	t.Cleanup(func() { deleteAllProposals(t) })
	for _, sa := range []string{"kaddie-agent", "kaddie-sessions", "kaddie-ui"} {
		if err := asServiceAccount(t, sa).Create(ctx, newAPIProposal()); !apierrors.IsForbidden(err) {
			t.Errorf("%s created a proposal: err = %v, want Forbidden", sa, err)
		}
	}
	if err := operator(t).Create(ctx, newAPIProposal()); !apierrors.IsForbidden(err) {
		t.Errorf("an operator created a proposal: err = %v, want Forbidden", err)
	}
	if err := asHuman(t, "nobody@example.com").Create(ctx, newAPIProposal()); !apierrors.IsForbidden(err) {
		t.Errorf("an unbound human created a proposal: err = %v, want Forbidden", err)
	}

	tools := asServiceAccount(t, "kaddie-tools")
	for _, p := range []*v1.SREProposal{newAPIProposal(), newGitProposal()} {
		if err := tools.Create(ctx, p); err != nil {
			t.Errorf("kaddie-tools creating a %s proposal in kaddie-system: %v", p.Spec.Type, err)
		}
	}
	elsewhere := newAPIProposal()
	elsewhere.Namespace = "default"
	if err := tools.Create(ctx, elsewhere); !apierrors.IsForbidden(err) {
		t.Errorf("kaddie-tools created a proposal in default: err = %v, want Forbidden", err)
	}
}

// TestT8_GT5_SpecImmutableForClusterAdmin is base T8 (and GT5's T8 part):
// updating SREProposal.spec, even as cluster-admin, is denied.
func TestT8_GT5_SpecImmutableForClusterAdmin(t *testing.T) {
	t.Cleanup(func() { deleteAllProposals(t) })
	for _, p := range []*v1.SREProposal{newAPIProposal(), newGitProposal()} {
		if err := admin.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
		p.Spec.Justification = "rewritten by an admin"
		if err := admin.Update(ctx, p); err == nil || !strings.Contains(err.Error(), "spec is immutable") {
			t.Errorf("cluster-admin updating a %s proposal's spec: err = %v, want spec is immutable", p.Spec.Type, err)
		}
	}
}

// TestT43_SREConfigUntouchedByHelm is base T43: only "default" is a valid
// name; a fresh install has no SREConfig; helm upgrade, uninstall and a
// reinstall leave an existing SREConfig and its settings unchanged.
func TestT43_SREConfigUntouchedByHelm(t *testing.T) {
	restoreChart(t)
	deleteSREConfig(t)
	t.Cleanup(func() { deleteSREConfig(t) })

	if err := admin.Create(ctx, &v1.SREConfig{ObjectMeta: metav1.ObjectMeta{Name: "other"}}); !apierrors.IsInvalid(err) {
		t.Errorf("SREConfig named other: err = %v, want Invalid", err)
	}

	// Fresh install: no SREConfig.
	uninstallChart(t)
	installChart(t)
	if err := admin.Get(ctx, client.ObjectKey{Name: "default"}, &v1.SREConfig{}); !apierrors.IsNotFound(err) {
		t.Fatalf("after a fresh install, get SREConfig/default: err = %v, want NotFound", err)
	}

	enabled := false
	cfg := &v1.SREConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
		Spec: v1.SREConfigSpec{
			Mode: v1.ModeExecutionPaused, Reason: "INC-1", ChangedBy: "bob@example.com", TriggersPaused: true,
			API: v1.APISettings{Enabled: &enabled},
		},
	}
	if err := admin.Create(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	want := getSREConfig(t).Spec

	check := func(step string) {
		t.Helper()
		got := getSREConfig(t)
		if got.Spec.Mode != want.Mode || got.Spec.Reason != want.Reason || got.Spec.TriggersPaused != want.TriggersPaused ||
			got.Spec.API.Enabled == nil || *got.Spec.API.Enabled {
			t.Fatalf("after %s, SREConfig spec = %+v, want %+v", step, got.Spec, want)
		}
		if _, owned := got.Annotations["meta.helm.sh/release-name"]; owned {
			t.Fatalf("after %s, Helm owns SREConfig/default", step)
		}
	}
	installChart(t, "--set", "proposals.quota=999")
	check("helm upgrade")
	uninstallChart(t)
	check("helm uninstall")
	installChart(t)
	check("reinstall")
}

// TestT27_AgentReadSurvivesHelm is base T27's cluster-mode part: helm upgrade
// leaves rules added to kaddie-agent-read after install in place and its
// notes list new shipped allowlist entries; helm uninstall keeps the role.
func TestT27_AgentReadSurvivesHelm(t *testing.T) {
	restoreChart(t)
	t.Cleanup(func() { resetAgentRead(t) })
	resetAgentRead(t)

	role := &rbacv1.ClusterRole{}
	if err := admin.Get(ctx, client.ObjectKey{Name: "kaddie-agent-read"}, role); err != nil {
		t.Fatal(err)
	}
	// An operator adds a kind by hand, and an old install lacks a shipped entry.
	added := rbacv1.PolicyRule{APIGroups: []string{"cert-manager.io"}, Resources: []string{"certificates"}, Verbs: []string{"get", "list", "watch"}}
	role.Rules = append(role.Rules, added)
	role.Rules = removeResource(role.Rules, "batch", "cronjobs")
	if err := admin.Update(ctx, role); err != nil {
		t.Fatal(err)
	}

	out := helm(t, append(append([]string{"upgrade", releaseName, chartPath(), "--namespace", releaseNamespace, "--wait"}, defaultValues...), "--set", "proposals.quota=998")...)
	if !strings.Contains(out, "batch/cronjobs") {
		t.Errorf("upgrade notes don't list the missing shipped entry batch/cronjobs:\n%s", out)
	}
	if !hasRule(t, added) {
		t.Fatal("helm upgrade removed the hand-added rule")
	}

	uninstallChart(t)
	if !hasRule(t, added) {
		t.Fatal("helm uninstall deleted kaddie-agent-read or its hand-added rule")
	}
	installChart(t)
	if !hasRule(t, added) {
		t.Fatal("a reinstall overwrote kaddie-agent-read")
	}
}

// --- helpers ---------------------------------------------------------------

func chartPath() string { return testutil.Path("charts", "kaddie") }

func atLeast(v *version.Info, major, minor int) bool {
	ma, _ := strconv.Atoi(v.Major)
	mi, _ := strconv.Atoi(strings.TrimSuffix(v.Minor, "+"))
	return ma > major || (ma == major && mi >= minor)
}

func newAPIProposal() *v1.SREProposal {
	return &v1.SREProposal{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "e2e-", Namespace: systemNamespace},
		Spec: v1.SREProposalSpec{
			Type: v1.ProposalTypeAPI,
			API: &v1.APIProposal{Requests: []v1.APIRequest{{RequestFields: v1.RequestFields{
				Verb: "patch", APIVersion: "apps/v1", Resource: "deployments", Namespace: "payments", Name: "api",
				PatchType: "application/merge-patch+json", Body: `{"spec":{"replicas":3}}`,
			}}}},
			Justification: "e2e",
		},
	}
}

func newGitProposal() *v1.SREProposal {
	content := "kind: ConfigMap\n"
	return &v1.SREProposal{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "e2e-", Namespace: systemNamespace},
		Spec: v1.SREProposalSpec{
			Type: v1.ProposalTypeGit,
			Git: &v1.GitProposal{Repository: "platform-config", BaseSHA: "3f9c2e1b7a4d6c8e0f2a4b6c8d0e2f4a6b8c0d2e",
				Edits: []v1.GitEdit{{Path: "apps/a.yaml", Operation: "Create", Content: &content}}},
			Justification: "e2e",
		},
	}
}

func deleteAllProposals(t *testing.T) {
	t.Helper()
	if err := admin.DeleteAllOf(ctx, &v1.SREProposal{}, client.InNamespace(systemNamespace)); err != nil && !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
}

func getSREConfig(t *testing.T) *v1.SREConfig {
	t.Helper()
	cfg := &v1.SREConfig{}
	if err := admin.Get(ctx, client.ObjectKey{Name: "default"}, cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func deleteSREConfig(t *testing.T) {
	t.Helper()
	err := admin.Delete(ctx, &v1.SREConfig{ObjectMeta: metav1.ObjectMeta{Name: "default"}})
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
}

// resetAgentRead deletes kaddie-agent-read and reinstalls, so the chart
// renders the shipped role again (cluster mode renders it only when absent).
func resetAgentRead(t *testing.T) {
	t.Helper()
	err := admin.Delete(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "kaddie-agent-read"}})
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
	installChart(t)
}

func hasRule(t *testing.T, want rbacv1.PolicyRule) bool {
	t.Helper()
	role := &rbacv1.ClusterRole{}
	if err := admin.Get(ctx, client.ObjectKey{Name: "kaddie-agent-read"}, role); err != nil {
		if apierrors.IsNotFound(err) {
			return false
		}
		t.Fatal(err)
	}
	for _, r := range role.Rules {
		if equalRule(r, want) {
			return true
		}
	}
	return false
}

func equalRule(a, b rbacv1.PolicyRule) bool {
	return strings.Join(a.APIGroups, ",") == strings.Join(b.APIGroups, ",") &&
		strings.Join(a.Resources, ",") == strings.Join(b.Resources, ",") &&
		strings.Join(a.Verbs, ",") == strings.Join(b.Verbs, ",")
}

// removeResource drops one resource of one group from a rule list.
func removeResource(rules []rbacv1.PolicyRule, group, resource string) []rbacv1.PolicyRule {
	var out []rbacv1.PolicyRule
	for _, r := range rules {
		if len(r.APIGroups) == 1 && r.APIGroups[0] == group {
			var keep []string
			for _, res := range r.Resources {
				if res != resource {
					keep = append(keep, res)
				}
			}
			if len(keep) == 0 {
				continue
			}
			r.Resources = keep
		}
		out = append(out, r)
	}
	return out
}
