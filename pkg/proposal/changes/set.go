package changes

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/morozj01/kaddie/pkg/proposal/fieldpath"
)

// changeSet collects changes in the order they are found, one per path. A
// Subtree change and a leaf change at the same path are one Subtree change.
type changeSet struct {
	out  []fieldpath.Change
	seen map[string]int
	// adds counts every add call, duplicates included.
	adds int
}

// add records a change at path. It copies path.
func (s *changeSet) add(path []fieldpath.Segment, subtree bool) {
	s.adds++
	key := pathKey(path)
	if i, ok := s.seen[key]; ok {
		s.out[i].Subtree = s.out[i].Subtree || subtree
		return
	}
	if s.seen == nil {
		s.seen = map[string]int{}
	}
	s.seen[key] = len(s.out)
	s.out = append(s.out, fieldpath.Change{Path: slices.Clone(path), Subtree: subtree})
}

// pathKey identifies a path exactly, telling a list element "0" from a map
// key "0".
func pathKey(path []fieldpath.Segment) string {
	var b strings.Builder
	for _, seg := range path {
		b.WriteString(strconv.Quote(seg.Key))
		if seg.Elem {
			b.WriteByte('#')
		}
		b.WriteByte('/')
	}
	return b.String()
}

// rootChange is the answer when a body can't be read as its patch type: the
// whole object may have changed.
func rootChange() []fieldpath.Change {
	return []fieldpath.Change{{Subtree: true}}
}

// leaves adds a leaf change for every scalar, empty object and empty list
// held by v, in sorted key order.
func (s *changeSet) leaves(path []fieldpath.Segment, v any) {
	switch v := v.(type) {
	case map[string]any:
		if len(v) == 0 {
			s.add(path, false)
		}
		for _, k := range slices.Sorted(maps.Keys(v)) {
			s.leaves(withKey(path, k), v[k])
		}
	case []any:
		if len(v) == 0 {
			s.add(path, false)
		}
		for i, e := range v {
			s.leaves(withIndex(path, i), e)
		}
	default:
		s.add(path, false)
	}
}

func (s *changeSet) diff(path []fieldpath.Segment, a, b any) {
	switch av := a.(type) {
	case map[string]any:
		if bv, ok := b.(map[string]any); ok {
			s.diffMaps(path, av, bv)
			return
		}
	case []any:
		if bv, ok := b.([]any); ok {
			s.diffLists(path, av, bv)
			return
		}
	default:
		if !isContainer(b) {
			if a != b {
				s.add(path, false)
			}
			return
		}
	}
	// The kind of value changed: everything either side held is a change.
	s.leaves(path, a)
	s.leaves(path, b)
}

func (s *changeSet) diffMaps(path []fieldpath.Segment, a, b map[string]any) {
	if len(a) > 0 && len(b) == 0 {
		s.add(path, false)
	}
	keys := slices.Sorted(maps.Keys(a))
	keys = append(keys, slices.Sorted(maps.Keys(b))...)
	slices.Sort(keys)
	for _, k := range slices.Compact(keys) {
		p := withKey(path, k)
		av, inA := a[k]
		bv, inB := b[k]
		switch {
		case !inB:
			s.leaves(p, av)
		case !inA:
			s.leaves(p, bv)
		default:
			s.diff(p, av, bv)
		}
	}
}

func (s *changeSet) diffLists(path []fieldpath.Segment, a, b []any) {
	if len(a) > 0 && len(b) == 0 {
		s.add(path, false)
	}
	for i := range max(len(a), len(b)) {
		p := withIndex(path, i)
		switch {
		case i >= len(b):
			s.leaves(p, a[i])
		case i >= len(a):
			s.leaves(p, b[i])
		default:
			s.diff(p, a[i], b[i])
		}
	}
}

func isContainer(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return true
	}
	return false
}

func withKey(path []fieldpath.Segment, key string) []fieldpath.Segment {
	return append(slices.Clip(path), fieldpath.Segment{Key: key})
}

func withIndex(path []fieldpath.Segment, i int) []fieldpath.Segment {
	return append(slices.Clip(path), fieldpath.Segment{Key: strconv.Itoa(i), Elem: true})
}

func withElem(path []fieldpath.Segment) []fieldpath.Segment {
	return append(slices.Clip(path), fieldpath.Segment{Elem: true})
}

// decode reads one JSON document, keeping numbers as written.
func decode(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing data after the JSON document")
	}
	return v, nil
}
