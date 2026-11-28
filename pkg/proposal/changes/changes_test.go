package changes_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/pkg/proposal/changes"
	"github.com/morozj01/kaddie/pkg/proposal/fieldpath"
	"github.com/morozj01/kaddie/pkg/proposal/schema"
)

const (
	create v1alpha1.RequestVerb = "create"
	patch  v1alpha1.RequestVerb = "patch"
	del    v1alpha1.RequestVerb = "delete"

	jsonPatch      v1alpha1.PatchType = "application/json-patch+json"
	mergePatch     v1alpha1.PatchType = "application/merge-patch+json"
	strategicPatch v1alpha1.PatchType = "application/strategic-merge-patch+json"
)

var (
	deployments = schema.Resource{Group: "apps", Version: "v1", Resource: "deployments"}
	pods        = schema.Resource{Version: "v1", Resource: "pods"}
	cronjobs    = schema.Resource{Group: "batch", Version: "v1", Resource: "cronjobs"}

	deploymentRoot = schema.Vendored().Root(deployments)
)

// render writes a change as its path, with * after a Subtree change and <root>
// for the root.
func render(cs []fieldpath.Change) []string {
	out := []string{}
	for _, c := range cs {
		s := fieldpath.Format(c.Path)
		if s == "" {
			s = "<root>"
		}
		if c.Subtree {
			s += "*"
		}
		out = append(out, s)
	}
	return out
}

// sorted renders cs in sorted order, for tests that don't care about order.
func sorted(cs []fieldpath.Change) []string {
	out := render(cs)
	slices.Sort(out)
	return out
}

func want(paths ...string) []string {
	out := slices.Clone(paths)
	slices.Sort(out)
	if out == nil {
		out = []string{}
	}
	return out
}

func checkSorted(t *testing.T, got []fieldpath.Change, wantPaths ...string) {
	t.Helper()
	if diff := cmp.Diff(want(wantPaths...), sorted(got)); diff != "" {
		t.Errorf("changes (-want +got):\n%s", diff)
	}
}

const deployment = `{
  "apiVersion": "apps/v1",
  "kind": "Deployment",
  "metadata": {"name": "web", "labels": {"app": "web"}},
  "spec": {
    "replicas": 2,
    "selector": {"matchLabels": {"app": "web"}},
    "template": {
      "metadata": {"labels": {"app": "web"}},
      "spec": {
        "serviceAccountName": "web-sa",
        "containers": [
          {"name": "app", "image": "nginx:1.25", "args": ["-g", "daemon off;"], "env": []}
        ]
      }
    }
  }
}`

func TestCreateChangesEveryField(t *testing.T) {
	got := changes.Static(create, "", []byte(deployment), deploymentRoot)
	checkSorted(t, got,
		"apiVersion",
		"kind",
		"metadata.name",
		"metadata.labels.app",
		"spec.replicas",
		"spec.selector.matchLabels.app",
		"spec.template.metadata.labels.app",
		"spec.template.spec.serviceAccountName",
		"spec.template.spec.containers[0].name",
		"spec.template.spec.containers[0].image",
		"spec.template.spec.containers[0].args[0]",
		"spec.template.spec.containers[0].args[1]",
		"spec.template.spec.containers[0].env", // an empty list is a leaf
	)
	for _, c := range got {
		if c.Subtree {
			t.Errorf("create change %s is a Subtree; every create change is a leaf", fieldpath.Format(c.Path))
		}
	}
}

func TestCreateListElementsAreElemSegments(t *testing.T) {
	got := changes.Static(create, "", []byte(`{"spec":{"containers":[{"image":"a"},{"image":"b"}]}}`), schema.Node{})
	want := []fieldpath.Change{
		{Path: []fieldpath.Segment{{Key: "spec"}, {Key: "containers"}, {Key: "0", Elem: true}, {Key: "image"}}},
		{Path: []fieldpath.Segment{{Key: "spec"}, {Key: "containers"}, {Key: "1", Elem: true}, {Key: "image"}}},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("changes (-want +got):\n%s", diff)
	}
}

func TestCreateEmptyContainers(t *testing.T) {
	got := changes.Static(create, "", []byte(`{"metadata":{"labels":{}},"spec":{"volumes":[],"x":null}}`), schema.Node{})
	checkSorted(t, got, "metadata.labels", "spec.volumes", "spec.x")
}

func TestCreateNeedsAnObject(t *testing.T) {
	for _, body := range []string{`[]`, `"x"`, `null`, `{`, ``, `{} {}`, string(rune(0xFEFF)) + "{}"} {
		got := changes.Static(create, "", []byte(body), schema.Node{})
		checkSorted(t, got, "<root>*")
	}
}

func TestDeleteChangesNothing(t *testing.T) {
	for _, body := range []string{``, `{}`, `{"spec":{"replicas":1}}`, `garbage`} {
		if got := changes.Static(del, "", []byte(body), deploymentRoot); len(got) != 0 {
			t.Errorf("delete with body %q changed %v", body, render(got))
		}
	}
}

func TestStatic_UnknownVerbOrPatchTypeChangesRoot(t *testing.T) {
	checkSorted(t, changes.Static("update", "", []byte(`{}`), schema.Node{}), "<root>*")
	checkSorted(t, changes.Static(patch, "", []byte(`{}`), schema.Node{}), "<root>*")
	checkSorted(t, changes.Static(patch, "text/plain", []byte(`{}`), schema.Node{}), "<root>*")
}

func TestJSONPatch(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{"replace is a subtree at the path", `[{"op":"replace","path":"/spec/replicas","value":3}]`, []string{"spec.replicas*"}},
		{"add reaches into a list element", `[{"op":"add","path":"/spec/template/spec/containers/0/image","value":"x"}]`,
			[]string{"spec.template.spec.containers[0].image*"}},
		{"add appends with -", `[{"op":"add","path":"/spec/template/spec/containers/-","value":{"name":"x"}}]`,
			[]string{"spec.template.spec.containers[-]*"}},
		{"remove", `[{"op":"remove","path":"/spec/template/spec/volumes"}]`, []string{"spec.template.spec.volumes*"}},
		{"move changes the path and from", `[{"op":"move","from":"/spec/a","path":"/spec/b"}]`, []string{"spec.a*", "spec.b*"}},
		{"copy changes the path and from", `[{"op":"copy","from":"/spec/a","path":"/spec/b"}]`, []string{"spec.a*", "spec.b*"}},
		{"test changes nothing", `[{"op":"test","path":"/spec/replicas","value":1}]`, nil},
		{"test is skipped among real operations", `[{"op":"test","path":"/spec/x","value":1},{"op":"remove","path":"/spec/y"}]`, []string{"spec.y*"}},
		{"~1 is a slash", `[{"op":"add","path":"/metadata/annotations/example.com~1owner","value":"x"}]`,
			[]string{`metadata.annotations["example.com/owner"]*`}},
		{"~0 is a tilde and ~01 is ~1", `[{"op":"add","path":"/metadata/annotations/a~0b~01c","value":"x"}]`,
			[]string{"metadata.annotations.a~b~1c*"}},
		{"the empty pointer is the root", `[{"op":"replace","path":"","value":{}}]`, []string{"<root>*"}},
		{"an empty list changes nothing", `[]`, nil},
		{"duplicates collapse", `[{"op":"add","path":"/a","value":1},{"op":"replace","path":"/a","value":2}]`, []string{"a*"}},
		{"a non-array body changes the root", `{"op":"replace","path":"/spec/replicas","value":3}`, []string{"<root>*"}},
		{"invalid JSON changes the root", `[{`, []string{"<root>*"}},
		{"an unknown op changes the root", `[{"op":"frobnicate","path":"/spec"}]`, []string{"<root>*"}},
		{"a missing op changes the root", `[{"path":"/spec"}]`, []string{"<root>*"}},
		{"a missing path changes the root", `[{"op":"add","value":1}]`, []string{"<root>*"}},
		{"a pointer without a leading slash changes the root", `[{"op":"add","path":"spec/x","value":1}]`, []string{"<root>*"}},
		{"move without from changes the root", `[{"op":"move","path":"/spec/b"}]`, []string{"<root>*"}},
		{"a non-object operation changes the root", `[1]`, []string{"<root>*"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkSorted(t, changes.Static(patch, jsonPatch, []byte(tt.body), deploymentRoot), tt.want...)
		})
	}
}

func TestJSONPatchSegments(t *testing.T) {
	body := `[{"op":"replace","path":"/spec/template/spec/containers/0/image","value":"x"},
	          {"op":"add","path":"/spec/template/spec/containers/-","value":{}},
	          {"op":"replace","path":"/spec/replicas","value":1}]`
	got := changes.Static(patch, jsonPatch, []byte(body), deploymentRoot)
	want := []fieldpath.Change{
		{Subtree: true, Path: []fieldpath.Segment{{Key: "spec"}, {Key: "template"}, {Key: "spec"}, {Key: "containers"}, {Key: "0", Elem: true}, {Key: "image"}}},
		{Subtree: true, Path: []fieldpath.Segment{{Key: "spec"}, {Key: "template"}, {Key: "spec"}, {Key: "containers"}, {Key: "-", Elem: true}}},
		{Subtree: true, Path: []fieldpath.Segment{{Key: "spec"}, {Key: "replicas"}}},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("changes (-want +got):\n%s", diff)
	}
}

func TestMergePatch(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{"scalars are leaves", `{"spec":{"replicas":3,"paused":true,"x":"y"}}`, []string{"spec.replicas", "spec.paused", "spec.x"}},
		{"nested objects are walked", `{"spec":{"template":{"spec":{"serviceAccountName":"x"}}}}`, []string{"spec.template.spec.serviceAccountName"}},
		{"a list value is a subtree", `{"spec":{"template":{"spec":{"containers":[{"name":"a","image":"b"}]}}}}`,
			[]string{"spec.template.spec.containers*"}},
		{"an empty list value is a subtree", `{"spec":{"template":{"spec":{"volumes":[]}}}}`, []string{"spec.template.spec.volumes*"}},
		{"a null value is a subtree", `{"spec":{"template":{"spec":{"nodeName":null}}}}`, []string{"spec.template.spec.nodeName*"}},
		{"an empty object is a leaf", `{"metadata":{"labels":{}}}`, []string{"metadata.labels"}},
		{"dollar keys are ordinary keys", `{"spec":{"$patch":"delete"}}`, []string{"spec.$patch"}},
		{"a top-level array changes the root", `[{"spec":1}]`, []string{"<root>*"}},
		{"a top-level scalar changes the root", `5`, []string{"<root>*"}},
		{"a top-level null changes the root", `null`, []string{"<root>*"}},
		{"invalid JSON changes the root", `{"spec":`, []string{"<root>*"}},
		{"numeric keys stay map keys", `{"data":{"0":"x"}}`, []string{"data.0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkSorted(t, changes.Static(patch, mergePatch, []byte(tt.body), deploymentRoot), tt.want...)
		})
	}
}

func TestMergePatchKeysAreNotElements(t *testing.T) {
	got := changes.Static(patch, mergePatch, []byte(`{"data":{"0":"x"}}`), schema.Node{})
	want := []fieldpath.Change{{Path: []fieldpath.Segment{{Key: "data"}, {Key: "0"}}}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("changes (-want +got):\n%s", diff)
	}
}

const containersPrefix = "spec.template.spec.containers"

func TestStrategicMergePatch(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			"a container matched by name: resources change, name and image don't",
			`{"spec":{"template":{"spec":{"containers":[{"name":"app","resources":{"limits":{"cpu":"1"},"requests":{"cpu":"500m"}}}]}}}}`,
			[]string{containersPrefix + "[*].resources.limits.cpu", containersPrefix + "[*].resources.requests.cpu"},
		},
		{
			"a container matched by name: image changes",
			`{"spec":{"template":{"spec":{"containers":[{"name":"app","image":"nginx:1.26"}]}}}}`,
			[]string{containersPrefix + "[*].image"},
		},
		{
			"replicas is a leaf",
			`{"spec":{"replicas":3}}`,
			[]string{"spec.replicas"},
		},
		{
			"each element is walked",
			`{"spec":{"template":{"spec":{"containers":[{"name":"a","image":"x"},{"name":"b","resources":{"limits":{"cpu":"1"}}}]}}}}`,
			[]string{containersPrefix + "[*].image", containersPrefix + "[*].resources.limits.cpu"},
		},
		{
			"a nested keyed list: env by name",
			`{"spec":{"template":{"spec":{"containers":[{"name":"app","env":[{"name":"A","value":"1"}]}]}}}}`,
			[]string{containersPrefix + "[*].env[*].value"},
		},
		{
			"a container that is only a name still appears",
			`{"spec":{"template":{"spec":{"containers":[{"name":"sidecar"}]}}}}`,
			[]string{containersPrefix + "[*]"},
		},
		{
			"an env var that is only a name still appears",
			`{"spec":{"template":{"spec":{"containers":[{"name":"app","env":[{"name":"FOO"}]}]}}}}`,
			[]string{containersPrefix + "[*].env[*]"},
		},
		{
			"an empty keyed list is a leaf",
			`{"spec":{"template":{"spec":{"containers":[]}}}}`,
			[]string{containersPrefix},
		},
		{
			"args, an atomic list, is a subtree",
			`{"spec":{"template":{"spec":{"containers":[{"name":"app","args":["-x"]}]}}}}`,
			[]string{containersPrefix + "[*].args*"},
		},
		{
			"a null value is a subtree",
			`{"spec":{"template":{"spec":{"serviceAccountName":null}}}}`,
			[]string{"spec.template.spec.serviceAccountName*"},
		},
		{
			"a primitive list with merge adds leaf elements",
			`{"metadata":{"finalizers":["example.com/x","example.com/y"]}}`,
			[]string{"metadata.finalizers[*]"},
		},
		{
			"$patch: delete on an element is a subtree of the element",
			`{"spec":{"template":{"spec":{"containers":[{"name":"sidecar","$patch":"delete"}]}}}}`,
			[]string{containersPrefix + "[*]*"},
		},
		{
			"$patch: replace on spec.template.spec is a subtree there",
			`{"spec":{"template":{"spec":{"$patch":"replace","containers":[{"name":"app","image":"x"}]}}}}`,
			[]string{"spec.template.spec*"},
		},
		{
			"$patch: replace as a list element replaces the list",
			`{"spec":{"template":{"spec":{"containers":[{"$patch":"replace"},{"name":"app","image":"x"}]}}}}`,
			[]string{containersPrefix + "*"},
		},
		{
			"$patch: merge is conservative too",
			`{"spec":{"template":{"spec":{"$patch":"merge","serviceAccountName":"x"}}}}`,
			[]string{"spec.template.spec*"},
		},
		{
			"$retainKeys makes the object a subtree",
			`{"spec":{"strategy":{"$retainKeys":["type"],"type":"Recreate"}}}`,
			[]string{"spec.strategy*"},
		},
		{
			"$setElementOrder/containers makes containers a subtree",
			`{"spec":{"template":{"spec":{"$setElementOrder/containers":[{"name":"app"}],"containers":[{"name":"app","resources":{"limits":{"cpu":"1"}}}]}}}}`,
			[]string{containersPrefix + "*", containersPrefix + "[*].resources.limits.cpu"},
		},
		{
			"$deleteFromPrimitiveList/args makes args a subtree",
			`{"spec":{"template":{"spec":{"containers":[{"name":"app","$deleteFromPrimitiveList/args":["-x"]}]}}}}`,
			[]string{containersPrefix + "[*].args*"},
		},
		{
			"an unknown directive makes the object a subtree",
			`{"spec":{"template":{"spec":{"$somethingNew":true}}}}`,
			[]string{"spec.template.spec*"},
		},
		{
			"a directive with no field makes the object a subtree",
			`{"spec":{"template":{"spec":{"$setElementOrder/":[]}}}}`,
			[]string{"spec.template.spec*"},
		},
		{
			"a top-level directive changes the root",
			`{"$patch":"replace","spec":{}}`,
			[]string{"<root>*"},
		},
		{
			"a non-object element makes the list a subtree",
			`{"spec":{"template":{"spec":{"containers":["app"]}}}}`,
			[]string{containersPrefix + "*"},
		},
		{
			"a top-level array changes the root",
			`[]`,
			[]string{"<root>*"},
		},
		{
			"invalid JSON changes the root",
			`{`,
			[]string{"<root>*"},
		},
		{
			"an empty patch creates an empty object leaf at the root",
			`{}`,
			[]string{"<root>"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkSorted(t, changes.Static(patch, strategicPatch, []byte(tt.body), deploymentRoot), tt.want...)
		})
	}
}

func TestStrategicMergePatchOnOtherKinds(t *testing.T) {
	t.Run("pods", func(t *testing.T) {
		body := `{"spec":{"containers":[{"name":"app","image":"x"}],"initContainers":[{"name":"i","command":["a"]}]}}`
		checkSorted(t, changes.Static(patch, strategicPatch, []byte(body), schema.Vendored().Root(pods)),
			"spec.containers[*].image", "spec.initContainers[*].command*")
	})
	t.Run("cronjobs", func(t *testing.T) {
		body := `{"spec":{"jobTemplate":{"spec":{"template":{"spec":{"containers":[{"name":"c","image":"x"}]}}}}}}`
		checkSorted(t, changes.Static(patch, strategicPatch, []byte(body), schema.Vendored().Root(cronjobs)),
			"spec.jobTemplate.spec.template.spec.containers[*].image")
	})
}

func TestStrategicMergePatchListElementPathHasNoIndex(t *testing.T) {
	body := `{"spec":{"template":{"spec":{"containers":[{"name":"app","image":"x"}]}}}}`
	got := changes.Static(patch, strategicPatch, []byte(body), deploymentRoot)
	want := []fieldpath.Change{{Path: []fieldpath.Segment{
		{Key: "spec"}, {Key: "template"}, {Key: "spec"}, {Key: "containers"}, {Elem: true}, {Key: "image"},
	}}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("changes (-want +got):\n%s", diff)
	}
}

func TestStrategicMergePatchUnderUnknownSchema(t *testing.T) {
	body := `{"spec":{"template":{"spec":{"containers":[{"name":"app","resources":{"limits":{"cpu":"1"}}}],"serviceAccountName":"x"}}}}`
	got := changes.Static(patch, strategicPatch, []byte(body), schema.Node{})
	checkSorted(t, got, "spec.template.spec.containers*", "spec.template.spec.serviceAccountName")

	// The same lists a known schema merges are replaced when it is unknown,
	// including primitive lists with a merge strategy.
	got = changes.Static(patch, strategicPatch, []byte(`{"metadata":{"finalizers":["x"]}}`), schema.Node{})
	checkSorted(t, got, "metadata.finalizers*")
}

func TestStatic_DeterministicAndWithoutDuplicates(t *testing.T) {
	body := []byte(deployment)
	first := render(changes.Static(create, "", body, deploymentRoot))
	for range 20 {
		if diff := cmp.Diff(first, render(changes.Static(create, "", body, deploymentRoot))); diff != "" {
			t.Fatalf("Static is not deterministic (-first +later):\n%s", diff)
		}
	}
	// Keys come out sorted, list elements in order.
	wantPrefix := []string{"apiVersion", "kind", "metadata.labels.app", "metadata.name", "spec.replicas"}
	if diff := cmp.Diff(wantPrefix, first[:len(wantPrefix)]); diff != "" {
		t.Errorf("order (-want +got):\n%s", diff)
	}

	dup := []byte(`{"spec":{"containers":[{"name":"a","image":"x"},{"name":"b","image":"y"}]}}`)
	got := render(changes.Static(patch, strategicPatch, dup, schema.Vendored().Root(pods)))
	if diff := cmp.Diff([]string{"spec.containers[*].image"}, got); diff != "" {
		t.Errorf("two containers' images are one path (-want +got):\n%s", diff)
	}
}

func TestStatic_ChangesDoNotShareStorage(t *testing.T) {
	body := `{"spec":{"template":{"spec":{"containers":[{"name":"a","image":"x","args":["p"]}]}}}}`
	got := changes.Static(create, "", []byte(body), deploymentRoot)
	if len(got) < 3 {
		t.Fatalf("got %v", render(got))
	}
	others := render(got[1:])
	for i := range got[0].Path {
		got[0].Path[i].Key = "mutated"
	}
	if diff := cmp.Diff(others, render(got[1:])); diff != "" {
		t.Errorf("mutating one change's path altered another's (-want +got): %s", diff)
	}
}

func TestDiffChangedImageOnly(t *testing.T) {
	after := strings.Replace(deployment, "nginx:1.25", "nginx:1.26", 1)
	got, err := changes.Diff([]byte(deployment), []byte(after))
	if err != nil {
		t.Fatal(err)
	}
	checkSorted(t, got, "spec.template.spec.containers[0].image")
	if got[0].Subtree {
		t.Error("a Diff change is a leaf")
	}
}

func TestDiffAddedAndRemovedFields(t *testing.T) {
	tests := []struct {
		name          string
		before, after string
		want          []string
	}{
		{
			"a removed env list gives each removed leaf",
			`{"c":{"env":[{"name":"A","value":"1"},{"name":"B","value":"2"}],"image":"x"}}`,
			`{"c":{"image":"x"}}`,
			[]string{"c.env[0].name", "c.env[0].value", "c.env[1].name", "c.env[1].value"},
		},
		{
			"an added env list gives each added leaf",
			`{"c":{"image":"x"}}`,
			`{"c":{"image":"x","env":[{"name":"A"}]}}`,
			[]string{"c.env[0].name"},
		},
		{
			"an emptied list is a leaf and so are the removed elements",
			`{"c":{"args":["a","b"]}}`,
			`{"c":{"args":[]}}`,
			[]string{"c.args", "c.args[0]", "c.args[1]"},
		},
		{
			"an emptied object is a leaf and so are the removed fields",
			`{"c":{"labels":{"a":"1"}}}`,
			`{"c":{"labels":{}}}`,
			[]string{"c.labels", "c.labels.a"},
		},
		{
			"a created empty object is a leaf",
			`{"c":{}}`,
			`{"c":{"labels":{}}}`,
			[]string{"c.labels"},
		},
		{
			"a created empty list is a leaf",
			`{"c":{}}`,
			`{"c":{"args":[]}}`,
			[]string{"c.args"},
		},
		{
			"a shortened list loses its tail",
			`{"c":{"args":["a","b","c"]}}`,
			`{"c":{"args":["a"]}}`,
			[]string{"c.args[1]", "c.args[2]"},
		},
		{
			"a changed element is compared by index",
			`{"c":{"args":["a","b"]}}`,
			`{"c":{"args":["a","z"]}}`,
			[]string{"c.args[1]"},
		},
		{
			"a scalar replaced by an object",
			`{"c":{"x":1}}`,
			`{"c":{"x":{"y":2}}}`,
			[]string{"c.x", "c.x.y"},
		},
		{
			"an object replaced by a list",
			`{"c":{"x":{"y":2}}}`,
			`{"c":{"x":[3]}}`,
			[]string{"c.x.y", "c.x[0]"},
		},
		{
			"null becomes a value",
			`{"c":{"x":null}}`,
			`{"c":{"x":"v"}}`,
			[]string{"c.x"},
		},
		{
			"a string and a number of the same text differ",
			`{"c":{"x":"1"}}`,
			`{"c":{"x":1}}`,
			[]string{"c.x"},
		},
		{
			"a bool flips",
			`{"c":{"x":true}}`,
			`{"c":{"x":false}}`,
			[]string{"c.x"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := changes.Diff([]byte(tt.before), []byte(tt.after))
			if err != nil {
				t.Fatal(err)
			}
			checkSorted(t, got, tt.want...)
			for _, c := range got {
				if c.Subtree {
					t.Errorf("Diff change %s is a Subtree", fieldpath.Format(c.Path))
				}
			}
		})
	}
}

func TestDiffNumbersCompareAsWritten(t *testing.T) {
	same, err := changes.Diff([]byte(`{"replicas":1,"big":12345678901234567890,"f":0.1}`), []byte(`{"f":0.1,"big":12345678901234567890,"replicas":1}`))
	if err != nil {
		t.Fatal(err)
	}
	checkSorted(t, same)

	differ, err := changes.Diff([]byte(`{"replicas":1}`), []byte(`{"replicas":1.0}`))
	if err != nil {
		t.Fatal(err)
	}
	checkSorted(t, differ, "replicas")

	// Two integers float64 can't tell apart still differ.
	big, err := changes.Diff([]byte(`{"n":12345678901234567890}`), []byte(`{"n":12345678901234567891}`))
	if err != nil {
		t.Fatal(err)
	}
	checkSorted(t, big, "n")
}

func TestDiff_IdenticalDocumentsAreUnchanged(t *testing.T) {
	got, err := changes.Diff([]byte(deployment), []byte(deployment))
	if err != nil {
		t.Fatal(err)
	}
	checkSorted(t, got)
}

func TestDiff_ErrorsOnInvalidJSON(t *testing.T) {
	for _, tt := range []struct{ before, after string }{{`{`, `{}`}, {`{}`, `{`}, {``, `{}`}, {`{}`, `{} {}`}} {
		if _, err := changes.Diff([]byte(tt.before), []byte(tt.after)); err == nil {
			t.Errorf("Diff(%q, %q) succeeded", tt.before, tt.after)
		}
	}
}

func TestDiff_Deterministic(t *testing.T) {
	before := `{"a":{"x":1,"y":2,"z":[1,2]},"b":3,"c":[{"k":1}]}`
	after := `{"a":{"x":2,"w":0},"d":4,"c":[{"k":2},{"k":3}]}`
	first, err := changes.Diff([]byte(before), []byte(after))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.w", "a.x", "a.y", "a.z[0]", "a.z[1]", "b", "c[0].k", "c[1].k", "d"}
	if diff := cmp.Diff(want, render(first)); diff != "" {
		t.Errorf("changes (-want +got):\n%s", diff)
	}
	for range 20 {
		again, _ := changes.Diff([]byte(before), []byte(after))
		if diff := cmp.Diff(render(first), render(again)); diff != "" {
			t.Fatalf("Diff is not deterministic:\n%s", diff)
		}
	}
}

const live = `{
  "apiVersion": "apps/v1",
  "kind": "Deployment",
  "metadata": {"name": "web", "namespace": "default"},
  "spec": {
    "replicas": 2,
    "template": {
      "spec": {
        "containers": [
          {"name": "app", "image": "nginx:1.25", "args": ["-g"]},
          {"name": "sidecar", "image": "envoy:1.30"}
        ]
      }
    }
  }
}`

func apply(t *testing.T, pt v1alpha1.PatchType, patchBody string, root schema.Node) any {
	t.Helper()
	out, err := changes.Apply(pt, []byte(live), []byte(patchBody), root)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	var doc any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("Apply returned invalid JSON %q: %v", out, err)
	}
	return doc
}

func doc(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestApply_JSONPatch(t *testing.T) {
	got := apply(t, jsonPatch, `[
		{"op":"test","path":"/spec/replicas","value":2},
		{"op":"replace","path":"/spec/replicas","value":5},
		{"op":"add","path":"/spec/template/spec/containers/0/image","value":"nginx:1.26"},
		{"op":"remove","path":"/spec/template/spec/containers/1"},
		{"op":"add","path":"/metadata/annotations","value":{"a/b":"c"}}
	]`, deploymentRoot)
	want := doc(t, `{
	  "apiVersion": "apps/v1", "kind": "Deployment",
	  "metadata": {"name": "web", "namespace": "default", "annotations": {"a/b": "c"}},
	  "spec": {"replicas": 5, "template": {"spec": {"containers": [
	    {"name": "app", "image": "nginx:1.26", "args": ["-g"]}
	  ]}}}
	}`)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("document (-want +got):\n%s", diff)
	}
}

func TestApply_JSONPatchErrors(t *testing.T) {
	for name, body := range map[string]string{
		"failed test":    `[{"op":"test","path":"/spec/replicas","value":99}]`,
		"missing path":   `[{"op":"remove","path":"/spec/nothing/here"}]`,
		"not a patch":    `{"op":"add"}`,
		"invalid JSON":   `[`,
		"unknown op":     `[{"op":"frobnicate","path":"/a"}]`,
		"bad array slot": `[{"op":"replace","path":"/spec/template/spec/containers/7","value":{}}]`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := changes.Apply(jsonPatch, []byte(live), []byte(body), deploymentRoot); err == nil {
				t.Error("Apply succeeded")
			}
		})
	}
}

func TestApply_MergePatchReplacesLists(t *testing.T) {
	got := apply(t, mergePatch, `{"spec":{"replicas":3,"template":{"spec":{"containers":[{"name":"app","image":"nginx:1.26"}]}}},"metadata":{"namespace":null}}`, deploymentRoot)
	want := doc(t, `{
	  "apiVersion": "apps/v1", "kind": "Deployment",
	  "metadata": {"name": "web"},
	  "spec": {"replicas": 3, "template": {"spec": {"containers": [
	    {"name": "app", "image": "nginx:1.26"}
	  ]}}}
	}`)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("document (-want +got):\n%s", diff)
	}
}

func TestApply_StrategicMergePatchKeepsUntouchedContainers(t *testing.T) {
	got := apply(t, strategicPatch, `{"spec":{"template":{"spec":{"containers":[{"name":"app","image":"nginx:1.26"}]}}}}`, deploymentRoot)
	want := doc(t, `{
	  "apiVersion": "apps/v1", "kind": "Deployment",
	  "metadata": {"name": "web", "namespace": "default"},
	  "spec": {"replicas": 2, "template": {"spec": {"containers": [
	    {"name": "app", "image": "nginx:1.26", "args": ["-g"]},
	    {"name": "sidecar", "image": "envoy:1.30"}
	  ]}}}
	}`)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("document (-want +got):\n%s", diff)
	}
}

func TestApply_StrategicMergePatchDirectives(t *testing.T) {
	t.Run("$patch: delete removes the element", func(t *testing.T) {
		got := apply(t, strategicPatch, `{"spec":{"template":{"spec":{"containers":[{"name":"sidecar","$patch":"delete"}]}}}}`, deploymentRoot)
		want := doc(t, `{
		  "apiVersion": "apps/v1", "kind": "Deployment",
		  "metadata": {"name": "web", "namespace": "default"},
		  "spec": {"replicas": 2, "template": {"spec": {"containers": [
		    {"name": "app", "image": "nginx:1.25", "args": ["-g"]}
		  ]}}}
		}`)
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("document (-want +got):\n%s", diff)
		}
	})
	t.Run("$patch: replace replaces the object", func(t *testing.T) {
		got := apply(t, strategicPatch, `{"spec":{"template":{"spec":{"$patch":"replace","containers":[{"name":"only","image":"x"}]}}}}`, deploymentRoot)
		want := doc(t, `{
		  "apiVersion": "apps/v1", "kind": "Deployment",
		  "metadata": {"name": "web", "namespace": "default"},
		  "spec": {"replicas": 2, "template": {"spec": {"containers": [{"name": "only", "image": "x"}]}}}
		}`)
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("document (-want +got):\n%s", diff)
		}
	})
	t.Run("an atomic list is replaced", func(t *testing.T) {
		got := apply(t, strategicPatch, `{"spec":{"template":{"spec":{"containers":[{"name":"app","args":["-x"]}]}}}}`, deploymentRoot)
		want := doc(t, `{
		  "apiVersion": "apps/v1", "kind": "Deployment",
		  "metadata": {"name": "web", "namespace": "default"},
		  "spec": {"replicas": 2, "template": {"spec": {"containers": [
		    {"name": "app", "image": "nginx:1.25", "args": ["-x"]},
		    {"name": "sidecar", "image": "envoy:1.30"}
		  ]}}}
		}`)
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("document (-want +got):\n%s", diff)
		}
	})
}

func TestApply_StrategicMergePatchUnderUnknownSchemaReplacesLists(t *testing.T) {
	got := apply(t, strategicPatch, `{"spec":{"template":{"spec":{"containers":[{"name":"app","image":"nginx:1.26"}]}}}}`, schema.Node{})
	want := doc(t, `{
	  "apiVersion": "apps/v1", "kind": "Deployment",
	  "metadata": {"name": "web", "namespace": "default"},
	  "spec": {"replicas": 2, "template": {"spec": {"containers": [
	    {"name": "app", "image": "nginx:1.26"}
	  ]}}}
	}`)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("document (-want +got):\n%s", diff)
	}
}

func TestApply_Errors(t *testing.T) {
	if _, err := changes.Apply("text/plain", []byte(live), []byte(`{}`), deploymentRoot); err == nil {
		t.Error("an unknown patch type succeeded")
	}
	if _, err := changes.Apply(mergePatch, []byte(live), []byte(`{`), deploymentRoot); err == nil {
		t.Error("an invalid merge patch succeeded")
	}
	if _, err := changes.Apply(strategicPatch, []byte(live), []byte(`[1]`), deploymentRoot); err == nil {
		t.Error("a strategic merge patch that isn't an object succeeded")
	}
}

// TestApplyThenDiffIsCoveredByStatic is the property the exact evaluation
// leans on: whatever Diff finds between a document and its patched self lies
// at or beneath, or is an ancestor leaf under, some change Static predicted.
func TestApplyThenDiffIsCoveredByStatic(t *testing.T) {
	tests := []struct {
		pt    v1alpha1.PatchType
		patch string
	}{
		{jsonPatch, `[{"op":"replace","path":"/spec/replicas","value":9},{"op":"add","path":"/spec/template/spec/containers/0/image","value":"x"}]`},
		{mergePatch, `{"spec":{"replicas":3,"template":{"spec":{"containers":[{"name":"app","image":"y"}]}}}}`},
		{strategicPatch, `{"spec":{"template":{"spec":{"containers":[{"name":"app","image":"z"},{"name":"sidecar","$patch":"delete"}]}}}}`},
		{strategicPatch, `{"spec":{"template":{"spec":{"$patch":"replace","containers":[{"name":"n","image":"i"}]}}}}`},
	}
	for _, tt := range tests {
		static := changes.Static(patch, tt.pt, []byte(tt.patch), deploymentRoot)
		out, err := changes.Apply(tt.pt, []byte(live), []byte(tt.patch), deploymentRoot)
		if err != nil {
			t.Fatal(err)
		}
		exact, err := changes.Diff([]byte(live), out)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range exact {
			if !coveredBy(static, e.Path) {
				t.Errorf("%s: exact change %s is not covered by static %v", tt.pt, fieldpath.Format(e.Path), render(static))
			}
		}
	}
}

// coveredBy reports whether some static change is at or above path, a leaf
// change standing for its own field and a Subtree change for everything
// beneath it. List element indexes are ignored, as the rule paths do.
func coveredBy(static []fieldpath.Change, path []fieldpath.Segment) bool {
	for _, s := range static {
		if len(s.Path) > len(path) || (!s.Subtree && len(s.Path) != len(path)) {
			continue
		}
		ok := true
		for i, seg := range s.Path {
			p := path[i]
			if seg.Elem != p.Elem || (!seg.Elem && seg.Key != p.Key) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
