// Package fieldpath parses the field paths of base §4.2.1 and relates a rule
// path to a changed field.
package fieldpath

// Step is one step of a rule path.
type Step struct {
	// Name is a field name or map key. Empty when AnyElem is set.
	Name string
	// AnyElem is [*]: any list element.
	AnyElem bool
}

// Path is a parsed rule path such as spec.template.spec.serviceAccountName,
// metadata.annotations["example.com/owner"] or **.containers[*].image.
type Path struct {
	// AnyDepth is a leading **.: Steps may start at any depth.
	AnyDepth bool
	Steps    []Step
}

// Parse parses the path syntax of base §4.2.1: dot-separated names, [*] for
// any list element, ["key"] for a key containing dots or slashes, and an
// optional leading **.
func Parse(s string) (Path, error) {
	panic("unimplemented")
}

// MustParse is Parse for the compiled-in catalogue; it panics on error.
func MustParse(s string) Path {
	panic("unimplemented")
}

// String renders p back in the syntax Parse accepts.
func (p Path) String() string {
	panic("unimplemented")
}

// Segment is one step of a concrete changed path.
type Segment struct {
	// Key is a field name or map key. For a list element it holds the index
	// text when the analysis knows it ("0", or "-" from a JSON Patch append).
	Key string
	// Elem marks a list element. A JSON Patch segment that is a number or "-"
	// may address a list element or a map key, so it sets both Key and Elem.
	Elem bool
}

// Change is one field a request changes.
type Change struct {
	// Path is the changed field, from the object root. Empty is the root.
	Path []Segment
	// Subtree means everything beneath Path may have changed too: a value
	// replaced or removed whose old content the analysis can't see.
	Subtree bool
}

// Format renders a concrete path for findings, for example
// spec.template.spec.containers[0].image or metadata.labels["a.b/c"].
func Format(path []Segment) string {
	panic("unimplemented")
}

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
	panic("unimplemented")
}
