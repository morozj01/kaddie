package schema

import (
	"encoding/json"
	"testing"

	"k8s.io/kube-openapi/pkg/spec3"

	"github.com/morozj01/kaddie/pkg/proposal/fieldpath"
)

// docNode builds the root Node of component Root in a hand-written document,
// for the shapes the vendored workload kinds don't contain: recursion, kept
// unknown fields and free-form objects.
func docNode(t *testing.T, schemas string) Node {
	t.Helper()
	var doc spec3.OpenAPI
	if err := json.Unmarshal([]byte(`{"components":{"schemas":`+schemas+`}}`), &doc); err != nil {
		t.Fatal(err)
	}
	return Node{schemas: doc.Components.Schemas, s: doc.Components.Schemas["Root"]}
}

func TestReaches_RecursiveSchemaTerminates(t *testing.T) {
	root := docNode(t, `{
		"Root": {"type": "object", "properties": {"tree": {"allOf": [{"$ref": "#/components/schemas/Tree"}]}}},
		"Tree": {"type": "object", "properties": {
			"name": {"type": "string"},
			"children": {"type": "array", "items": {"allOf": [{"$ref": "#/components/schemas/Tree"}]}}}}
	}`)

	if root.Reaches(rulePath(true, "image")) {
		t.Error("found a field the recursive schema lacks")
	}
	if !root.Reaches(rulePath(true, "children", "#", "name")) {
		t.Error("missed a field at any depth of the recursive schema")
	}
	if !root.At([]fieldpath.Segment{{Key: "tree"}, {Key: "children"}, {Elem: true}, {Key: "children"}, {Elem: true}, {Key: "name"}}).Known() {
		t.Error("At could not walk the recursive schema")
	}
}

func TestReaches_OpenSchemas(t *testing.T) {
	root := docNode(t, `{
		"Root": {"type": "object", "properties": {
			"kept":     {"type": "object", "x-kubernetes-preserve-unknown-fields": true, "properties": {"a": {"type": "string"}}},
			"freeform": {"type": "object"},
			"anything": {"type": "object", "additionalProperties": true},
			"typed":    {"type": "object", "additionalProperties": {"type": "string"}},
			"closed":   {"type": "object", "properties": {"a": {"type": "string"}}}
		}}
	}`)
	for _, field := range []string{"kept", "freeform", "anything"} {
		if !root.Field(field).Reaches(rulePath(false, "x", "y")) {
			t.Errorf("%s does not reach everything", field)
		}
		if !root.Field(field).Reaches(rulePath(true, "x")) {
			t.Errorf("%s does not reach everything through **", field)
		}
	}
	for _, field := range []string{"typed", "closed"} {
		if root.Field(field).Reaches(rulePath(true, "x", "y")) {
			t.Errorf("%s reaches a field it lacks", field)
		}
	}
}

func TestPatchMeta_UnresolvableReferenceIsUnknown(t *testing.T) {
	root := docNode(t, `{"Root": {"allOf": [{"$ref": "#/components/schemas/Missing"}]}}`)
	if root.Known() {
		t.Error("a dangling reference is a known node")
	}
	if !root.Reaches(rulePath(false, "a")) {
		t.Error("a dangling reference should be treated as unknown and reach everything")
	}
	if _, ok := root.PatchMeta().(unknownPatchMeta); !ok {
		t.Errorf("PatchMeta() = %T, want the unknown metadata", root.PatchMeta())
	}
}

func rulePath(anyDepth bool, steps ...string) fieldpath.Path {
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
