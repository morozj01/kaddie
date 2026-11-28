package proposal

import (
	"slices"
	"strings"
	"sync"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/pkg/proposal/changes"
	"github.com/morozj01/kaddie/pkg/proposal/fieldpath"
	"github.com/morozj01/kaddie/pkg/proposal/schema"
)

// EvaluateStatic evaluates a proposal from its own content, with no cluster
// access (base §6.2): well-formedness, the proposal namespace, body hygiene,
// and the built-in protections with field rules evaluated on changes.Static.
// The kaddie-agent-read exception applies only to a patch whose every
// operation adds, replaces or removes whole rules, each passing the
// read-role rule (base §5.4.4). No findings means not high risk. Findings are
// ordered: the proposal's own (NoRequest) first, then by request, and within
// a request Malformed, body hygiene, then catalogue order. Git proposals get
// the proposal checks only until pkg/gitproposal's static checks land (T12.2).
func EvaluateStatic(p *v1alpha1.SREProposal, schemas schema.Provider) []Finding {
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

// protectionFindings matches one request against the catalogue from its own
// content: field rules see the changes it may make (changes.Static), and the
// kaddie-agent-read exception is judged on its patch body. A request whose
// apiVersion doesn't parse has no target, so Malformed is all that reports it.
func protectionFindings(index int, req v1alpha1.RequestFields, schemas schema.Provider) []Finding {
	t, err := targetOf(req)
	if err != nil {
		return nil
	}
	return matchProtections(protectionInput{
		index: index,
		t:     t,
		changed: sync.OnceValue(func() requestChanges {
			root := rootOf(req, t, schemas)
			return requestChanges{
				root: root,
				own:  withoutIgnored(changes.Static(req.Verb, req.PatchType, []byte(req.Body), root), t.subresource),
			}
		}),
		readRolePasses: func() bool { return patchPassesReadRole(req.PatchType, req.Body) },
	})
}

// requestChanges are the fields a request changes, split by who changes them.
// Static evaluation has own changes only.
type requestChanges struct {
	// root is the schema of the object, which says whether a field can exist
	// beneath a Subtree change.
	root schema.Node
	// own are the changes the request itself makes; admission's are the ones
	// the server's admission adds on top (exact evaluation only).
	own, admission []fieldpath.Change
}

// protectionInput is what the catalogue is matched against. Static and exact
// evaluation differ only in these inputs, so they share matchProtections.
type protectionInput struct {
	index int
	t     target
	// ownSource is the Source of a finding caused by an own change or by the
	// target alone: empty for static, SourceRequest for exact.
	ownSource Source
	// changed returns the changes field rules match against. Most requests
	// match no field rule, and then it is never called.
	changed func() requestChanges
	// readRolePasses judges the kaddie-agent-read exception: whether the
	// role the request leaves behind passes the read-role rule.
	readRolePasses func() bool
}

// matchProtections matches one request against the catalogue, in catalogue
// order. A protection whose rule has no fields is reported once, when the
// request's target matches. A rule with fields matches only when the request
// changes one of them, and each protected path is reported once with the
// first change that matched it, an own change before an admission one.
func matchProtections(in protectionInput) []Finding {
	var out []Finding
	for _, prot := range catalogue {
		targetHit := false
		seen := map[string]bool{}
		for _, rule := range prot.Rules {
			if !rule.matchesTarget(in.t) {
				continue
			}
			if len(rule.Fields) == 0 {
				targetHit = true
				continue
			}
			if in.t.verb == v1alpha1.RequestVerbDelete {
				continue
			}
			ch := in.changed()
			for _, path := range rule.Fields {
				field := path.String()
				if seen[field] {
					continue
				}
				source := in.ownSource
				c, ok := firstMatch(path, ch.own, ch.root)
				if !ok {
					c, ok = firstMatch(path, ch.admission, ch.root)
					source = SourceAdmission
				}
				if ok {
					seen[field] = true
					out = append(out, Finding{Reason: prot.Reason, Request: in.index, Field: field,
						Changed: fieldpath.Format(c.Path), Source: source, Explanation: prot.Explanation})
				}
			}
		}
		if targetHit {
			if reason, flagged := applyException(prot.Reason, in.t, in.readRolePasses); flagged {
				explanation := prot.Explanation
				if reason == ReadRoleRule {
					explanation = readRoleExplanation
				}
				out = append(out, Finding{Reason: reason, Request: in.index, Source: in.ownSource, Explanation: explanation})
			}
		}
	}
	return out
}

func rootOf(req v1alpha1.RequestFields, t target, schemas schema.Provider) schema.Node {
	version := req.APIVersion
	if _, v, ok := strings.Cut(req.APIVersion, "/"); ok {
		version = v
	}
	return schemas.Root(schema.Resource{Group: t.group, Version: version, Resource: t.resource, Subresource: t.subresource})
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
