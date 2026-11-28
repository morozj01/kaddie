// Package changes works out which fields a request changes: statically from
// the request alone (base §4.2.1 item 1, "when in doubt it changed"), and
// exactly from a dry run (item 2).
package changes

import (
	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/pkg/proposal/fieldpath"
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
// A body Static can't parse as its patch type requires changes the root.
func Static(verb v1alpha1.RequestVerb, pt v1alpha1.PatchType, body []byte, root schema.Node) []fieldpath.Change {
	panic("unimplemented")
}

// Diff returns the leaf fields that differ between two JSON documents: added,
// removed and changed scalars, empty objects and empty lists, list elements
// compared by index. A removed object or list contributes every leaf it held.
func Diff(before, after []byte) ([]fieldpath.Change, error) {
	panic("unimplemented")
}

// Apply applies a patch client-side, as the API server would before
// admission: JSON Patch and merge patch with the evanphx/json-patch rules the
// API server uses, strategic merge patch with root's merge keys.
func Apply(pt v1alpha1.PatchType, original, patch []byte, root schema.Node) ([]byte, error) {
	panic("unimplemented")
}
