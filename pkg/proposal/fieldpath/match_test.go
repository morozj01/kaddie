package fieldpath

import (
	"slices"
	"testing"
)

// seg builds a concrete path from a compact spelling: a plain word is a
// field, a number or "-" is a list element (also possibly a map key, as a
// JSON Patch segment is), and "*" is a list element of unknown index.
func seg(parts ...string) []Segment {
	out := make([]Segment, len(parts))
	for i, p := range parts {
		switch {
		case p == "*":
			out[i] = Segment{Elem: true}
		case p == "-" || (p != "" && p[0] >= '0' && p[0] <= '9'):
			out[i] = Segment{Key: p, Elem: true}
		default:
			out[i] = Segment{Key: p}
		}
	}
	return out
}

func strs(paths []Path) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = p.String()
	}
	return out
}

// TestT38_AncestorsAndDescendants covers the engine part of T38: a rule
// covers a change at or beneath the rule's path, and a change above the
// rule's path relates to it through below, to be settled by the schema.
func TestT38_AncestorsAndDescendants(t *testing.T) {
	tests := []struct {
		name      string
		rule      string
		change    []Segment
		wantCover bool
		wantBelow []string
	}{
		{
			name:      "replacing spec.template relates to the service account rule through below",
			rule:      "spec.template.spec.serviceAccountName",
			change:    seg("spec", "template"),
			wantBelow: []string{"spec.serviceAccountName"},
		},
		{
			name:      "replacing the whole object relates to every rule through below",
			rule:      "spec.template.spec.serviceAccountName",
			change:    nil,
			wantBelow: []string{"spec.template.spec.serviceAccountName"},
		},
		{
			name:      "a change at the rule path is covered",
			rule:      "spec.template.spec.serviceAccountName",
			change:    seg("spec", "template", "spec", "serviceAccountName"),
			wantCover: true,
		},
		{
			name:      "a change beneath the rule path is covered",
			rule:      "spec.template.spec.volumes",
			change:    seg("spec", "template", "spec", "volumes", "0", "name"),
			wantCover: true,
		},
		{
			name:   "a sibling is unrelated",
			rule:   "spec.template.spec.serviceAccountName",
			change: seg("spec", "replicas"),
		},
		{
			name:   "a different field below the shared ancestor is unrelated",
			rule:   "spec.template.spec.serviceAccountName",
			change: seg("spec", "template", "spec", "containers"),
		},
		{
			name:      "any-depth image rule covers the Pod path",
			rule:      "**.containers[*].image",
			change:    seg("spec", "containers", "0", "image"),
			wantCover: true,
		},
		{
			name:      "any-depth image rule covers the Deployment path",
			rule:      "**.containers[*].image",
			change:    seg("spec", "template", "spec", "containers", "0", "image"),
			wantCover: true,
		},
		{
			name:      "any-depth image rule covers the CronJob path",
			rule:      "**.containers[*].image",
			change:    seg("spec", "jobTemplate", "spec", "template", "spec", "containers", "0", "image"),
			wantCover: true,
		},
		{
			name:      "a JSON Patch append segment matches [*]",
			rule:      "**.containers[*].image",
			change:    seg("spec", "containers", "-", "image"),
			wantCover: true,
		},
		{
			name:      "an unknown-index element matches [*]",
			rule:      "**.containers[*].image",
			change:    seg("spec", "containers", "*", "image"),
			wantCover: true,
		},
		{
			name:      "a change beneath the matched run is covered",
			rule:      "**.containers[*].image",
			change:    seg("spec", "containers", "0", "image", "x"),
			wantCover: true,
		},
		{
			name:      "another container field is covered by nothing; the rule only reaches below it",
			rule:      "**.containers[*].image",
			change:    seg("spec", "containers", "0", "name"),
			wantBelow: []string{"**.containers[*].image"},
		},
		{
			name:      "a map key does not match [*]",
			rule:      "**.containers[*].image",
			change:    seg("spec", "containers", "web", "image"),
			wantBelow: []string{"**.containers[*].image"},
		},
		{
			name:      "replacing one container relates to the image rule through its remainder",
			rule:      "**.containers[*].image",
			change:    seg("spec", "containers", "0"),
			wantBelow: []string{"image", "**.containers[*].image"},
		},
		{
			name:      "replacing the containers list relates through its remainder",
			rule:      "**.containers[*].image",
			change:    seg("spec", "containers"),
			wantBelow: []string{"[*].image", "**.containers[*].image"},
		},
		{
			name:      "replacing spec relates only through the rule itself",
			rule:      "**.serviceAccountName",
			change:    seg("spec"),
			wantBelow: []string{"**.serviceAccountName"},
		},
		{
			name:      "replacing the root relates through the rule itself and never covers",
			rule:      "**.serviceAccountName",
			change:    nil,
			wantBelow: []string{"**.serviceAccountName"},
		},
		{
			name:      "an alignment is found after a false start",
			rule:      "**.a.b",
			change:    seg("x", "a", "a", "b"),
			wantCover: true,
		},
		{
			name:      "a false start that runs past the end keeps its remainder",
			rule:      "**.a.b",
			change:    seg("a", "a"),
			wantBelow: []string{"b", "**.a.b"},
		},
		{
			name:      "an exact name step matches a numeric JSON Patch segment (map key)",
			rule:      "data.0",
			change:    seg("data", "0"),
			wantCover: true,
		},
		{
			name:      "a key with dots matches by key",
			rule:      `metadata.annotations["example.com/owner"]`,
			change:    []Segment{{Key: "metadata"}, {Key: "annotations"}, {Key: "example.com/owner"}},
			wantCover: true,
		},
		{
			name:      "the rule on metadata.annotations covers one annotation",
			rule:      "metadata.annotations",
			change:    []Segment{{Key: "metadata"}, {Key: "annotations"}, {Key: "a.b/c"}},
			wantCover: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			covers, below := Relate(MustParse(tc.rule), tc.change)
			if covers != tc.wantCover {
				t.Errorf("covers = %v, want %v", covers, tc.wantCover)
			}
			if got := strs(below); !slices.Equal(got, tc.wantBelow) && (len(got) != 0 || len(tc.wantBelow) != 0) {
				t.Errorf("below = %v, want %v", got, tc.wantBelow)
			}
		})
	}
}

func TestRelateRemainderIsAnchored(t *testing.T) {
	_, below := Relate(MustParse("**.containers[*].image"), seg("spec", "containers", "0"))
	for _, b := range below {
		if b.String() == "image" && b.AnyDepth {
			t.Errorf("remainder %q must be anchored at the changed path", b)
		}
	}
}

func TestRelateEmptyRule(t *testing.T) {
	if covers, below := Relate(Path{}, seg("spec")); covers || len(below) != 0 {
		t.Errorf("an empty rule relates to nothing, got covers=%v below=%v", covers, below)
	}
}
