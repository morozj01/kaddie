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
