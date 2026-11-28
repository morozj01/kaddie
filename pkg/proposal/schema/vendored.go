package schema

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kube-openapi/pkg/spec3"
)

// openapi holds what hack/vendor-openapi recorded: IndexFile and one pruned
// document per group version.
//
//go:embed openapi/*.json
var openapi embed.FS

// Vendored returns the provider of the embedded schemas, recorded from a
// 1.28 API server by hack/vendor-openapi: the eight workload kinds the
// built-in field rule covers, and their subresources.
func Vendored() Provider {
	return provider{src: &vendoredSource{
		files: openapi,
		docs:  map[schema.GroupVersion]*spec3.OpenAPI{},
	}}
}

// Recorded returns every resource and subresource Vendored knows, in the
// order of the index.
func Recorded() []Resource {
	var rs []Resource
	for _, e := range mustIndex(openapi) {
		rs = append(rs, e.Resource)
	}
	return rs
}

type vendoredSource struct {
	files embed.FS

	mu    sync.Mutex
	index map[Resource]schema.GroupVersionKind
	docs  map[schema.GroupVersion]*spec3.OpenAPI
}

func (v *vendoredSource) kind(r Resource) (schema.GroupVersionKind, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.index == nil {
		v.index = map[Resource]schema.GroupVersionKind{}
		for _, e := range mustIndex(v.files) {
			v.index[e.Resource] = e.Kind
		}
	}
	gvk, ok := v.index[r]
	if !ok {
		return schema.GroupVersionKind{}, fmt.Errorf("%s is not vendored", r.key())
	}
	return gvk, nil
}

func (v *vendoredSource) document(gv schema.GroupVersion) (*spec3.OpenAPI, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if doc, ok := v.docs[gv]; ok {
		return doc, nil
	}
	b, err := v.files.ReadFile("openapi/" + documentFile(gv))
	if err != nil {
		return nil, err
	}
	doc := &spec3.OpenAPI{}
	if err := json.Unmarshal(b, doc); err != nil {
		return nil, fmt.Errorf("vendored document of %s: %w", gv, err)
	}
	v.docs[gv] = doc
	return doc, nil
}

// mustIndex reads the index of the embedded files. The files are part of the
// binary, so a failure is a corrupt build, not an input.
func mustIndex(files embed.FS) []indexEntry {
	b, err := files.ReadFile("openapi/" + IndexFile)
	if err != nil {
		panic(err)
	}
	var idx struct {
		Resources []indexEntry `json:"resources"`
	}
	if err := json.Unmarshal(b, &idx); err != nil {
		panic(fmt.Errorf("vendored index: %w", err))
	}
	return idx.Resources
}
