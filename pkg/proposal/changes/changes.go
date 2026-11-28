// Package changes works out which fields a request changes: statically from
// the request alone (base §4.2.1 item 1, "when in doubt it changed"), and
// exactly from a dry run (item 2).
package changes

import (
	"fmt"

	jsonpatch "gopkg.in/evanphx/json-patch.v4"
	"k8s.io/apimachinery/pkg/util/strategicpatch"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/pkg/proposal/fieldpath"
	"github.com/morozj01/kaddie/pkg/proposal/internal/jsondoc"
	"github.com/morozj01/kaddie/pkg/proposal/schema"
)

// Static returns the fields a request may change, from its verb, patch type
// and body alone. root is the schema of the body (schema.Provider.Root).
//
//   - delete changes nothing.
//   - create changes every field present: one leaf Change per scalar, empty
//     object and empty list, list elements as Elem segments.
//   - JSON Patch changes each operation's path, and from for move and copy,
//     each as a Subtree Change. test operations change nothing.
//   - JSON merge patch changes every field present; a list or null value is
//     a Subtree Change, and a body that isn't an object changes the root.
//   - strategic merge patch is a merge patch walked alongside root: a list
//     whose patch strategy is merge and has a merge key is matched element by
//     element, the merge key identifying the element without counting as a
//     change; a primitive list with strategy merge adds leaf elements; any
//     other list, and any list under an unknown schema, is a Subtree Change.
//     $patch: replace or delete, $retainKeys and every other $ directive make
//     the enclosing object a Subtree Change; $setElementOrder/<f> and
//     $deleteFromPrimitiveList/<f> make field f one.
//
// A body Static can't parse as its patch type, or that repeats an object key
// anywhere (the server and a map-based reader would keep different values),
// changes the root, as a Subtree Change with an empty Path. A strategic merge
// patch element that holds only its merge key, or an object that is empty, is
// a leaf Change at that element or object, since it still adds one. A
// strategic merge list element has no known index, so its segment is
// {Elem: true} with an empty Key.
//
// The result holds one Change per path, a Subtree Change winning over a leaf
// at the same path, in the order found: map keys sorted, list elements and
// patch operations in order.
func Static(verb v1alpha1.RequestVerb, pt v1alpha1.PatchType, body []byte, root schema.Node) []fieldpath.Change {
	var set changeSet
	switch verb {
	case v1alpha1.RequestVerbDelete:
		return nil
	case v1alpha1.RequestVerbCreate:
		doc, err := jsondoc.Decode(body)
		obj, ok := doc.(map[string]any)
		if err != nil || !ok {
			return rootChange()
		}
		set.leaves(nil, obj)
	case v1alpha1.RequestVerbPatch:
		if !staticPatch(&set, pt, body, root) {
			return rootChange()
		}
	default:
		return rootChange()
	}
	return set.out
}

// Diff returns the leaf fields that differ between two JSON documents: added,
// removed and changed scalars, empty objects and empty lists, list elements
// compared by index. A removed object or list contributes every leaf it held.
func Diff(before, after []byte) ([]fieldpath.Change, error) {
	a, err := jsondoc.Decode(before)
	if err != nil {
		return nil, fmt.Errorf("before: %w", err)
	}
	b, err := jsondoc.Decode(after)
	if err != nil {
		return nil, fmt.Errorf("after: %w", err)
	}
	var set changeSet
	set.diff(nil, a, b)
	return set.out, nil
}

// Apply applies a patch client-side, as the API server would before
// admission: JSON Patch and merge patch with the evanphx/json-patch rules the
// API server uses, strategic merge patch with root's merge keys.
func Apply(pt v1alpha1.PatchType, original, patch []byte, root schema.Node) ([]byte, error) {
	switch pt {
	case v1alpha1.PatchTypeJSONPatch:
		p, err := jsonpatch.DecodePatch(patch)
		if err != nil {
			return nil, fmt.Errorf("decode JSON patch: %w", err)
		}
		return p.Apply(original)
	case v1alpha1.PatchTypeMergePatch:
		return jsonpatch.MergePatch(original, patch)
	case v1alpha1.PatchTypeStrategicMergePatch:
		return strategicpatch.StrategicMergePatchUsingLookupPatchMeta(original, patch, root.PatchMeta())
	}
	return nil, fmt.Errorf("unsupported patch type %q", pt)
}
