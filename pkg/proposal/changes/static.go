package changes

import (
	"maps"
	"slices"
	"strings"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/pkg/proposal/fieldpath"
	"github.com/morozj01/kaddie/pkg/proposal/schema"
)

const (
	setElementOrderPrefix = "$setElementOrder/"
	deleteFromListPrefix  = "$deleteFromPrimitiveList/"
)

// staticPatch adds the changes of a patch body and reports whether the body
// parsed as its patch type.
func staticPatch(s *changeSet, pt v1alpha1.PatchType, body []byte, root schema.Node) bool {
	doc, err := decode(body)
	if err != nil {
		return false
	}
	switch pt {
	case v1alpha1.PatchTypeJSONPatch:
		ops, ok := doc.([]any)
		return ok && s.jsonPatch(ops)
	case v1alpha1.PatchTypeMergePatch:
		obj, ok := doc.(map[string]any)
		if ok {
			s.mergePatch(nil, obj)
		}
		return ok
	case v1alpha1.PatchTypeStrategicMergePatch:
		obj, ok := doc.(map[string]any)
		if ok {
			s.strategicPatch(nil, obj, root, "")
		}
		return ok
	}
	return false
}

// jsonPatch adds the paths each operation touches. It reports false when an
// operation is not a well-formed RFC 6902 operation.
func (s *changeSet) jsonPatch(ops []any) bool {
	for _, o := range ops {
		op, ok := o.(map[string]any)
		if !ok {
			return false
		}
		name, _ := op["op"].(string)
		fields := []string{"path"}
		switch name {
		case "test":
			continue
		case "add", "remove", "replace":
		case "move", "copy":
			fields = append(fields, "from")
		default:
			return false
		}
		for _, field := range fields {
			ptr, ok := op[field].(string)
			if !ok {
				return false
			}
			segs, ok := parsePointer(ptr)
			if !ok {
				return false
			}
			s.add(segs, true)
		}
	}
	return true
}

var pointerUnescape = strings.NewReplacer("~1", "/", "~0", "~")

// parsePointer reads an RFC 6901 JSON pointer. A segment that is all digits
// or "-" may address a list element or a map key, so it is both.
func parsePointer(ptr string) ([]fieldpath.Segment, bool) {
	if ptr == "" {
		return nil, true
	}
	rest, ok := strings.CutPrefix(ptr, "/")
	if !ok {
		return nil, false
	}
	var segs []fieldpath.Segment
	for raw := range strings.SplitSeq(rest, "/") {
		key := pointerUnescape.Replace(raw)
		segs = append(segs, fieldpath.Segment{Key: key, Elem: isIndex(key)})
	}
	return segs, true
}

func isIndex(s string) bool {
	if s == "-" {
		return true
	}
	return s != "" && !strings.ContainsFunc(s, func(r rune) bool { return r < '0' || r > '9' })
}

// mergePatch adds every field of an RFC 7386 merge patch object.
func (s *changeSet) mergePatch(path []fieldpath.Segment, obj map[string]any) {
	if len(obj) == 0 {
		s.add(path, false)
	}
	for _, k := range slices.Sorted(maps.Keys(obj)) {
		p := withKey(path, k)
		switch v := obj[k].(type) {
		case map[string]any:
			s.mergePatch(p, v)
		case []any, nil:
			s.add(p, true)
		default:
			s.add(p, false)
		}
	}
}

// strategicPatch adds the fields of a strategic merge patch object whose
// schema is n. mergeKey, when set, is the field identifying the list element
// obj is, which doesn't count as a change.
func (s *changeSet) strategicPatch(path []fieldpath.Segment, obj map[string]any, n schema.Node, mergeKey string) {
	before := s.adds
	keys := slices.Sorted(maps.Keys(obj))
	for _, k := range keys {
		if !strings.HasPrefix(k, "$") {
			continue
		}
		field, ok := strings.CutPrefix(k, setElementOrderPrefix)
		if !ok {
			field, ok = strings.CutPrefix(k, deleteFromListPrefix)
		}
		if !ok || field == "" {
			// $patch, $retainKeys and any other directive: the object itself.
			s.add(path, true)
			return
		}
		s.add(withKey(path, field), true)
	}
	for _, k := range keys {
		if strings.HasPrefix(k, "$") || k == mergeKey {
			continue
		}
		p := withKey(path, k)
		child := n.Field(k)
		switch v := obj[k].(type) {
		case map[string]any:
			s.strategicPatch(p, v, child, "")
		case []any:
			s.strategicList(p, v, child)
		case nil:
			s.add(p, true)
		default:
			s.add(p, false)
		}
	}
	if s.adds == before {
		// Empty, or only a merge key: an empty object is created or an
		// element still appears.
		s.add(path, false)
	}
}

// strategicList adds the changes of a list value whose schema is n.
func (s *changeSet) strategicList(path []fieldpath.Segment, list []any, n schema.Node) {
	merge, key := n.ListMerge()
	if !merge {
		s.add(path, true)
		return
	}
	if len(list) == 0 {
		s.add(path, false)
		return
	}
	for _, e := range list {
		switch e := e.(type) {
		case map[string]any:
			if key == "" || e["$patch"] == "replace" {
				s.add(path, true)
				return
			}
			s.strategicPatch(withElem(path), e, n.Elem(), key)
		case []any, nil:
			s.add(path, true)
			return
		default:
			if key != "" {
				s.add(path, true)
				return
			}
			s.add(withElem(path), false)
		}
	}
}
