// Package crds checks the generated CRDs against the spec's YAML samples
// (T1.1): they must match exactly. Descriptions and integer formats are
// ignored, because the samples document fields in comments instead.
package crds

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"

	"github.com/morozj01/kaddie/test/internal/testutil"
)

func TestGeneratedCRDsMatchSamples(t *testing.T) {
	for _, plural := range []string{"sreproposals", "sreconfigs", "sretriggers"} {
		t.Run(plural, func(t *testing.T) {
			got := load(t, "charts", "kaddie", "crds", plural+".yaml")
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
