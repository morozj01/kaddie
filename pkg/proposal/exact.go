package proposal

import (
	"encoding/json"
	"slices"
	"sync"

	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/pkg/proposal/changes"
	"github.com/morozj01/kaddie/pkg/proposal/fieldpath"
	"github.com/morozj01/kaddie/pkg/proposal/schema"
)

// evaluateExact is EvaluateExact: findings for each observed request, in
// request order. Hygiene and well-formedness are static-only and not repeated.
func evaluateExact(p *v1alpha1.SREProposal, observed []Observed, schemas schema.Provider) []Finding {
	if p.Spec.API == nil {
		return nil
	}
	sorted := slices.SortedStableFunc(slices.Values(observed), func(a, b Observed) int { return a.Request - b.Request })
	var out []Finding
	for _, obs := range sorted {
		if obs.Request < 0 || obs.Request >= len(p.Spec.API.Requests) {
			continue
		}
		req := p.Spec.API.Requests[obs.Request].RequestFields
		t, err := targetOf(req)
		if err != nil {
			continue
		}
		out = append(out, matchProtections(protectionInput{
			index:     obs.Request,
			t:         t,
			ownSource: SourceRequest,
			changed: sync.OnceValue(func() requestChanges {
				return exactChanges(req, t, obs, schemas)
			}),
			readRolePasses: func() bool { return roleStaysReadOnly(obs.DryRun) },
		})...)
	}
	return out
}

// exactChanges works out what an observed request really changes, split into
// the request's own changes and the ones admission added (base §4.2.1 item 2).
// When the observed objects can't be compared it falls back on the static
// changes, all counted as the request's own: when in doubt it changed.
func exactChanges(req v1alpha1.RequestFields, t target, obs Observed, schemas schema.Provider) requestChanges {
	root := rootOf(req, t, schemas)
	own, admission, ok := observedChanges(req, root, obs)
	if !ok {
		own = changes.Static(req.Verb, req.PatchType, []byte(req.Body), root)
		admission = nil
	}
	return requestChanges{
		root:      root,
		own:       withoutIgnored(own, t.subresource),
		admission: withoutIgnored(admission, t.subresource),
	}
}

// observedChanges splits the difference the dry run shows. ok is false when
// the objects can't be compared.
//
// A patch's own changes are what applying it client-side does to Live, and
// admission's are what the dry run then differs by. If the patch can't be
// applied here, everything that differs counts as the request's own. A
// create's own changes are the body's fields, and admission's are those the
// dry run changed or dropped: a field the body leaves unset that the server
// fills in is a default, which admission can't be told from, and is ignored.
func observedChanges(req v1alpha1.RequestFields, root schema.Node, obs Observed) (own, admission []fieldpath.Change, ok bool) {
	switch req.Verb {
	case verbDelete:
		return nil, nil, true
	case verbCreate:
		own = changes.Static(req.Verb, req.PatchType, []byte(req.Body), root)
		diff, err := changes.Diff([]byte(req.Body), obs.DryRun)
		if err != nil {
			return nil, nil, false
		}
		bodyFields := map[string]bool{}
		for _, c := range own {
			bodyFields[fieldpath.Format(c.Path)] = true
		}
		for _, c := range diff {
			if bodyFields[fieldpath.Format(c.Path)] {
				admission = append(admission, c)
			}
		}
		return own, admission, true
	case verbPatch:
		if applied, err := changes.Apply(req.PatchType, obs.Live, []byte(req.Body), root); err == nil {
			own, err1 := changes.Diff(obs.Live, applied)
			admission, err2 := changes.Diff(applied, obs.DryRun)
			return own, admission, err1 == nil && err2 == nil
		}
		own, err := changes.Diff(obs.Live, obs.DryRun)
		return own, nil, err == nil
	}
	return nil, nil, false
}

// roleStaysReadOnly judges the kaddie-agent-read exception from the object
// the dry run returned: every rule passes the read-role rule and there is no
// aggregationRule. An object that isn't a ClusterRole fails, so a missing
// dry run can't pass.
func roleStaysReadOnly(dryRun json.RawMessage) bool {
	var role rbacv1.ClusterRole
	if json.Unmarshal(dryRun, &role) != nil || role.AggregationRule != nil {
		return false
	}
	return !slices.ContainsFunc(role.Rules, func(r rbacv1.PolicyRule) bool { return len(readRoleViolations(r)) > 0 })
}
