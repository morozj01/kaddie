// Package proposal is the shared rule engine (base §6.5): the built-in
// protections, static and exact evaluation, and the acting precondition
// CanAct. kaddie-tools and kaddie-ui import it, so they can't disagree about
// whether a proposal is high risk (invariant I11). kaddie-git-writer must not.
package proposal

import (
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
	group, resource, subresource, verb string
	// namespaces holds the request's namespace and, for create, the body's
	// metadata.namespace; for the namespaces resource, the names below.
	namespaces []string
	// names holds the request's name and, for create, the body's
	// metadata.name and metadata.generateName followed by *.
	names []string
}

// matchesTarget reports whether t matches every field of r except Fields.
func (r Rule) matchesTarget(t target) bool {
	panic("unimplemented")
}
