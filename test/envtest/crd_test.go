//go:build envtest

package envtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	v1 "github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/test/internal/testutil"
)

// TestCRD_SamplesAdmitted creates every instance in yaml-samples/crds/ as is
// (T1.1). Status, which the API server ignores on create, is then written
// through the status subresource so the status schema is exercised too.
func TestCRD_SamplesAdmitted(t *testing.T) {
	dir := testutil.Path("test", "fixtures", "yaml-samples", "crds")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no samples found")
	}
	for _, e := range entries {
		t.Run(e.Name(), func(t *testing.T) {
			obj := readSample(t, filepath.Join(dir, e.Name()))
			status, hasStatus, _ := unstructured.NestedFieldCopy(obj.Object, "status")
			unstructured.RemoveNestedField(obj.Object, "status")
			for _, f := range []string{"uid", "creationTimestamp", "resourceVersion"} {
				unstructured.RemoveNestedField(obj.Object, "metadata", f)
			}
			if obj.GetName() != "" && obj.GetGenerateName() != "" {
				obj.SetGenerateName("") // a fixed name wins; keep the test repeatable
			}

			if err := k8s.Create(ctx, obj); err != nil {
				t.Fatalf("create rejected: %v", err)
			}
			t.Cleanup(func() { _ = k8s.Delete(ctx, obj) })

			if hasStatus {
				if err := unstructured.SetNestedField(obj.Object, status, "status"); err != nil {
					t.Fatal(err)
				}
				if err := k8s.Status().Update(ctx, obj); err != nil {
					t.Fatalf("status update rejected: %v", err)
				}
			}
		})
	}
}

// TestCRD_T10_SynchronousRejections covers base T10: the API server rejects a
// bad verb including update, a missing name, an oversized body, more than 20
// requests and a block that doesn't match type. It also covers the other
// Api rules of base §6.1.
func TestCRD_T10_SynchronousRejections(t *testing.T) {
	ok := patchReq()
	cases := []struct {
		name    string
		mutate  func(p *v1.SREProposal)
		message string
	}{
		{"verb update", func(p *v1.SREProposal) { p.Spec.API.Requests[0].Verb = "update" }, "Unsupported value"},
		{"verb deletecollection", func(p *v1.SREProposal) { p.Spec.API.Requests[0].Verb = "deletecollection" }, "Unsupported value"},
		{"verb get", func(p *v1.SREProposal) { p.Spec.API.Requests[0].Verb = "get" }, "Unsupported value"},
		{"patch without name", func(p *v1.SREProposal) { p.Spec.API.Requests[0].Name = "" }, "name is required for patch and delete"},
		{"delete without name", func(p *v1.SREProposal) {
			p.Spec.API.Requests[0] = v1.APIRequest{RequestFields: v1.RequestFields{Verb: "delete", APIVersion: "v1", Resource: "pods", Namespace: "a"}}
		}, "name is required for patch and delete"},
		{"patch without patchType", func(p *v1.SREProposal) { p.Spec.API.Requests[0].PatchType = "" }, "patchType is required for patch"},
		{"unknown patchType", func(p *v1.SREProposal) { p.Spec.API.Requests[0].PatchType = "application/apply-patch+yaml" }, "Unsupported value"},
		{"patch without body", func(p *v1.SREProposal) { p.Spec.API.Requests[0].Body = "" }, "body is required for create and patch"},
		{"oversized body", func(p *v1.SREProposal) { p.Spec.API.Requests[0].Body = strings.Repeat("a", 65537) }, "Too long"},
		{"21 requests", func(p *v1.SREProposal) {
			p.Spec.API.Requests = nil
			for range 21 {
				p.Spec.API.Requests = append(p.Spec.API.Requests, ok)
			}
		}, "Too many"},
		{"0 requests", func(p *v1.SREProposal) { p.Spec.API.Requests = []v1.APIRequest{} }, "should have at least 1 items"},
		{"type Api with a git block", func(p *v1.SREProposal) { p.Spec.Git = gitSpec(createEdit("a.yaml", "x")) }, "exactly the block matching spec.type must be set"},
		{"type Api without an api block", func(p *v1.SREProposal) { p.Spec.API = nil }, "exactly the block matching spec.type must be set"},
		{"type Git with an api block", func(p *v1.SREProposal) { p.Spec.Type = v1.ProposalTypeGit }, "exactly the block matching spec.type must be set"},
		{"unknown type", func(p *v1.SREProposal) { p.Spec.Type = "Exec" }, "Unsupported value"},
		{"oversized justification", func(p *v1.SREProposal) { p.Spec.Justification = strings.Repeat("j", 4001) }, "Too long"},
		{"21 evidence entries", func(p *v1.SREProposal) {
			for range 21 {
				p.Spec.Evidence = append(p.Spec.Evidence, v1.EvidenceRef{Kind: "Pod", Name: "x"})
			}
		}, "Too many"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := apiProposal(ok)
			tc.mutate(p)
			expectInvalid(t, p, tc.message)
		})
	}

	t.Run("valid create, patch and delete are admitted", func(t *testing.T) {
		p := apiProposal(
			v1.APIRequest{RequestFields: v1.RequestFields{Verb: "create", APIVersion: "v1", Resource: "configmaps", Namespace: "a", Body: `{"metadata":{"generateName":"x-"}}`}},
			ok,
			v1.APIRequest{RequestFields: v1.RequestFields{Verb: "delete", APIVersion: "v1", Resource: "pods", Namespace: "a", Name: "p"}},
		)
		expectAdmitted(t, p)
	})
}

// TestCRD_GT7_GitSchemaRejections covers GT7's schema part and gitops
// §7.1.1: edit count, operation fields, path syntax, duplicate paths and a
// malformed baseSHA.
func TestCRD_GT7_GitSchemaRejections(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(g *v1.GitProposal)
		message string
	}{
		{"21 edits", func(g *v1.GitProposal) {
			g.Edits = nil
			for i := range 21 {
				g.Edits = append(g.Edits, createEdit("f"+strings.Repeat("x", i)+".yaml", "a"))
			}
		}, "Too many"},
		{"unknown operation", func(g *v1.GitProposal) { g.Edits[0].Operation = "Rename" }, "Unsupported value"},
		{"Edit without newText", func(g *v1.GitProposal) { g.Edits[0] = v1.GitEdit{Path: "a.yaml", Operation: "Edit", OldText: ptr("x")} }, "Edit requires oldText and newText"},
		{"Edit with content", func(g *v1.GitProposal) {
			g.Edits[0] = v1.GitEdit{Path: "a.yaml", Operation: "Edit", OldText: ptr("x"), NewText: ptr("y"), Content: ptr("z")}
		}, "an edit carries only the fields its operation uses"},
		{"Create without content", func(g *v1.GitProposal) { g.Edits[0] = v1.GitEdit{Path: "a.yaml", Operation: "Create"} }, "Create requires content"},
		{"Create with oldText", func(g *v1.GitProposal) { g.Edits[0].OldText = ptr("x") }, "an edit carries only the fields its operation uses"},
		{"Delete with newText", func(g *v1.GitProposal) {
			g.Edits[0] = v1.GitEdit{Path: "a.yaml", Operation: "Delete", NewText: ptr("y")}
		}, "an edit carries only the fields its operation uses"},
		{"oversized content", func(g *v1.GitProposal) { g.Edits[0].Content = ptr(strings.Repeat("c", 65537)) }, "Too long"},
		{"oversized newText", func(g *v1.GitProposal) {
			g.Edits[0] = v1.GitEdit{Path: "a.yaml", Operation: "Edit", OldText: ptr("x"), NewText: ptr(strings.Repeat("n", 65537))}
		}, "Too long"},
		{"leading slash", func(g *v1.GitProposal) { g.Edits[0].Path = "/etc/passwd" }, "path must be relative"},
		{"dot-dot segment", func(g *v1.GitProposal) { g.Edits[0].Path = "apps/../secrets.yaml" }, "path must be relative"},
		{"dot segment", func(g *v1.GitProposal) { g.Edits[0].Path = "./apps/a.yaml" }, "path must be relative"},
		{"empty segment", func(g *v1.GitProposal) { g.Edits[0].Path = "apps//a.yaml" }, "path must be relative"},
		{"trailing slash", func(g *v1.GitProposal) { g.Edits[0].Path = "apps/" }, "path must be relative"},
		{"backslash", func(g *v1.GitProposal) { g.Edits[0].Path = `apps\a.yaml` }, "path must be relative"},
		{"control character", func(g *v1.GitProposal) { g.Edits[0].Path = "apps/a\x01.yaml" }, "path must be relative"},
		{"duplicate paths", func(g *v1.GitProposal) { g.Edits = append(g.Edits, createEdit(g.Edits[0].Path, "other")) }, "no two edits may target the same path"},
		{"short baseSHA", func(g *v1.GitProposal) { g.BaseSHA = "3f9c2e1" }, "should match"},
		{"uppercase baseSHA", func(g *v1.GitProposal) { g.BaseSHA = strings.ToUpper(sha1) }, "should match"},
		{"41-character baseSHA", func(g *v1.GitProposal) { g.BaseSHA = sha1 + "a" }, "should match"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := gitProposal(createEdit("apps/a.yaml", "kind: ConfigMap\n"))
			tc.mutate(p.Spec.Git)
			expectInvalid(t, p, tc.message)
		})
	}

	t.Run("valid edits are admitted", func(t *testing.T) {
		expectAdmitted(t, gitProposal(
			createEdit("apps/new.yaml", "kind: ConfigMap\n"),
			v1.GitEdit{Path: "apps/values.yaml", Operation: "Edit", OldText: ptr("replicas: 3"), NewText: ptr("")},
			v1.GitEdit{Path: "apps/old.yaml", Operation: "Delete"},
		))
	})
	t.Run("SHA-256 baseSHA is admitted", func(t *testing.T) {
		p := gitProposal(createEdit("a.yaml", "x"))
		p.Spec.Git.BaseSHA = sha1 + strings.Repeat("0", 24)
		expectAdmitted(t, p)
	})
}

// TestCRD_TriggerRules covers harness §7.2's CEL rules and defaults.
func TestCRD_TriggerRules(t *testing.T) {
	watch := &v1.WatchSource{APIVersion: "apps/v1", Resource: "deployments", When: "true"}
	sched := &v1.ScheduleSource{Cron: "0 8 * * *"}

	t.Run("no source", func(t *testing.T) {
		expectInvalid(t, trigger(func(s *v1.SRETriggerSpec) {}), "exactly one of watch, schedule or webhook must be set")
	})
	t.Run("two sources", func(t *testing.T) {
		expectInvalid(t, trigger(func(s *v1.SRETriggerSpec) { s.Watch, s.Schedule = watch, sched }), "exactly one of watch, schedule or webhook must be set")
	})
	t.Run("ProposalSession on a schedule", func(t *testing.T) {
		expectInvalid(t, trigger(func(s *v1.SRETriggerSpec) { s.Schedule, s.Session.Resume = sched, "ProposalSession" }), "ProposalSession resume requires a watch on sreproposals")
	})
	t.Run("ProposalSession on a deployment watch", func(t *testing.T) {
		expectInvalid(t, trigger(func(s *v1.SRETriggerSpec) { s.Watch, s.Session.Resume = watch, "ProposalSession" }), "ProposalSession resume requires a watch on sreproposals")
	})
	t.Run("bad webhook path", func(t *testing.T) {
		expectInvalid(t, trigger(func(s *v1.SRETriggerSpec) {
			s.Webhook = &v1.WebhookSource{Path: "Bad/Path", Auth: v1.WebhookAuth{Type: "Bearer", SecretName: "s"}, When: "true"}
		}), "should match")
	})
	t.Run("defaults", func(t *testing.T) {
		tr := trigger(func(s *v1.SRETriggerSpec) {
			s.Webhook = &v1.WebhookSource{Path: "am", Auth: v1.WebhookAuth{Type: "HMAC", SecretName: "s"}, When: "true"}
		})
		expectAdmitted(t, tr)
		if tr.Spec.Mode != v1.TriggerModeShadow || tr.Spec.Session.Resume != "None" || tr.Spec.Webhook.Subject != "payload.name" || tr.Spec.Webhook.ExpireAfter != "24h" {
			t.Fatalf("defaults not applied: mode=%q resume=%q subject=%q expireAfter=%q",
				tr.Spec.Mode, tr.Spec.Session.Resume, tr.Spec.Webhook.Subject, tr.Spec.Webhook.ExpireAfter)
		}
	})
}

// --- helpers ---------------------------------------------------------------

const sha1 = "3f9c2e1b7a4d6c8e0f2a4b6c8d0e2f4a6b8c0d2e"

func ptr(s string) *string { return &s }

func patchReq() v1.APIRequest {
	return v1.APIRequest{RequestFields: v1.RequestFields{
		Verb: "patch", APIVersion: "apps/v1", Resource: "deployments", Namespace: "payments", Name: "api",
		PatchType: "application/merge-patch+json", Body: `{"spec":{"replicas":3}}`,
	}}
}

func apiProposal(reqs ...v1.APIRequest) *v1.SREProposal {
	return &v1.SREProposal{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "proposal-", Namespace: systemNamespace},
		Spec: v1.SREProposalSpec{
			Type:          v1.ProposalTypeAPI,
			API:           &v1.APIProposal{Requests: reqs},
			Justification: "test",
		},
	}
}

func createEdit(path, content string) v1.GitEdit {
	return v1.GitEdit{Path: path, Operation: "Create", Content: &content}
}

func gitSpec(edits ...v1.GitEdit) *v1.GitProposal {
	return &v1.GitProposal{Repository: "platform-config", BaseSHA: sha1, Edits: edits}
}

func gitProposal(edits ...v1.GitEdit) *v1.SREProposal {
	return &v1.SREProposal{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "proposal-", Namespace: systemNamespace},
		Spec:       v1.SREProposalSpec{Type: v1.ProposalTypeGit, Git: gitSpec(edits...), Justification: "test"},
	}
}

func trigger(mutate func(*v1.SRETriggerSpec)) *v1.SRETrigger {
	tr := &v1.SRETrigger{ObjectMeta: metav1.ObjectMeta{GenerateName: "trigger-"}}
	mutate(&tr.Spec)
	return tr
}

func readSample(t *testing.T, path string) *unstructured.Unstructured {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(raw, &obj.Object); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return obj
}

func expectInvalid(t *testing.T, obj client.Object, message string) {
	t.Helper()
	err := k8s.Create(ctx, obj)
	if err == nil {
		_ = k8s.Delete(ctx, obj)
		t.Fatalf("create admitted, want rejection containing %q", message)
	}
	if !apierrors.IsInvalid(err) {
		t.Fatalf("create failed with %v, want an Invalid error", err)
	}
	if !strings.Contains(err.Error(), message) {
		t.Fatalf("rejection %q does not mention %q", err, message)
	}
}

func expectAdmitted(t *testing.T, obj client.Object) {
	t.Helper()
	if err := k8s.Create(ctx, obj); err != nil {
		t.Fatalf("create rejected: %v", err)
	}
	t.Cleanup(func() { _ = k8s.Delete(ctx, obj) })
}
