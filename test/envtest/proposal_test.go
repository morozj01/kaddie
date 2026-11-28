//go:build envtest

package envtest

import (
	"encoding/json"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/pkg/proposal"
	"github.com/morozj01/kaddie/pkg/proposal/schema"
)

const deploymentJSON = `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"payments"},
"spec":{"replicas":2,"selector":{"matchLabels":{"app":"api"}},"template":{"metadata":{"labels":{"app":"api"}},
"spec":{"containers":[{"name":"api","image":"example.com/api:1"},{"name":"sidecar","image":"example.com/sidecar:1"}]}}}}`

// TestProposal_ExactOnRealDryRuns feeds EvaluateExact what a real 1.28 API
// server returns from server-side dry runs, which the conformance vectors
// only imitate: managedFields, resourceVersion, generation and defaulted
// fields must not raise findings, and a real protected change must.
func TestProposal_ExactOnRealDryRuns(t *testing.T) {
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "payments"}}
	if err := k8s.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatal(err)
	}
	deployments := dyn.Resource(k8sschema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}).Namespace("payments")
	pods := dyn.Resource(k8sschema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace("payments")
	var obj unstructured.Unstructured
	if err := json.Unmarshal([]byte(deploymentJSON), &obj.Object); err != nil {
		t.Fatal(err)
	}
	if _, err := deployments.Create(ctx, &obj, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatal(err)
	}
	live, err := deployments.Get(ctx, "api", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	liveJSON, err := live.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	template, err := json.Marshal(live.Object["spec"].(map[string]any)["template"])
	if err != nil {
		t.Fatal(err)
	}
	relabelled := `[{"op":"replace","path":"/spec/template","value":` + string(template) + `},` +
		`{"op":"add","path":"/spec/template/metadata/labels/tier","value":"web"}]`

	dryRun := []string{metav1.DryRunAll}
	tests := []struct {
		name      string
		req       v1alpha1.RequestFields
		wantExact []string
	}{
		{
			name: "a replicas change by strategic merge patch",
			req:  patch(types.StrategicMergePatchType, `{"spec":{"replicas":5}}`),
		},
		{
			name:      "an image change by strategic merge patch",
			req:       patch(types.StrategicMergePatchType, `{"spec":{"template":{"spec":{"containers":[{"name":"api","image":"example.com/api:2"}]}}}}`),
			wantExact: []string{"WorkloadField **.containers[*].image Request"},
		},
		{
			name: "the template replaced with itself plus a label, by JSON Patch",
			req:  patch(types.JSONPatchType, relabelled),
		},
		{
			name: "a pod create, whose dry run adds defaults the body doesn't set",
			req: v1alpha1.RequestFields{
				Verb: "create", APIVersion: "v1", Resource: "pods", Namespace: "payments",
				Body: `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"debug"},"spec":{"containers":[{"name":"c","image":"example.com/debug:1"}]}}`,
			},
			wantExact: []string{"WorkloadField **.containers[*].image Request"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := proposal.Observed{Request: 0}
			if tt.req.Verb == "create" {
				var body unstructured.Unstructured
				if err := json.Unmarshal([]byte(tt.req.Body), &body.Object); err != nil {
					t.Fatal(err)
				}
				out, err := pods.Create(ctx, &body, metav1.CreateOptions{DryRun: dryRun})
				if err != nil {
					t.Fatal(err)
				}
				obs.DryRun = mustJSON(t, out)
			} else {
				out, err := deployments.Patch(ctx, "api", types.PatchType(tt.req.PatchType), []byte(tt.req.Body), metav1.PatchOptions{DryRun: dryRun})
				if err != nil {
					t.Fatal(err)
				}
				obs.Live, obs.DryRun = liveJSON, mustJSON(t, out)
			}
			p := &v1alpha1.SREProposal{
				ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: systemNamespace},
				Spec: v1alpha1.SREProposalSpec{Type: v1alpha1.ProposalTypeAPI, Justification: "test",
					API: &v1alpha1.APIProposal{Requests: []v1alpha1.APIRequest{{RequestFields: tt.req}}}},
			}
			var got []string
			for _, f := range proposal.EvaluateExact(p, []proposal.Observed{obs}, schema.Vendored()) {
				got = append(got, string(f.Reason)+" "+f.Field+" "+string(f.Source))
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.wantExact) {
				t.Fatalf("exact findings = %q, want %q", got, tt.wantExact)
			}
		})
	}
}

func patch(pt types.PatchType, body string) v1alpha1.RequestFields {
	return v1alpha1.RequestFields{Verb: "patch", APIVersion: "apps/v1", Resource: "deployments",
		Namespace: "payments", Name: "api", PatchType: v1alpha1.PatchType(pt), Body: body}
}

func mustJSON(t *testing.T, u *unstructured.Unstructured) []byte {
	t.Helper()
	b, err := u.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	return b
}
