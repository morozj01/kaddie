// Package crds checks the generated CRDs against the spec's YAML samples.
//
// T1.1: "Generated CRDs are diffed against yaml-samples/crd-definitions/. The
// samples lack ... the 256 KiB spec limit; those additions are the only
// allowed differences." The samples now carry gitops §7.1.1's rules, the
// 256 KiB rule proved too costly for CEL, and a bound on SREConfig
// repositories had to be added (allowedSchemaAdditions). Descriptions and
// integer formats are ignored: the samples document fields in comments.
package crds

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"

	"github.com/morozj01/kaddie/test/internal/testutil"
)

// allowedSchemaAdditions are schema keywords present in the generated CRDs
// and absent from the samples on purpose, by CRD and property path.
var allowedSchemaAdditions = map[string]map[string][]string{
	// A 1.28 API server rejects the sample as written: the url rule's
	// estimated CEL cost exceeds the budget unless the list is bounded.
	"sreconfigs": {"spec.repositories": {"maxItems"}},
}

// allowedAdditions are x-kubernetes-validations messages present in the
// generated CRDs and absent from the samples on purpose. None today: the
// 256 KiB whole-spec limit the plan allowed is too costly for CEL on 1.28
// (see SREProposalSpec).
var allowedAdditions = map[string]bool{}

func TestGeneratedCRDsMatchSamples(t *testing.T) {
	for _, plural := range []string{"sreproposals", "sreconfigs", "sretriggers"} {
		t.Run(plural, func(t *testing.T) {
			got := load(t, "charts", "kaddie", "crds", plural+".yaml")
			dropAllowed(got, allowedSchemaAdditions[plural])
			want := load(t, "test", "fixtures", "yaml-samples", "crd-definitions", "crd-"+plural+".yaml")
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("generated CRD differs from the sample (-sample +generated):\n%s", diff)
			}
		})
	}
}

// load reads a CRD and returns a normalised, comparable form of its spec.
func load(t *testing.T, path ...string) any {
	t.Helper()
	var crd apiextensionsv1.CustomResourceDefinition
	if err := yaml.UnmarshalStrict(testutil.MustRead(path...), &crd); err != nil {
		t.Fatalf("%v: %v", path, err)
	}
	raw, err := json.Marshal(crd.Spec)
	if err != nil {
		t.Fatal(err)
	}
	var m any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return normalise(m)
}

// dropAllowed removes allowed keywords from a normalised generated CRD.
func dropAllowed(crd any, allowed map[string][]string) {
	for path, keys := range allowed {
		for _, v := range crd.(map[string]any)["versions"].([]any) {
			node := v.(map[string]any)["schema"].(map[string]any)["openAPIV3Schema"].(map[string]any)
			for _, p := range strings.Split(path, ".") {
				node = node["properties"].(map[string]any)[p].(map[string]any)
			}
			for _, k := range keys {
				delete(node, k)
			}
		}
	}
}

func normalise(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			switch {
			case k == "description":
				continue
			case k == "format" && (val == "int32" || val == "int64"):
				continue
			case k == "required":
				// required is a set; controller-gen sorts it, the samples don't.
				req := append([]any(nil), val.([]any)...)
				sort.Slice(req, func(i, j int) bool { return req[i].(string) < req[j].(string) })
				out[k] = req
				continue
			case k == "x-kubernetes-validations":
				if rules := filterRules(val); len(rules) > 0 {
					out[k] = rules
				}
				continue
			}
			out[k] = normalise(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = normalise(x[i])
		}
		return out
	default:
		return v
	}
}

func filterRules(v any) []any {
	var kept []any
	for _, r := range v.([]any) {
		if m, ok := r.(map[string]any); ok && allowedAdditions[m["message"].(string)] {
			continue
		}
		kept = append(kept, normalise(r))
	}
	return kept
}
