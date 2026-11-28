package schema

import (
	"fmt"
	"slices"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kube-openapi/pkg/spec3"
)

// source is where a provider learns what kind a resource carries and where
// that kind's group version document lives. Live and Vendored differ only
// here; the rest of the resolution is shared.
type source interface {
	// kind returns the kind of the body of a request on r.
	kind(r Resource) (schema.GroupVersionKind, error)
	// document returns the OpenAPI v3 document of a group version.
	document(gv schema.GroupVersion) (*spec3.OpenAPI, error)
}

// provider resolves a Resource to a Node through a source.
type provider struct{ src source }

func (p provider) Root(r Resource) Node {
	n, err := resolve(p.src, r)
	if err != nil {
		return Node{}
	}
	return n
}

func resolve(src source, r Resource) (Node, error) {
	gvk, err := src.kind(r)
	if err != nil {
		return Node{}, err
	}
	n, _, err := kindNode(src, schema.GroupVersion{Group: r.Group, Version: r.Version}, gvk)
	return n, err
}

// kindNode returns the root Node of kind gvk in the document of gv, and the
// name of the component that holds it. The document is the one of the group
// version the request addresses, not of the kind's own: the API server
// publishes the autoscaling/v1 Scale of deployments/scale inside apps/v1.
func kindNode(src source, gv schema.GroupVersion, gvk schema.GroupVersionKind) (Node, string, error) {
	doc, err := src.document(gv)
	if err != nil {
		return Node{}, "", err
	}
	if doc.Components == nil {
		return Node{}, "", fmt.Errorf("document of %s has no components", gv)
	}
	for name, s := range doc.Components.Schemas {
		if s != nil && hasGVK(s.Extensions[extGVK], gvk) {
			return Node{schemas: doc.Components.Schemas, s: s}, name, nil
		}
	}
	return Node{}, "", fmt.Errorf("document of %s has no schema for %s", gv, gvk)
}

// hasGVK reports whether an x-kubernetes-group-version-kind extension value,
// a list of {group, version, kind} objects, names gvk.
func hasGVK(ext any, gvk schema.GroupVersionKind) bool {
	list, _ := ext.([]any)
	return slices.ContainsFunc(list, func(e any) bool {
		m, _ := e.(map[string]any)
		g, _ := m["group"].(string)
		v, _ := m["version"].(string)
		k, _ := m["kind"].(string)
		return g == gvk.Group && v == gvk.Version && k == gvk.Kind
	})
}
