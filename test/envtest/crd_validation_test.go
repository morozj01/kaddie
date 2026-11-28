//go:build envtest

package envtest

import (
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/morozj01/kaddie/pkg/apis/v1alpha1"
)

// TestCRD_T8_SpecImmutable covers base T8: updating SREProposal.spec is
// denied for every caller. envtest's client is a cluster admin, so this is
// the "even as cluster-admin" case; the kind e2e repeats it with RBAC.
func TestCRD_T8_SpecImmutable(t *testing.T) {
	p := apiProposal(patchReq())
	expectAdmitted(t, p)

	mutations := map[string]func(*v1.SREProposal){
		"justification": func(p *v1.SREProposal) { p.Spec.Justification = "changed" },
		"request body":  func(p *v1.SREProposal) { p.Spec.API.Requests[0].Body = `{"spec":{"replicas":30}}` },
		"riskExplanation added": func(p *v1.SREProposal) {
			p.Spec.RiskExplanation = "now it is fine"
		},
		"sessionId": func(p *v1.SREProposal) { p.Spec.SessionID = "ses_other" },
	}
	for name, mutate := range mutations {
		t.Run("update "+name, func(t *testing.T) {
			cur := &v1.SREProposal{}
			if err := k8s.Get(ctx, client.ObjectKeyFromObject(p), cur); err != nil {
				t.Fatal(err)
			}
			mutate(cur)
			err := k8s.Update(ctx, cur)
			if err == nil || !apierrors.IsInvalid(err) || !strings.Contains(err.Error(), "spec is immutable") {
				t.Fatalf("update = %v, want Invalid: spec is immutable", err)
			}
		})
	}

	t.Run("merge patch of spec", func(t *testing.T) {
		patch := client.RawPatch(types.MergePatchType, []byte(`{"spec":{"justification":"patched"}}`))
		err := k8s.Patch(ctx, p.DeepCopy(), patch)
		if err == nil || !strings.Contains(err.Error(), "spec is immutable") {
			t.Fatalf("patch = %v, want spec is immutable", err)
		}
	})

	t.Run("status remains writable", func(t *testing.T) {
		cur := &v1.SREProposal{}
		if err := k8s.Get(ctx, client.ObjectKeyFromObject(p), cur); err != nil {
			t.Fatal(err)
		}
		cur.Status.Execution = &v1.ExecutionStatus{Phase: "Dismissed", ActedBy: "alice@example.com"}
		if err := k8s.Status().Update(ctx, cur); err != nil {
			t.Fatalf("status update: %v", err)
		}
	})

	t.Run("metadata remains writable", func(t *testing.T) {
		cur := &v1.SREProposal{}
		if err := k8s.Get(ctx, client.ObjectKeyFromObject(p), cur); err != nil {
			t.Fatal(err)
		}
		cur.Labels = map[string]string{"x": "y"}
		if err := k8s.Update(ctx, cur); err != nil {
			t.Fatalf("label update: %v", err)
		}
	})
}

// TestCRD_T43_SREConfigSingleton covers base T43's schema part: an SREConfig
// named anything but "default" is rejected, and a bare "default" starts with
// the safe defaults (mode Halted).
func TestCRD_T43_SREConfigSingleton(t *testing.T) {
	for _, name := range []string{"other", "default2", "kaddie"} {
		t.Run("name "+name, func(t *testing.T) {
			expectInvalid(t, &v1.SREConfig{ObjectMeta: metav1.ObjectMeta{Name: name}}, `supported values: "default"`)
		})
	}

	t.Run("generateName", func(t *testing.T) {
		expectInvalid(t, &v1.SREConfig{ObjectMeta: metav1.ObjectMeta{GenerateName: "default-"}}, `supported values: "default"`)
	})

	t.Run("bare default gets safe defaults", func(t *testing.T) {
		cfg := &v1.SREConfig{ObjectMeta: metav1.ObjectMeta{Name: v1.SREConfigName}}
		expectAdmitted(t, cfg)
		s := cfg.Spec
		if s.Mode != v1.ModeHalted {
			t.Errorf("mode = %q, want Halted", s.Mode)
		}
		if s.TriggersPaused {
			t.Error("triggersPaused defaulted to true")
		}
		if s.API.Enabled == nil || !*s.API.Enabled {
			t.Error("api.enabled did not default to true")
		}
		if s.Repositories == nil || len(s.Repositories) != 0 {
			t.Errorf("repositories = %#v, want []", s.Repositories)
		}
	})

	t.Run("repository URL with credentials", func(t *testing.T) {
		cfg := &v1.SREConfig{
			ObjectMeta: metav1.ObjectMeta{Name: v1.SREConfigName},
			Spec: v1.SREConfigSpec{Repositories: []v1.Repository{{
				Name: "r", URL: "https://user:token@git.example.com/r.git", TargetBranch: "main",
			}}},
		}
		expectInvalid(t, cfg, "credentials must not appear in the URL")
	})

	t.Run("repository defaults", func(t *testing.T) {
		cfg := &v1.SREConfig{
			ObjectMeta: metav1.ObjectMeta{Name: v1.SREConfigName},
			Spec: v1.SREConfigSpec{Repositories: []v1.Repository{{
				Name: "r", URL: "git@git.example.com:org/r.git", TargetBranch: "main",
				DeployPaths: []v1.DeployPath{{Name: "app", Path: "apps/app", Tool: "Kustomize"}},
			}}},
		}
		expectAdmitted(t, cfg)
		r := cfg.Spec.Repositories[0]
		if r.ProtectedPaths == nil || len(r.ProtectedPaths) != 0 {
			t.Errorf("protectedPaths = %#v, want []", r.ProtectedPaths)
		}
		if d := r.DeployPaths[0]; d.ExtraArgs == nil || len(d.ExtraArgs) != 0 {
			t.Errorf("deployPaths[0].extraArgs = %#v, want []", d.ExtraArgs)
		}
		if r.PR.BranchPrefix != "kaddie/" || r.PR.MinRequiredApprovals == nil || *r.PR.MinRequiredApprovals != 1 {
			t.Errorf("pr defaults = %+v", r.PR)
		}
	})
}
