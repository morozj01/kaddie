// Package schema supplies the OpenAPI v3 schemas the field analysis needs:
// strategic-merge list keys and what fields can exist beneath a changed one.
// Vendored serves schemas recorded from a 1.28 API server, offline; Live asks
// the cluster's discovery and OpenAPI v3 endpoints. Both read the same
// OpenAPI v3 documents through the same code, so they agree wherever their
// documents do.
package schema

import (
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/morozj01/kaddie/pkg/proposal/fieldpath"
)

// Resource is what a request addresses.
type Resource struct {
	Group       string
	Version     string
	Resource    string
	Subresource string
}

// Node is a position in an object's OpenAPI v3 schema. The zero Node is
// unknown: nothing is known about what lies beneath it, so every query on it
// answers conservatively (anything may exist, no list has a merge key).
type Node struct {
	schemas map[string]*spec.Schema
	s       *spec.Schema
}

// Known reports whether n carries a schema. A reference that doesn't resolve
// leaves a node unknown.
func (n Node) Known() bool {
	return n.resolved() != nil
}

// Field returns the node of property name, or of a map's values for any key.
// An unknown name returns the unknown Node.
func (n Node) Field(name string) Node {
	if c, ok := n.child(fieldpath.Step{Name: name}); ok {
		return c
	}
	return Node{}
}

// Elem returns the node of a list's elements.
func (n Node) Elem() Node {
	if c, ok := n.child(fieldpath.Step{AnyElem: true}); ok {
		return c
	}
	return Node{}
}

// At walks a concrete changed path from n.
func (n Node) At(path []fieldpath.Segment) Node {
	for _, seg := range path {
		switch {
		case seg.Elem && n.isList():
			n = n.Elem()
		case seg.Elem && seg.Key == "":
			return Node{}
		default:
			n = n.Field(seg.Key)
		}
	}
	return n
}

// ListMerge reports how a strategic merge patch treats the list at n (a node
// returned by Field): merge is true when its patch strategy includes merge,
// and key is its merge key, empty for a list of primitives. An unknown node
// returns false: the patch replaces the whole list.
func (n Node) ListMerge() (merge bool, key string) {
	if !n.isList() {
		return false, ""
	}
	strategy, _ := n.s.Extensions[extPatchStrategy].(string)
	for _, s := range strings.Split(strategy, ",") {
		if s == "merge" {
			merge = true
		}
	}
	key, _ = n.s.Extensions[extPatchMergeKey].(string)
	return merge, key
}

// Reaches reports whether a field named by p, relative to n, can exist
// beneath n. Unknown nodes, and schemas open to unknown fields, reach
// everything.
func (n Node) Reaches(p fieldpath.Path) bool {
	if !p.AnyDepth {
		return n.reachesSteps(p.Steps)
	}
	return n.reachesBeneath(p.Steps, map[*spec.Schema]bool{})
}

// PatchMeta returns n as strategic-merge metadata, for applying a patch
// client-side with strategicpatch.StrategicMergePatchUsingLookupPatchMeta.
// The unknown Node returns metadata that merges maps and replaces every
// list.
func (n Node) PatchMeta() strategicpatch.LookupPatchMeta {
	s := n.resolved()
	if s == nil {
		return unknownPatchMeta{}
	}
	return strategicpatch.PatchMetaFromOpenAPIV3{SchemaList: n.schemas, Schema: s}
}

const (
	extPatchStrategy = "x-kubernetes-patch-strategy"
	extPatchMergeKey = "x-kubernetes-patch-merge-key"
	extPreserve      = "x-kubernetes-preserve-unknown-fields"
	extGVK           = "x-kubernetes-group-version-kind"
	refPrefix        = "#/components/schemas/"
	maxResolveDepth  = 32
)

// resolved returns the schema n stands for with $ref and single-element
// allOf followed, or nil when n is unknown or the reference doesn't resolve.
func (n Node) resolved() *spec.Schema {
	s := n.s
	for i := 0; s != nil && i < maxResolveDepth; i++ {
		if ref := s.Ref.String(); ref != "" {
			name, ok := strings.CutPrefix(ref, refPrefix)
			if !ok {
				return nil
			}
			s = n.schemas[name]
			continue
		}
		if len(s.AllOf) == 1 && len(s.Type) == 0 && len(s.Properties) == 0 && s.Items == nil && s.AdditionalProperties == nil {
			s = &s.AllOf[0]
			continue
		}
		return s
	}
	return nil
}

func (n Node) at(s *spec.Schema) Node {
	return Node{schemas: n.schemas, s: s}
}

func (n Node) isList() bool {
	s := n.resolved()
	return s != nil && (s.Items != nil || s.Type.Contains("array"))
}

// open reports whether anything may exist beneath n: it is unknown, keeps
// unknown fields, or is a free-form object.
func (n Node) open() bool {
	s := n.resolved()
	if s == nil {
		return true
	}
	if v, _ := s.Extensions[extPreserve].(bool); v {
		return true
	}
	switch {
	case s.Items != nil || s.Type.Contains("array"):
		return false
	case len(s.OneOf)+len(s.AnyOf) > 0:
		// IntOrString and Quantity: a union of scalars has nothing beneath it.
		for _, b := range append(slices.Clone(s.OneOf), s.AnyOf...) {
			if !n.at(&b).scalar() {
				return true
			}
		}
		return false
	case len(s.Properties) > 0:
		return s.AdditionalProperties != nil && s.AdditionalProperties.Allows && s.AdditionalProperties.Schema == nil
	case s.AdditionalProperties != nil:
		return s.AdditionalProperties.Allows && s.AdditionalProperties.Schema == nil
	case len(s.Type) > 0 && !s.Type.Contains("object"):
		return false
	}
	return true
}

func (n Node) scalar() bool {
	s := n.resolved()
	return s != nil && len(s.Type) > 0 && !s.Type.Contains("object") && !s.Type.Contains("array")
}

// child returns the node one step beneath n. ok is false when the schema
// admits no such field; an open n answers with the unknown Node.
func (n Node) child(st fieldpath.Step) (c Node, ok bool) {
	if n.open() {
		return Node{}, true
	}
	s := n.resolved()
	if st.AnyElem {
		if s.Items != nil && s.Items.Schema != nil {
			return n.at(s.Items.Schema), true
		}
		return Node{}, false
	}
	if p, found := s.Properties[st.Name]; found {
		return n.at(&p), true
	}
	if s.AdditionalProperties != nil && s.AdditionalProperties.Schema != nil {
		return n.at(s.AdditionalProperties.Schema), true
	}
	return Node{}, false
}

// children returns every node one step beneath n.
func (n Node) children() []Node {
	s := n.resolved()
	if s == nil {
		return nil
	}
	var out []Node
	for name := range s.Properties {
		p := s.Properties[name]
		out = append(out, n.at(&p))
	}
	if s.Items != nil && s.Items.Schema != nil {
		out = append(out, n.at(s.Items.Schema))
	}
	if s.AdditionalProperties != nil && s.AdditionalProperties.Schema != nil {
		out = append(out, n.at(s.AdditionalProperties.Schema))
	}
	return out
}

func (n Node) reachesSteps(steps []fieldpath.Step) bool {
	for _, st := range steps {
		if n.open() {
			return true
		}
		c, ok := n.child(st)
		if !ok {
			return false
		}
		n = c
	}
	return true
}

// anchors reports whether steps match from n as the tail of a ** path. Such a
// path names a field, so its first step must be a property of n: a map key
// that happens to read serviceAccountName is not that field, and treating it
// as one would flag the removal of any labels or annotations map.
func (n Node) anchors(steps []fieldpath.Step) bool {
	if len(steps) > 0 && !steps[0].AnyElem {
		if _, ok := n.resolved().Properties[steps[0].Name]; !ok {
			return false
		}
	}
	return n.reachesSteps(steps)
}

// reachesBeneath reports whether steps match from n or from any node beneath
// it. seen holds the schemas already searched, which ends the walk of a
// recursive schema.
func (n Node) reachesBeneath(steps []fieldpath.Step, seen map[*spec.Schema]bool) bool {
	if n.open() || n.anchors(steps) {
		return true
	}
	if r := n.resolved(); r != nil {
		if seen[r] {
			return false
		}
		seen[r] = true
	}
	for _, c := range n.children() {
		if c.reachesBeneath(steps, seen) {
			return true
		}
	}
	return false
}

// unknownPatchMeta is the strategic-merge metadata of an unknown schema: no
// list has a merge key, so every list is replaced and maps merge by key.
type unknownPatchMeta struct{}

func (u unknownPatchMeta) LookupPatchMetadataForStruct(string) (strategicpatch.LookupPatchMeta, strategicpatch.PatchMeta, error) {
	return u, strategicpatch.PatchMeta{}, nil
}

func (u unknownPatchMeta) LookupPatchMetadataForSlice(string) (strategicpatch.LookupPatchMeta, strategicpatch.PatchMeta, error) {
	return u, strategicpatch.PatchMeta{}, nil
}

func (unknownPatchMeta) Name() string { return "unknown" }

// Provider resolves the schema of the object a request reads or writes.
type Provider interface {
	// Root returns the schema of the body of a request on r: the kind for a
	// resource, the subresource's kind (Scale for deployments/scale) for a
	// subresource. It returns the unknown Node when it can't tell.
	Root(r Resource) Node
}
