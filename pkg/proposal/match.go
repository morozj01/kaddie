// Package proposal is the shared rule engine (base §6.5): the built-in
// protections, static and exact evaluation, and the acting precondition
// CanAct. kaddie-tools and kaddie-ui import it, so they can't disagree about
// whether a proposal is high risk (invariant I11). kaddie-git-writer must not.
package proposal

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/pkg/proposal/fieldpath"
)

// Rule is the fixed rule shape of base §4.2. A request matches a rule when
// every field present in the rule matches it; an omitted field matches
// anything.
type Rule struct {
	// APIGroups lists groups; "" is core and "*" is any.
	APIGroups []string
	// Resources lists resources. "r/sub" matches that subresource only, a
	// bare "r" also matches all of r's subresources, "*" is any resource and
	// "*/sub" is sub on any resource.
	Resources []string
	// Verbs lists verbs.
	Verbs []string
	// Namespaces lists namespaces. A request on the namespaces resource
	// itself is in the namespace its name names.
	Namespaces []string
	// Names lists name patterns; a trailing * is a prefix match.
	Names []string
	// Fields lists protected field paths (base §4.2.1). A rule with fields
	// never matches delete.
	Fields []fieldpath.Path
}

// target is what a request addresses, as rules see it. Namespaces and names
// are pattern lists: every value the request could act on, so no spelling of
// a request evades a rule. A trailing * marks a generateName prefix.
type target struct {
	group, resource, subresource string
	verb                         v1alpha1.RequestVerb
	// namespaces holds the request's namespace and, for create, the body's
	// metadata.namespace; for the namespaces resource, the names below.
	namespaces []string
	// names holds the request's name and, for create, the body's
	// metadata.name and metadata.generateName followed by *.
	names []string
}

// targetOf resolves what req addresses. It fails when apiVersion is neither
// v1 (the core group) nor group/version. A create body that isn't a JSON
// object contributes no names or namespaces; body hygiene flags it elsewhere.
func targetOf(req v1alpha1.RequestFields) (target, error) {
	group, err := groupOf(req.APIVersion)
	if err != nil {
		return target{}, err
	}
	t := target{
		group:       group,
		resource:    req.Resource,
		subresource: req.Subresource,
		verb:        req.Verb,
	}
	var names, namespaces []string
	add := func(list *[]string, v string) {
		if v != "" && !slices.Contains(*list, v) {
			*list = append(*list, v)
		}
	}
	add(&names, req.Name)
	add(&namespaces, req.Namespace)
	if t.verb == v1alpha1.RequestVerbCreate {
		var body map[string]any
		if json.Unmarshal([]byte(req.Body), &body) == nil {
			meta, _ := body["metadata"].(map[string]any)
			name, _ := meta["name"].(string)
			generateName, _ := meta["generateName"].(string)
			namespace, _ := meta["namespace"].(string)
			add(&names, name)
			if generateName != "" {
				add(&names, generateName+"*")
			}
			add(&namespaces, namespace)
		}
	}
	if group == "" && req.Resource == "namespaces" {
		namespaces = slices.Clone(names)
	}
	t.names, t.namespaces = names, namespaces
	return t, nil
}

// groupOf returns the API group of an apiVersion: "" for v1, g for g/v.
func groupOf(apiVersion string) (string, error) {
	if apiVersion == "v1" {
		return "", nil
	}
	group, version, ok := strings.Cut(apiVersion, "/")
	if !ok || group == "" || version == "" || strings.Contains(version, "/") {
		return "", fmt.Errorf("apiVersion %q is neither v1 nor group/version", apiVersion)
	}
	return group, nil
}

// matchesTarget reports whether t matches every field of r except Fields.
func (r Rule) matchesTarget(t target) bool {
	return listed(r.APIGroups, func(g string) bool { return g == "*" || g == t.group }) &&
		listed(r.Resources, func(res string) bool { return resourceMatches(res, t) }) &&
		listed(r.Verbs, func(v string) bool { return v == "*" || v == string(t.verb) }) &&
		overlapsAny(r.Namespaces, t.namespaces) &&
		overlapsAny(r.Names, t.names)
}

// listed reports whether the rule field is omitted or some entry matches.
func listed(entries []string, matches func(string) bool) bool {
	return len(entries) == 0 || slices.ContainsFunc(entries, matches)
}

// resourceMatches matches one Rule.Resources entry: "r" covers r and all its
// subresources, "r/sub" only that subresource, "*" any resource, "*/sub" sub
// on any resource, and a "*" sub any subresource.
func resourceMatches(entry string, t target) bool {
	res, sub, hasSub := strings.Cut(entry, "/")
	if res != "*" && res != t.resource {
		return false
	}
	switch {
	case !hasSub:
		return true
	case sub == "*":
		return t.subresource != ""
	default:
		return sub == t.subresource
	}
}

// overlapsAny reports whether the rule field is omitted or some rule pattern
// overlaps some candidate. A rule that lists values never matches an empty
// candidate list.
func overlapsAny(rule, candidates []string) bool {
	return len(rule) == 0 || slices.ContainsFunc(rule, func(p string) bool {
		return slices.ContainsFunc(candidates, func(c string) bool { return patternsOverlap(p, c) })
	})
}

// patternsOverlap reports whether some string matches both patterns. Each
// pattern is exact or a prefix ending in *: two exact patterns overlap when
// equal, a prefix and an exact one when the exact one starts with the prefix,
// and two prefixes when one starts with the other.
func patternsOverlap(a, b string) bool {
	ap, aPrefix := strings.CutSuffix(a, "*")
	bp, bPrefix := strings.CutSuffix(b, "*")
	switch {
	case aPrefix && bPrefix:
		return strings.HasPrefix(ap, bp) || strings.HasPrefix(bp, ap)
	case aPrefix:
		return strings.HasPrefix(b, ap)
	case bPrefix:
		return strings.HasPrefix(a, bp)
	default:
		return a == b
	}
}
