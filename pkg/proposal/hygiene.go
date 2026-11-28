package proposal

import (
	"encoding/json"
	"strings"
	"unicode"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
)

// redactedMarker starts every placeholder the redactor writes in place of a
// secret value.
const redactedMarker = "<redacted:"

// Fixed explanations of the body hygiene findings. They never quote the body.
const (
	invalidJSONExplanation = "The body is not valid JSON of the kind this verb and patch type require. The API server would reject it, or an executor could read it differently from what is shown."
	redactedExplanation    = "The body contains a redaction placeholder. If the request were executed, the placeholder text would be written in place of the real value."
	invisibleExplanation   = "The body contains an invisible or control character, such as a zero-width space or a byte-order mark. It can make a name or value look different from what it is."
)

// hygieneFindings checks one request's body (base §6.2): its JSON, redaction
// placeholders and invisible characters. Each reason is reported at most once
// per request, in that order. It runs on every request, whatever the target.
func hygieneFindings(index int, req v1alpha1.RequestFields) []Finding {
	var doc any
	decoded := json.Unmarshal([]byte(req.Body), &doc) == nil

	var out []Finding
	add := func(reason Reason, explanation string) {
		out = append(out, Finding{Reason: reason, Request: index, Explanation: explanation})
	}
	// Only create and patch need a body. A verb Malformed already reports
	// isn't held to one either.
	bodyNeeded := req.Verb == v1alpha1.RequestVerbCreate || req.Verb == v1alpha1.RequestVerbPatch
	if valid := (decoded && bodyShapeOK(req, doc)) || (req.Body == "" && !bodyNeeded); !valid {
		add(InvalidJSON, invalidJSONExplanation)
	}
	redacted, invisible := false, strings.ContainsFunc(req.Body, isInvisible)
	if decoded {
		walkStrings(doc, func(s string) {
			redacted = redacted || strings.Contains(s, redactedMarker)
			invisible = invisible || strings.ContainsFunc(s, isInvisible)
		})
	}
	if redacted {
		add(RedactedPlaceholder, redactedExplanation)
	}
	if invisible {
		add(InvisibleCharacter, invisibleExplanation)
	}
	return out
}

// isInvisible reports whether r is a Unicode format (Cf) or private use (Co)
// character. A leading byte-order mark is a Cf character.
func isInvisible(r rune) bool {
	return unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Co, r)
}

// bodyShapeOK reports whether doc, the decoded body, is the kind of JSON the
// request's verb and patch type require. A patch with a missing or unknown
// patch type is only held to being JSON; Malformed reports the type.
func bodyShapeOK(req v1alpha1.RequestFields, doc any) bool {
	_, isObject := doc.(map[string]any)
	switch req.Verb {
	case v1alpha1.RequestVerbCreate, v1alpha1.RequestVerbDelete:
		return isObject
	case v1alpha1.RequestVerbPatch:
		switch req.PatchType {
		case v1alpha1.PatchTypeMergePatch, v1alpha1.PatchTypeStrategicMergePatch:
			return isObject
		case v1alpha1.PatchTypeJSONPatch:
			return jsonPatchOK(doc)
		}
	}
	return true
}

// jsonPatchOK reports whether doc is an RFC 6902 patch: an array of objects,
// each with a known op, a path that is a JSON pointer, a value where the op
// needs one, and a from that is a JSON pointer for move and copy.
func jsonPatchOK(doc any) bool {
	ops, ok := doc.([]any)
	if !ok {
		return false
	}
	for _, o := range ops {
		op, ok := o.(map[string]any)
		if !ok {
			return false
		}
		name, _ := op["op"].(string)
		if !pointerField(op, "path") {
			return false
		}
		switch name {
		case "add", "replace", "test":
			if _, ok := op["value"]; !ok {
				return false
			}
		case "move", "copy":
			if !pointerField(op, "from") {
				return false
			}
		case "remove":
		default:
			return false
		}
	}
	return true
}

// pointerField reports whether op[key] is a string holding a JSON pointer
// (RFC 6901): empty, or starting with / and using ~ only as ~0 or ~1.
func pointerField(op map[string]any, key string) bool {
	ptr, ok := op[key].(string)
	if !ok || (ptr != "" && ptr[0] != '/') {
		return false
	}
	for i := 0; i < len(ptr); i++ {
		if ptr[i] == '~' && (i+1 == len(ptr) || (ptr[i+1] != '0' && ptr[i+1] != '1')) {
			return false
		}
	}
	return true
}

// walkStrings calls f with every key and string value in a decoded document.
func walkStrings(v any, f func(string)) {
	switch v := v.(type) {
	case string:
		f(v)
	case []any:
		for _, e := range v {
			walkStrings(e, f)
		}
	case map[string]any:
		for k, e := range v {
			f(k)
			walkStrings(e, f)
		}
	}
}
