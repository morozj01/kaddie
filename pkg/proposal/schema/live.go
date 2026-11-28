package schema

import (
	"fmt"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/openapi3"
	"k8s.io/kube-openapi/pkg/spec3"
)

// Live returns a provider backed by the cluster's discovery and OpenAPI v3
// documents, fetched once per group version and cached. A failed fetch
// yields the unknown Node.
func Live(d discovery.DiscoveryInterface) Provider {
	return provider{src: newLiveSource(d)}
}

// liveSource asks a cluster. It keeps what it fetched, but not a failure, so
// a cluster that was briefly unreachable is asked again.
type liveSource struct {
	d discovery.DiscoveryInterface

	mu    sync.Mutex
	lists map[schema.GroupVersion]*metav1.APIResourceList
	docs  map[schema.GroupVersion]*spec3.OpenAPI
}

func newLiveSource(d discovery.DiscoveryInterface) *liveSource {
	return &liveSource{
		d:     d,
		lists: map[schema.GroupVersion]*metav1.APIResourceList{},
		docs:  map[schema.GroupVersion]*spec3.OpenAPI{},
	}
}

// resources returns what discovery lists for gv, subresources included under
// names such as "deployments/scale".
func (l *liveSource) resources(gv schema.GroupVersion) ([]metav1.APIResource, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	list, ok := l.lists[gv]
	if !ok {
		var err error
		if list, err = l.d.ServerResourcesForGroupVersion(gv.String()); err != nil {
			return nil, err
		}
		l.lists[gv] = list
	}
	return list.APIResources, nil
}

// entry returns the discovery entry of r, with Group and Version filled in
// from the list when the entry leaves them empty.
func (l *liveSource) entry(r Resource) (metav1.APIResource, error) {
	if r.Version == "" || r.Resource == "" {
		return metav1.APIResource{}, fmt.Errorf("resource %+v lacks a version or a name", r)
	}
	gv := schema.GroupVersion{Group: r.Group, Version: r.Version}
	rs, err := l.resources(gv)
	if err != nil {
		return metav1.APIResource{}, err
	}
	name := r.Resource
	if r.Subresource != "" {
		name += "/" + r.Subresource
	}
	for _, e := range rs {
		if e.Name == name {
			if e.Group == "" {
				e.Group = gv.Group
			}
			if e.Version == "" {
				e.Version = gv.Version
			}
			return e, nil
		}
	}
	return metav1.APIResource{}, fmt.Errorf("%s is not served in %s", name, gv)
}

func (l *liveSource) kind(r Resource) (schema.GroupVersionKind, error) {
	e, err := l.entry(r)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	return schema.GroupVersionKind{Group: e.Group, Version: e.Version, Kind: e.Kind}, nil
}

func (l *liveSource) document(gv schema.GroupVersion) (*spec3.OpenAPI, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if doc, ok := l.docs[gv]; ok {
		return doc, nil
	}
	doc, err := openapi3.NewRoot(l.d.OpenAPIV3()).GVSpec(gv)
	if err != nil {
		return nil, err
	}
	l.docs[gv] = doc
	return doc, nil
}
