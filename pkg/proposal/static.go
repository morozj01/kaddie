package proposal

import (
	"slices"
	"strings"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/pkg/proposal/changes"
	"github.com/morozj01/kaddie/pkg/proposal/fieldpath"
	"github.com/morozj01/kaddie/pkg/proposal/schema"
)

// evaluateStatic is EvaluateStatic: proposal-level findings first, then each
// request in order, a request's well-formedness first, its body hygiene next
// and its protections last, in catalogue order.
func evaluateStatic(p *v1alpha1.SREProposal, schemas schema.Provider) []Finding {
	out := proposalFindings(p)
	if p.Spec.Type != v1alpha1.ProposalTypeAPI || p.Spec.API == nil {
		return out
	}
	for i, req := range p.Spec.API.Requests {
		out = append(out, requestMalformed(i, req.RequestFields)...)
		out = append(out, hygieneFindings(i, req.RequestFields)...)
		out = append(out, protectionFindings(i, req.RequestFields, schemas)...)
	}
	return out
}

// protectionFindings matches one request against the catalogue. A protection
// whose rule has no fields is reported once, when the request's target
// matches. A rule with fields matches only when the request writes one of
// them, and each protected path is reported once with the first change that
// matched it. A request whose apiVersion doesn't parse has no target, so
// Malformed is all that reports it.
func protectionFindings(index int, req v1alpha1.RequestFields, schemas schema.Provider) []Finding {
	t, err := targetOf(req)
	if err != nil {
		return nil
	}
	changed := lazyChanges(req, t, schemas)
	var out []Finding
	for _, prot := range catalogue {
		targetHit := false
		seen := map[string]bool{}
		for _, rule := range prot.Rules {
			if !rule.matchesTarget(t) {
				continue
			}
			if len(rule.Fields) == 0 {
				targetHit = true
				continue
			}
			if req.Verb == verbDelete {
				continue
			}
			root, chs := changed()
			for _, path := range rule.Fields {
				field := path.String()
				if seen[field] {
					continue
				}
				if c, ok := firstMatch(path, chs, root); ok {
					seen[field] = true
					out = append(out, Finding{Reason: prot.Reason, Request: index, Field: field,
						Changed: fieldpath.Format(c.Path), Explanation: prot.Explanation})
				}
			}
		}
		if targetHit {
			if reason, flagged := applyException(prot.Reason, t, req); flagged {
				explanation := prot.Explanation
				if reason == ReadRoleRule {
					explanation = readRoleExplanation
				}
				out = append(out, Finding{Reason: reason, Request: index, Explanation: explanation})
			}
		}
	}
	return out
}

// lazyChanges returns a function that computes, once, the schema root of the
// request and the fields its body may change, less the ignored ones. Most
// requests match no field rule, and then nothing is computed.
func lazyChanges(req v1alpha1.RequestFields, t target, schemas schema.Provider) func() (schema.Node, []fieldpath.Change) {
	var (
		done bool
		root schema.Node
		chs  []fieldpath.Change
	)
	return func() (schema.Node, []fieldpath.Change) {
		if !done {
			done = true
			version := req.APIVersion
			if _, v, ok := strings.Cut(req.APIVersion, "/"); ok {
				version = v
			}
			root = schemas.Root(schema.Resource{Group: t.group, Version: version, Resource: t.resource, Subresource: t.subresource})
			chs = withoutIgnored(changes.Static(req.Verb, req.PatchType, []byte(req.Body), root), t.subresource)
		}
		return root, chs
	}
}

// firstMatch returns the first change that a field rule path names, or whose
// subtree may hold a field it names (base §4.2.1). root is the schema of the
// object, which says whether such a field can exist.
func firstMatch(path fieldpath.Path, chs []fieldpath.Change, root schema.Node) (fieldpath.Change, bool) {
	for _, c := range chs {
		covers, below := fieldpath.Relate(path, c.Path)
		if covers {
			return c, true
		}
		if !c.Subtree {
			continue
		}
		at := root.At(c.Path)
		for _, b := range below {
			if at.Reaches(b) {
				return c, true
			}
		}
	}
	return fieldpath.Change{}, false
}

// ignoredFields are paths no field rule sees (base §4.2.1): identity and
// bookkeeping the server sets or that name the object, never what it does.
// status is added unless the request is on the status subresource.
var ignoredFields = [][]string{
	{"apiVersion"}, {"kind"},
	{"metadata", "name"}, {"metadata", "generateName"}, {"metadata", "namespace"},
	{"metadata", "managedFields"}, {"metadata", "resourceVersion"}, {"metadata", "uid"},
	{"metadata", "generation"}, {"metadata", "creationTimestamp"},
}

// withoutIgnored drops each change at or beneath an ignored path. A Subtree
// change at an ancestor of one (metadata, the root) stays, since it may reach
// fields that aren't ignored.
func withoutIgnored(chs []fieldpath.Change, subresource string) []fieldpath.Change {
	ignored := ignoredFields
	if subresource != "status" {
		ignored = append(slices.Clone(ignored), []string{"status"})
	}
	var out []fieldpath.Change
	for _, c := range chs {
		if !beneathAny(c.Path, ignored) {
			out = append(out, c)
		}
	}
	return out
}

// beneathAny reports whether path is at or beneath one of the prefixes.
func beneathAny(path []fieldpath.Segment, prefixes [][]string) bool {
	for _, prefix := range prefixes {
		if len(path) < len(prefix) {
			continue
		}
		at := true
		for i, name := range prefix {
			if path[i].Key != name {
				at = false
				break
			}
		}
		if at {
			return true
		}
	}
	return false
}
