package fieldpath

import "slices"

// Relate reports how rule path r relates to changed path c.
//
// covers is true when r names c or one of its ancestors: the rule matches
// whatever kind of change c is.
//
// below holds the rule paths, relative to c, that would name a descendant of
// c. They match only a Subtree change, and only when one of them can exist
// beneath c in the object's schema (schema.Node.Reaches).
//
// An AnyDepth rule covers c when its steps align with a run of c's segments
// that ends at or before c's end; aligning with an empty run never counts,
// or **. would cover everything. Its below always includes the rule itself
// (it may start anywhere beneath c), plus the remainder of every alignment
// that starts on a segment of c and runs past c's end. So a Subtree change to
// spec.template matches **.serviceAccountName through the schema, while
// spec.replicas, an integer, reaches nothing.
func Relate(r Path, c []Segment) (covers bool, below []Path) {
	n := len(r.Steps)
	if n == 0 {
		return false, nil
	}
	starts := []int{0}
	if r.AnyDepth {
		starts = make([]int, len(c))
		for i := range starts {
			starts[i] = i
		}
	}
	for _, i := range starts {
		// Compare as much of the rule as fits inside c from segment i.
		run := min(n, len(c)-i)
		if !alignsWith(r.Steps[:run], c[i:i+run]) {
			continue
		}
		if run == n {
			return true, nil
		}
		below = appendUnique(below, Path{Steps: r.Steps[run:]})
	}
	if r.AnyDepth {
		below = appendUnique(below, r)
	}
	return false, below
}

// alignsWith reports whether each step matches the segment at its position.
func alignsWith(steps []Step, segs []Segment) bool {
	for i, st := range steps {
		if st.AnyElem {
			if !segs[i].Elem {
				return false
			}
		} else if segs[i].Key != st.Name {
			return false
		}
	}
	return true
}

func appendUnique(paths []Path, p Path) []Path {
	if slices.ContainsFunc(paths, func(q Path) bool {
		return q.AnyDepth == p.AnyDepth && slices.Equal(q.Steps, p.Steps)
	}) {
		return paths
	}
	return append(paths, p)
}
