package chart

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestT53_VersionGate: rendering for 1.27 fails with a message naming the
// requirement and renders nothing; 1.28 and later render.
func TestT53_VersionGate(t *testing.T) {
	for _, v := range []string{"1.27.0", "1.27.16", "1.20.0"} {
		out, err := helmTemplate(chartDir, "--kube-version", v)
		if err == nil {
			t.Fatalf("kube %s: rendered %d bytes, want a failure", v, len(out))
		}
		if !strings.Contains(err.Error(), "1.28") {
			t.Errorf("kube %s: error %q doesn't name the 1.28 requirement", v, err)
		}
	}
	for _, v := range []string{"1.28.0", "1.28.15", "1.31.4", "1.37.0"} {
		if objs := render(t, chartDir, "--kube-version", v); len(objs) == 0 {
			t.Errorf("kube %s rendered nothing", v)
		}
	}
}

// TestT43_ChartCreatesNoSREConfig: the chart never renders SREConfig, so a
// fresh install is Halted and Helm never owns the object.
func TestT43_ChartCreatesNoSREConfig(t *testing.T) {
	gitopsModes(t, func(t *testing.T, _ bool, args []string) {
		for _, o := range render(t, chartDir, args...) {
			if o.GetKind() == "SREConfig" || o.GetKind() == "CustomResourceDefinition" {
				t.Errorf("chart renders %s/%s; SREConfig must not be created and CRDs belong in crds/", o.GetKind(), o.GetName())
			}
		}
	})
}

func TestNamespaceAndQuotaMatchSamples(t *testing.T) {
	objs := render(t, chartDir)

	ns := find(objs, "Namespace", "kaddie-system")
	if ns == nil {
		t.Fatal("no kaddie-system Namespace rendered")
	}
	for k, v := range sample(t, "namespace", "namespace-kaddie-system.yaml").GetLabels() {
		if ns.GetLabels()[k] != v {
			t.Errorf("namespace label %s = %q, want %q", k, ns.GetLabels()[k], v)
		}
	}

	q := find(objs, "ResourceQuota", "kaddie-proposals")
	if q == nil {
		t.Fatal("no kaddie-proposals ResourceQuota rendered")
	}
	want := sample(t, "namespace", "resourcequota-kaddie-proposals.yaml")
	if q.GetNamespace() != want.GetNamespace() {
		t.Errorf("quota namespace = %q", q.GetNamespace())
	}
	got, _, _ := unstructured.NestedStringMap(q.Object, "spec", "hard")
	exp, _, _ := unstructured.NestedStringMap(want.Object, "spec", "hard")
	if len(got) != len(exp) || got["count/sreproposals.kaddie.org"] != exp["count/sreproposals.kaddie.org"] {
		t.Errorf("quota hard = %v, want %v", got, exp)
	}

	q = find(render(t, chartDir, "--set", "proposals.quota=25"), "ResourceQuota", "kaddie-proposals")
	if h, _, _ := unstructured.NestedString(q.Object, "spec", "hard", "count/sreproposals.kaddie.org"); h != "25" {
		t.Errorf("proposals.quota=25 rendered %q", h)
	}
}

func TestServiceAccountsMatchSamples(t *testing.T) {
	gitopsModes(t, func(t *testing.T, gitops bool, args []string) {
		want := []string{"kaddie-agent", "kaddie-sessions", "kaddie-tools", "kaddie-ui"}
		if gitops {
			want = append(want, "kaddie-git-writer", "kaddie-renderer")
		}
		objs := render(t, chartDir, args...)
		sas := ofKind(objs, "ServiceAccount")
		if len(sas) != len(want) {
			t.Errorf("rendered %d ServiceAccounts, want %d", len(sas), len(want))
		}
		for _, name := range want {
			got := find(objs, "ServiceAccount", name)
			if got == nil {
				t.Errorf("ServiceAccount %s not rendered", name)
				continue
			}
			exp := sample(t, "serviceaccounts", "serviceaccount-"+name+".yaml")
			if got.GetNamespace() != exp.GetNamespace() {
				t.Errorf("%s namespace = %q, want %q", name, got.GetNamespace(), exp.GetNamespace())
			}
			g, gok, _ := unstructured.NestedBool(got.Object, "automountServiceAccountToken")
			e, eok, _ := unstructured.NestedBool(exp.Object, "automountServiceAccountToken")
			if gok != eok || g != e {
				t.Errorf("%s automountServiceAccountToken = %v (set %v), want %v (set %v)", name, g, gok, e, eok)
			}
		}
	})
}
