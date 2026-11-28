package fieldpath

import (
	"slices"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		in   string
		want Path
	}{
		{"spec.replicas", Path{Steps: []Step{{Name: "spec"}, {Name: "replicas"}}}},
		{"metadata", Path{Steps: []Step{{Name: "metadata"}}}},
		{"**.containers[*].image", Path{AnyDepth: true, Steps: []Step{{Name: "containers"}, {AnyElem: true}, {Name: "image"}}}},
		{"**.serviceAccountName", Path{AnyDepth: true, Steps: []Step{{Name: "serviceAccountName"}}}},
		{`metadata.annotations["example.com/owner"]`, Path{Steps: []Step{{Name: "metadata"}, {Name: "annotations"}, {Name: "example.com/owner"}}}},
		{`metadata.labels["a.b"].x`, Path{Steps: []Step{{Name: "metadata"}, {Name: "labels"}, {Name: "a.b"}, {Name: "x"}}}},
		{`a[*][*].b`, Path{Steps: []Step{{Name: "a"}, {AnyElem: true}, {AnyElem: true}, {Name: "b"}}}},
		{`a["q\"uote"]`, Path{Steps: []Step{{Name: "a"}, {Name: `q"uote`}}}},
		{`["a.b"].c`, Path{Steps: []Step{{Name: "a.b"}, {Name: "c"}}}},
	}
	for _, tc := range tests {
		got, err := Parse(tc.in)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tc.in, err)
			continue
		}
		if got.AnyDepth != tc.want.AnyDepth || !slices.Equal(got.Steps, tc.want.Steps) {
			t.Errorf("Parse(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{
		"",
		".",
		"a.",
		".a",
		"a..b",
		"**",
		"**.",
		"a.**.b",
		"a.**",
		"**.**.a",
		"***.a",
		"*",
		"a*b",
		"a[",
		"a[*",
		"a[]",
		"a[0]",
		"a[x]",
		"a]",
		"a[*]]",
		`a["x`,
		`a["x"`,
		`a["x]`,
		`a["x"y]`,
		`a[""]`,
		`a["\q"]`,
		`a"b"`,
		`a.["b"]`,
		`a[*]b`,
	} {
		if p, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) = %+v, want an error", in, p)
		}
	}
}

func TestStringRoundTrip(t *testing.T) {
	for _, in := range []string{
		"spec.replicas",
		"**.containers[*].image",
		"**.ephemeralContainers[*]",
		`metadata.annotations["example.com/owner"]`,
		`metadata.labels["a.b/c"]`,
		`**.["x.y"].z`,
		`a[*][*]`,
		`a["q\"uote"]`,
		`a["back\\slash"]`,
	} {
		p, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if got := p.String(); got != in {
			t.Errorf("Parse(%q).String() = %q", in, got)
		}
		again, err := Parse(p.String())
		if err != nil || again.AnyDepth != p.AnyDepth || !slices.Equal(again.Steps, p.Steps) {
			t.Errorf("round trip of %q gave %+v, %v; want %+v", in, again, err, p)
		}
	}
}

func TestMustParsePanicsOnError(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustParse(\"a..b\") did not panic")
		}
	}()
	MustParse("a..b")
}

func TestFormat(t *testing.T) {
	tests := []struct {
		name string
		path []Segment
		want string
	}{
		{"root", nil, ""},
		{"plain", []Segment{{Key: "spec"}, {Key: "replicas"}}, "spec.replicas"},
		{"list element", []Segment{{Key: "spec"}, {Key: "containers"}, {Key: "0", Elem: true}, {Key: "image"}}, "spec.containers[0].image"},
		{"append", []Segment{{Key: "spec"}, {Key: "containers"}, {Key: "-", Elem: true}}, "spec.containers[-]"},
		{"unknown index", []Segment{{Key: "spec"}, {Key: "containers"}, {Elem: true}}, "spec.containers[*]"},
		{"key with dot and slash", []Segment{{Key: "metadata"}, {Key: "labels"}, {Key: "a.b/c"}}, `metadata.labels["a.b/c"]`},
		{"key with slash", []Segment{{Key: "metadata"}, {Key: "annotations"}, {Key: "example.com/owner"}}, `metadata.annotations["example.com/owner"]`},
		{"key with quote", []Segment{{Key: "data"}, {Key: `a"b`}}, `data["a\"b"]`},
		{"first key needs quoting", []Segment{{Key: "a.b"}, {Key: "c"}}, `["a.b"].c`},
		{"empty key", []Segment{{Key: "data"}, {Key: ""}}, `data[""]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Format(tc.path); got != tc.want {
				t.Errorf("Format = %q, want %q", got, tc.want)
			}
		})
	}
}
