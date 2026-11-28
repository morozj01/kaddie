// Package schema supplies the OpenAPI v3 schemas the field analysis needs:
// strategic-merge list keys and what fields can exist beneath a changed one.
// Vendored serves schemas recorded from a 1.28 API server, offline; Live asks
// the cluster's discovery and OpenAPI v3 endpoints. Both read the same
// OpenAPI v3 documents through the same code, so they agree wherever their
// documents do.
package schema

import (
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/discovery"
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

// Known reports whether n carries a schema.
func (n Node) Known() bool {
	panic("unimplemented")
}

// Field returns the node of property name, or of a map's values for any key.
// An unknown name returns the unknown Node.
func (n Node) Field(name string) Node {
	panic("unimplemented")
}

// Elem returns the node of a list's elements.
func (n Node) Elem() Node {
	panic("unimplemented")
}

// At walks a concrete changed path from n.
func (n Node) At(path []fieldpath.Segment) Node {
	panic("unimplemented")
}

// ListMerge reports how a strategic merge patch treats the list at n (a node
// returned by Field): merge is true when its patch strategy includes merge,
// and key is its merge key, empty for a list of primitives. An unknown node
// returns false: the patch replaces the whole list.
func (n Node) ListMerge() (merge bool, key string) {
	panic("unimplemented")
}

// Reaches reports whether a field named by p, relative to n, can exist
// beneath n. Unknown nodes, and schemas open to unknown fields, reach
// everything.
func (n Node) Reaches(p fieldpath.Path) bool {
	panic("unimplemented")
}

// PatchMeta returns n as strategic-merge metadata, for applying a patch
// client-side with strategicpatch.StrategicMergePatchUsingLookupPatchMeta.
func (n Node) PatchMeta() strategicpatch.LookupPatchMeta {
	panic("unimplemented")
}

// Provider resolves the schema of the object a request reads or writes.
type Provider interface {
	// Root returns the schema of the body of a request on r: the kind for a
	// resource, the subresource's kind (Scale for deployments/scale) for a
	// subresource. It returns the unknown Node when it can't tell.
	Root(r Resource) Node
}

// Vendored returns the provider of the embedded schemas, recorded from a
// 1.28 API server by hack/vendor-openapi: the eight workload kinds the
// built-in field rule covers, and their subresources.
func Vendored() Provider {
	panic("unimplemented")
}

// Live returns a provider backed by the cluster's discovery and OpenAPI v3
// documents, fetched once per group version and cached. A failed fetch
// yields the unknown Node.
func Live(d discovery.DiscoveryInterface) Provider {
	panic("unimplemented")
}
