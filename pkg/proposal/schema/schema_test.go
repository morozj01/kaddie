package schema_test

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/discovery/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/morozj01/kaddie/pkg/proposal/fieldpath"
	"github.com/morozj01/kaddie/pkg/proposal/schema"
)

var (
	deployments = schema.Resource{Group: "apps", Version: "v1", Resource: "deployments"}
	pods        = schema.Resource{Version: "v1", Resource: "pods"}
	cronjobs    = schema.Resource{Group: "batch", Version: "v1", Resource: "cronjobs"}
)

// segs builds a concrete changed path from keys, "#" being a list element.
func segs(keys ...string) []fieldpath.Segment {
	var out []fieldpath.Segment
	for _, k := range keys {
		if k == "#" {
			out = append(out, fieldpath.Segment{Elem: true})
			continue
		}
		out = append(out, fieldpath.Segment{Key: k})
	}
	return out
}

// rule builds a rule path from step names, "#" being [*].
func rule(anyDepth bool, steps ...string) fieldpath.Path {
	p := fieldpath.Path{AnyDepth: anyDepth}
	for _, s := range steps {
		if s == "#" {
			p.Steps = append(p.Steps, fieldpath.Step{AnyElem: true})
			continue
		}
		p.Steps = append(p.Steps, fieldpath.Step{Name: s})
	}
	return p
}

func TestListMerge(t *testing.T) {
	deployment := schema.Vendored().Root(deployments)
	pod := schema.Vendored().Root(pods)

	tests := []struct {
		name      string
		node      schema.Node
		wantMerge bool
		wantKey   string
	}{
		{"containers merge by name", deployment.At(segs("spec", "template", "spec", "containers")), true, "name"},
		{"container args are replaced", deployment.At(segs("spec", "template", "spec", "containers", "#", "args")), false, ""},
		{"container env merges by name", deployment.At(segs("spec", "template", "spec", "containers", "#", "env")), true, "name"},
		{"container ports merge by containerPort", deployment.At(segs("spec", "template", "spec", "containers", "#", "ports")), true, "containerPort"},
		{"volumes merge by name", deployment.At(segs("spec", "template", "spec", "volumes")), true, "name"},
		{"pod finalizers merge with no key", pod.At(segs("metadata", "finalizers")), true, ""},
		{"owner references merge by uid", pod.At(segs("metadata", "ownerReferences")), true, "uid"},
		{"a scalar is not a merged list", deployment.At(segs("spec", "replicas")), false, ""},
		{"unknown node replaces", schema.Node{}, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merge, key := tt.node.ListMerge()
			if merge != tt.wantMerge || key != tt.wantKey {
				t.Fatalf("ListMerge() = (%v, %q), want (%v, %q)", merge, key, tt.wantMerge, tt.wantKey)
			}
		})
	}
}

func TestField(t *testing.T) {
	pod := schema.Vendored().Root(pods)

	if !pod.Known() {
		t.Fatal("pod root is unknown")
	}
	if !pod.Field("spec").Known() {
		t.Error("Field(spec) is unknown")
	}
	if pod.Field("nonsense").Known() {
		t.Error("Field(nonsense) is known")
	}
	if (schema.Node{}).Field("spec").Known() {
		t.Error("the unknown node has a known field")
	}
	labels := pod.Field("metadata").Field("labels")
	if !labels.Field("any.example/key").Known() {
		t.Error("a map's values are unknown for an arbitrary key")
	}
	if pod.Field("metadata").Field("labels").Field("x").Field("y").Known() {
		t.Error("a string map value has a field")
	}
	if (schema.Node{}).Elem().Known() {
		t.Error("the unknown node has elements")
	}
	if pod.Field("spec").Elem().Known() {
		t.Error("an object has elements")
	}
}

func TestAt(t *testing.T) {
	deployment := schema.Vendored().Root(deployments)

	known := [][]fieldpath.Segment{
		nil,
		segs("spec", "template", "spec", "containers", "#", "image"),
		// An index text, as the analysis gives for containers[0].
		{{Key: "spec"}, {Key: "template"}, {Key: "spec"}, {Key: "containers"}, {Key: "0", Elem: true}, {Key: "image"}},
		// A JSON Patch append addressing a list element.
		{{Key: "spec"}, {Key: "template"}, {Key: "spec"}, {Key: "containers"}, {Key: "-", Elem: true}, {Key: "image"}},
		// A JSON Patch number that addresses a map key, not a list element.
		{{Key: "metadata"}, {Key: "labels"}, {Key: "0", Elem: true}},
		segs("metadata", "labels", "a.b/c"),
	}
	for _, p := range known {
		if !deployment.At(p).Known() {
			t.Errorf("At(%v) is unknown", p)
		}
	}

	unknown := [][]fieldpath.Segment{
		segs("spec", "nonsense"),
		segs("spec", "template", "spec", "nonsense", "#", "image"),
		segs("spec", "replicas", "#"),
		segs("spec", "template", "spec", "containers", "#", "image", "#"),
	}
	for _, p := range unknown {
		if deployment.At(p).Known() {
			t.Errorf("At(%v) is known", p)
		}
	}

	// A walk through the unknown stays unknown, never a panic.
	if (schema.Node{}).At(segs("a", "#", "b")).Known() {
		t.Error("walking from the unknown node found a schema")
	}
}

func TestReaches(t *testing.T) {
	deployment := schema.Vendored().Root(deployments)
	cronjob := schema.Vendored().Root(cronjobs)
	pod := schema.Vendored().Root(pods)

	serviceAccount := rule(true, "serviceAccountName")
	containerImage := rule(true, "containers", "#", "image")

	tests := []struct {
		name string
		from schema.Node
		path fieldpath.Path
		want bool
	}{
		{"template reaches **.serviceAccountName", deployment.At(segs("spec", "template")), serviceAccount, true},
		{"template reaches **.containers[*].image", deployment.At(segs("spec", "template")), containerImage, true},
		{"the root reaches **.containers[*].image", deployment, containerImage, true},
		{"replicas reaches nothing: serviceAccountName", deployment.At(segs("spec", "replicas")), serviceAccount, false},
		{"replicas reaches nothing: image", deployment.At(segs("spec", "replicas")), containerImage, false},
		{"replicas reaches nothing: a plain name", deployment.At(segs("spec", "replicas")), rule(false, "x"), false},
		{"strategy holds no pod fields", deployment.At(segs("spec", "strategy")), containerImage, false},
		{"maxSurge, an int-or-string, reaches nothing", deployment.At(segs("spec", "strategy", "rollingUpdate", "maxSurge")), containerImage, false},
		{"resources does not reach containers[*].image", deployment.At(segs("spec", "template", "spec", "containers", "#", "resources")), rule(false, "containers", "#", "image"), false},
		{"resources does not reach **.containers[*].image", deployment.At(segs("spec", "template", "spec", "containers", "#", "resources")), containerImage, false},
		{"a container reaches its image", deployment.At(segs("spec", "template", "spec", "containers", "#")), rule(false, "image"), true},
		{"a container has no serviceAccountName", deployment.At(segs("spec", "template", "spec", "containers", "#")), serviceAccount, false},
		{"jobTemplate reaches **.containers[*].image", cronjob.At(segs("spec", "jobTemplate")), containerImage, true},
		{"cronjob spec reaches **.serviceAccountName", cronjob.At(segs("spec")), serviceAccount, true},
		{"cronjob schedule reaches nothing", cronjob.At(segs("spec", "schedule")), containerImage, false},
		{"an exact path through the schema exists", deployment, rule(false, "spec", "template", "spec", "containers", "#", "image"), true},
		{"an exact path off the schema does not", deployment, rule(false, "spec", "template", "spec", "nonsense"), false},
		{"an exact path through a list needs [*]", deployment, rule(false, "spec", "template", "spec", "containers", "image"), false},
		{"a map's value exists for any key", pod, rule(false, "metadata", "labels", "any.example/key"), true},
		{"a string map value has nothing beneath", pod, rule(false, "metadata", "labels", "k", "deeper"), false},
		{"a labels map holds no serviceAccountName field", deployment.At(segs("metadata", "labels")), serviceAccount, false},
		{"an annotations map holds no pod field", deployment.At(segs("metadata", "annotations")), rule(true, "hostNetwork"), false},
		{"selector matchLabels holds no serviceAccountName field", deployment.At(segs("spec", "selector")), serviceAccount, false},
		{"an exact rule may still name a map key", deployment, rule(false, "metadata", "labels", "serviceAccountName"), true},
		{"fieldsV1, a serialized field set, reaches nothing", pod.At(segs("metadata", "managedFields", "#", "fieldsV1")), containerImage, false},
		{"a pod template's metadata holds no pod field", deployment.At(segs("spec", "template", "metadata")), serviceAccount, false},
		{"the unknown node reaches **.containers[*].image", schema.Node{}, containerImage, true},
		{"the unknown node reaches an exact path", schema.Node{}, rule(false, "a", "#", "b"), true},
		{"a node unknown beneath the root reaches everything", deployment.At(segs("spec", "nonsense")), serviceAccount, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.from.Reaches(tt.path); got != tt.want {
				t.Fatalf("Reaches(%+v) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestRoot_Subresources(t *testing.T) {
	vendored := schema.Vendored()
	scale := vendored.Root(schema.Resource{Group: "apps", Version: "v1", Resource: "deployments", Subresource: "scale"})
	if !scale.Known() {
		t.Fatal("deployments/scale is unknown")
	}
	if !scale.Field("spec").Field("replicas").Known() {
		t.Error("Scale has no spec.replicas")
	}
	if !scale.Field("status").Field("replicas").Known() {
		t.Error("Scale has no status.replicas")
	}
	if scale.Field("spec").Field("template").Known() {
		t.Error("deployments/scale resolved to the Deployment schema, not Scale")
	}

	for _, r := range []schema.Resource{
		{Group: "apps", Version: "v1", Resource: "deployments", Subresource: "status"},
		{Version: "v1", Resource: "pods", Subresource: "ephemeralcontainers"},
		{Version: "v1", Resource: "replicationcontrollers", Subresource: "scale"},
		{Group: "batch", Version: "v1", Resource: "cronjobs", Subresource: "status"},
	} {
		if !vendored.Root(r).Known() {
			t.Errorf("Root(%+v) is unknown", r)
		}
	}
	if !vendored.Root(schema.Resource{Version: "v1", Resource: "pods", Subresource: "ephemeralcontainers"}).
		At(segs("spec", "ephemeralContainers")).Known() {
		t.Error("pods/ephemeralcontainers is not the Pod schema")
	}
}

func TestVendored_UnrecordedResourcesAreUnknown(t *testing.T) {
	for _, r := range []schema.Resource{
		{Group: "apps", Version: "v1", Resource: "controllerrevisions"},
		{Version: "v1", Resource: "configmaps"},
		{Version: "v1", Resource: "pods", Subresource: "log"},
		{Group: "apps", Version: "v1beta1", Resource: "deployments"},
		{Group: "apps", Resource: "deployments"},
		{},
	} {
		if schema.Vendored().Root(r).Known() {
			t.Errorf("Root(%+v) is known", r)
		}
	}
}

func TestRecorded_EveryEntryResolves(t *testing.T) {
	recorded := schema.Recorded()
	if len(recorded) == 0 {
		t.Fatal("nothing is recorded")
	}
	tops := map[string]bool{}
	for _, r := range recorded {
		if !schema.Vendored().Root(r).Known() {
			t.Errorf("recorded %+v is unknown", r)
		}
		if r.Subresource == "" {
			tops[r.Group+"/"+r.Resource] = true
		}
	}
	for _, want := range []string{"/pods", "/replicationcontrollers", "apps/deployments", "apps/replicasets", "apps/statefulsets", "apps/daemonsets", "batch/jobs", "batch/cronjobs"} {
		if !tops[want] {
			t.Errorf("%s is not recorded", want)
		}
	}
}

func TestLive_FailureIsUnknown(t *testing.T) {
	d := &fake.FakeDiscovery{Fake: &clienttesting.Fake{}}
	if schema.Live(d).Root(deployments).Known() {
		t.Fatal("a cluster that lists nothing yielded a schema")
	}
}

func TestPatchMeta_MergesContainersByName(t *testing.T) {
	original := mustJSON(t, map[string]any{
		"spec": map[string]any{"template": map[string]any{"spec": map[string]any{"containers": []any{
			map[string]any{"name": "app", "image": "app:1"},
			map[string]any{"name": "sidecar", "image": "side:1", "resources": map[string]any{"limits": map[string]any{"cpu": "100m"}}},
		}}}},
	})
	patch := mustJSON(t, map[string]any{
		"spec": map[string]any{"template": map[string]any{"spec": map[string]any{"containers": []any{
			map[string]any{"name": "app", "resources": map[string]any{"limits": map[string]any{"memory": "64Mi"}}},
		}}}},
	})

	merged, err := strategicpatch.StrategicMergePatchUsingLookupPatchMeta(original, patch, schema.Vendored().Root(deployments).PatchMeta())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"spec": map[string]any{"template": map[string]any{"spec": map[string]any{"containers": []any{
			map[string]any{"name": "app", "image": "app:1", "resources": map[string]any{"limits": map[string]any{"memory": "64Mi"}}},
			map[string]any{"name": "sidecar", "image": "side:1", "resources": map[string]any{"limits": map[string]any{"cpu": "100m"}}},
		}}}},
	}
	if diff := cmp.Diff(want, mustDecode(t, merged)); diff != "" {
		t.Fatalf("merged object differs (-want +got):\n%s", diff)
	}

	// Without a schema the same patch replaces the list: that loss of the
	// sidecar is what merge keys prevent.
	replaced, err := strategicpatch.StrategicMergePatchUsingLookupPatchMeta(original, patch, (schema.Node{}).PatchMeta())
	if err != nil {
		t.Fatal(err)
	}
	containers := mustDecode(t, replaced)["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)
	if len(containers) != 1 {
		t.Fatalf("the unknown node's metadata kept %d containers, want the list replaced by 1", len(containers))
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustDecode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}
