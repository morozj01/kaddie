package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

// indexFile names the file Record writes beside the documents: which kind
// each recorded resource and subresource carries.
const indexFile = "index.json"

// kept lists the extensions a recorded schema keeps. Everything else the
// field analysis never reads is dropped, descriptions first.
var kept = []string{extPatchMergeKey, extPatchStrategy, extPreserve, extGVK}

// Record resolves each resource, and each of its writable subresources whose
// kind is the resource's own or Scale, through d, the code path Live takes.
// It returns the files Vendored serves, by name: an index of every recorded
// resource to its kind, and one pruned OpenAPI v3 document per group version
// holding the recorded kinds and the components reachable from them. The
// bytes are deterministic, so recording the same server again is a no-op.
func Record(d discovery.DiscoveryInterface, resources []Resource) (map[string][]byte, error) {
	src := newLiveSource(d)
	var entries []indexEntry
	roots := map[schema.GroupVersion]map[string]bool{}
	for _, r := range resources {
		targets, err := recordTargets(src, r)
		if err != nil {
			return nil, err
		}
		gv := schema.GroupVersion{Group: r.Group, Version: r.Version}
		for _, t := range targets {
			gvk, err := src.kind(t)
			if err != nil {
				return nil, err
			}
			_, name, err := kindNode(src, gv, gvk)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", t.key(), err)
			}
			if roots[gv] == nil {
				roots[gv] = map[string]bool{}
			}
			roots[gv][name] = true
			entries = append(entries, indexEntry{Resource: t, Kind: gvk})
		}
	}

	files := map[string][]byte{}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Resource.key() < entries[j].Resource.key() })
	b, err := marshal(map[string]any{"resources": entries})
	if err != nil {
		return nil, err
	}
	files[indexFile] = b

	for gv, names := range roots {
		doc, err := src.document(gv)
		if err != nil {
			return nil, err
		}
		schemas, err := reachable(doc.Components.Schemas, names)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", gv, err)
		}
		b, err := marshal(map[string]any{
			"openapi":    "3.0.0",
			"components": map[string]any{"schemas": schemas},
		})
		if err != nil {
			return nil, err
		}
		files[documentFile(gv)] = b
	}
	return files, nil
}

func recordTargets(src *liveSource, r Resource) ([]Resource, error) {
	own, err := src.entry(r)
	if err != nil {
		return nil, err
	}
	all, err := src.resources(schema.GroupVersion{Group: r.Group, Version: r.Version})
	if err != nil {
		return nil, err
	}
	targets := []Resource{r}
	for _, e := range all {
		sub, ok := strings.CutPrefix(e.Name, r.Resource+"/")
		if !ok || !writable(e) || (e.Kind != own.Kind && e.Kind != "Scale") {
			continue
		}
		targets = append(targets, Resource{Group: r.Group, Version: r.Version, Resource: r.Resource, Subresource: sub})
	}
	return targets, nil
}

// writable reports whether a request can carry a body to e. pods/log is
// listed as kind Pod but is only ever read.
func writable(e metav1.APIResource) bool {
	return slices.ContainsFunc(e.Verbs, func(v string) bool {
		return v == "create" || v == "update" || v == "patch" || v == "delete"
	})
}

// reachable prunes the schemas named by roots and every schema they refer to.
func reachable(all map[string]*spec.Schema, roots map[string]bool) (map[string]*spec.Schema, error) {
	out := map[string]*spec.Schema{}
	queue := make([]string, 0, len(roots))
	for name := range roots {
		queue = append(queue, name)
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if _, done := out[name]; done {
			continue
		}
		s, ok := all[name]
		if !ok {
			return nil, fmt.Errorf("reference to missing schema %s", name)
		}
		p, refs := prune(s)
		out[name] = p
		for _, ref := range refs {
			dep, ok := strings.CutPrefix(ref, refPrefix)
			if !ok {
				return nil, fmt.Errorf("%s: unsupported reference %s", name, ref)
			}
			queue = append(queue, dep)
		}
	}
	return out, nil
}

// prune copies s keeping only its structure and the extensions in kept, and
// returns the references the copy holds.
func prune(s *spec.Schema) (*spec.Schema, []string) {
	var refs []string
	var walk func(in *spec.Schema) *spec.Schema
	walkAll := func(in []spec.Schema) []spec.Schema {
		var out []spec.Schema
		for i := range in {
			out = append(out, *walk(&in[i]))
		}
		return out
	}
	walk = func(in *spec.Schema) *spec.Schema {
		out := &spec.Schema{}
		out.Type = in.Type
		out.Ref = in.Ref
		if r := in.Ref.String(); r != "" {
			refs = append(refs, r)
		}
		if len(in.Properties) > 0 {
			out.Properties = map[string]spec.Schema{}
			for k := range in.Properties {
				p := in.Properties[k]
				out.Properties[k] = *walk(&p)
			}
		}
		if in.Items != nil && in.Items.Schema != nil {
			out.Items = &spec.SchemaOrArray{Schema: walk(in.Items.Schema)}
		}
		if in.AdditionalProperties != nil {
			out.AdditionalProperties = &spec.SchemaOrBool{Allows: in.AdditionalProperties.Allows}
			if in.AdditionalProperties.Schema != nil {
				out.AdditionalProperties.Schema = walk(in.AdditionalProperties.Schema)
			}
		}
		out.AllOf = walkAll(in.AllOf)
		out.OneOf = walkAll(in.OneOf)
		out.AnyOf = walkAll(in.AnyOf)
		for _, k := range kept {
			if v, ok := in.Extensions[k]; ok {
				out.AddExtension(k, v)
			}
		}
		return out
	}
	return walk(s), refs
}

// marshal renders v as indented JSON with sorted keys and a final newline.
func marshal(v any) ([]byte, error) {
	compact, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var generic any
	if err := json.Unmarshal(compact, &generic); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(generic); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func documentFile(gv schema.GroupVersion) string {
	g := gv.Group
	if g == "" {
		g = "core"
	}
	return g + "_" + gv.Version + ".json"
}

func (r Resource) key() string {
	return strings.Join([]string{r.Group, r.Version, r.Resource, r.Subresource}, "/")
}

// indexEntry is one line of the index: a resource and the kind of its body.
type indexEntry struct {
	Resource Resource
	Kind     schema.GroupVersionKind
}

type indexJSON struct {
	Group       string   `json:"group,omitempty"`
	Version     string   `json:"version"`
	Resource    string   `json:"resource"`
	Subresource string   `json:"subresource,omitempty"`
	Kind        kindJSON `json:"kind"`
}

type kindJSON struct {
	Group   string `json:"group,omitempty"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

func (e indexEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal(indexJSON{
		Group:       e.Resource.Group,
		Version:     e.Resource.Version,
		Resource:    e.Resource.Resource,
		Subresource: e.Resource.Subresource,
		Kind:        kindJSON{Group: e.Kind.Group, Version: e.Kind.Version, Kind: e.Kind.Kind},
	})
}

func (e *indexEntry) UnmarshalJSON(b []byte) error {
	var j indexJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	e.Resource = Resource{Group: j.Group, Version: j.Version, Resource: j.Resource, Subresource: j.Subresource}
	e.Kind = schema.GroupVersionKind{Group: j.Kind.Group, Version: j.Kind.Version, Kind: j.Kind.Kind}
	return nil
}
