package proposal

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
)

// The kaddie-agent-read exception to the RBAC protection (base §5.4.4): a
// patch of that ClusterRole that only adds, replaces or removes whole
// read-only rules is how the agent gets to read a new kind, and a human still
// approves it. Everything else touching RBAC stays an RBAC finding, and a
// patch of that role that breaks the read-role rule is a ReadRoleRule finding.

const (
	rbacGroup     = "rbac.authorization.k8s.io"
	agentReadRole = "kaddie-agent-read"
)

// readRoleExplanation is the fixed explanation of a ReadRoleRule finding.
const readRoleExplanation = "The request patches the kaddie-agent-read ClusterRole in a way that is not just adding read-only rules. It could give the agent write access, access to Secrets or exec, or control over RBAC."

// applyException decides whether a protection whose rules matched a request
// is reported, and under which reason. Only the RBAC protection has an
// exception: for a patch of kaddie-agent-read it is not reported when the
// patch passes the read-role rule, and is reported as ReadRoleRule when it
// doesn't.
func applyException(reason Reason, t target, req v1alpha1.RequestFields) (Reason, bool) {
	if reason != RBAC || !targetsAgentReadRole(t) {
		return reason, true
	}
	if patchPassesReadRole(req.PatchType, req.Body) {
		return reason, false
	}
	return ReadRoleRule, true
}

// targetsAgentReadRole reports whether t is a patch of the ClusterRole
// kaddie-agent-read: named exactly, no subresource, and no namespace, which a
// cluster-scoped role has no use for.
func targetsAgentReadRole(t target) bool {
	return t.group == rbacGroup && t.resource == "clusterroles" && t.subresource == "" &&
		t.verb == verbPatch && len(t.namespaces) == 0 &&
		len(t.names) == 1 && t.names[0] == agentReadRole
}

var readVerbs = []string{"get", "list", "watch"}

// readRoleViolations returns what is wrong with one rule of kaddie-agent-read
// (base §5.4.4), as fixed phrases; none means the rule passes. It is the one
// definition of a passing rule, shared by static and exact evaluation.
func readRoleViolations(r rbacv1.PolicyRule) []string {
	var out []string
	if len(r.Verbs) == 0 {
		out = append(out, "no verbs")
	}
	for _, v := range r.Verbs {
		if v == "*" {
			out = append(out, "wildcard verb")
		} else if !slices.Contains(readVerbs, v) {
			out = append(out, "verb other than get, list or watch")
		}
	}
	if slices.Contains(r.APIGroups, "*") {
		out = append(out, "wildcard API group")
	}
	if len(r.ResourceNames) > 0 {
		out = append(out, "resourceNames")
	}
	if len(r.NonResourceURLs) > 0 {
		out = append(out, "nonResourceURLs")
	}
	for _, res := range r.Resources {
		if v := resourceViolation(res); v != "" {
			out = append(out, v)
		}
	}
	if slices.Contains(r.APIGroups, rbacGroup) {
		out = append(out, "RBAC API group")
	}
	if slices.Contains(r.APIGroups, "kaddie.org") && slices.ContainsFunc(r.Resources, func(res string) bool { return res != "sreproposals" }) {
		out = append(out, "kaddie.org resource other than sreproposals")
	}
	return slices.Compact(out)
}

// resourceViolation names why one entry of a rule's resources fails, or "".
// RBAC resource entries are exact: "pods" never grants "pods/exec", so only
// the entries that name a forbidden resource or subresource fail.
func resourceViolation(entry string) string {
	res, sub, _ := strings.Cut(entry, "/")
	switch {
	case strings.Contains(entry, "*"):
		return "wildcard resource"
	case res == "secrets":
		return "secrets"
	case entry == "serviceaccounts/token":
		return "serviceaccounts/token"
	case entry == "certificatesigningrequests/approval":
		return "certificatesigningrequests/approval"
	case slices.Contains([]string{"exec", "attach", "portforward", "proxy"}, sub):
		return "exec, attach, portforward or proxy subresource"
	}
	return ""
}

// readRuleKeys are the keys of a PolicyRule. A rule written with any other
// key, or with a key spelled in another case, is not one a patch can be shown
// to add.
var readRuleKeys = []string{"apiGroups", "resources", "verbs", "resourceNames", "nonResourceURLs"}

// rulePasses reports whether raw is a JSON PolicyRule that passes the
// read-role rule.
func rulePasses(raw json.RawMessage) bool {
	var keys map[string]json.RawMessage
	if json.Unmarshal(raw, &keys) != nil || keys == nil {
		return false
	}
	for k := range keys {
		if !slices.Contains(readRuleKeys, k) {
			return false
		}
	}
	var r rbacv1.PolicyRule
	return json.Unmarshal(raw, &r) == nil && len(readRoleViolations(r)) == 0
}

// rulesPass reports whether raw is a JSON list of passing rules.
func rulesPass(raw json.RawMessage) bool {
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) != nil || list == nil {
		return false
	}
	return !slices.ContainsFunc(list, func(r json.RawMessage) bool { return !rulePasses(r) })
}

var ruleIndexPath = regexp.MustCompile(`^/rules/(0|[1-9][0-9]*)$`)

// patchPassesReadRole is the static check of a patch body to
// kaddie-agent-read (base §5.4.4): every operation adds, replaces or removes
// whole rules and every rule it writes passes the read-role rule. Anything
// else, including a body that isn't valid for its patch type, fails.
//
// JSON Patch may hold test, remove of /rules/N, add of /rules/- or /rules/N,
// and replace of /rules/N or /rules. A merge patch or strategic merge patch
// may hold only the key rules, a list of passing rules. Neither can name
// aggregationRule or metadata, which would let the role gain rules from
// elsewhere or lose the label that others select by.
func patchPassesReadRole(pt v1alpha1.PatchType, body string) bool {
	switch pt {
	case patchJSON:
		var ops []map[string]json.RawMessage
		if json.Unmarshal([]byte(body), &ops) != nil || ops == nil {
			return false
		}
		return !slices.ContainsFunc(ops, func(op map[string]json.RawMessage) bool { return !jsonPatchOpPasses(op) })
	case patchMerge, patchStrategic:
		var doc map[string]json.RawMessage
		if json.Unmarshal([]byte(body), &doc) != nil || len(doc) != 1 {
			return false
		}
		rules, ok := doc["rules"]
		return ok && rulesPass(rules)
	}
	return false
}

func jsonPatchOpPasses(op map[string]json.RawMessage) bool {
	var name, path string
	if json.Unmarshal(op["op"], &name) != nil || json.Unmarshal(op["path"], &path) != nil {
		return false
	}
	value, hasValue := op["value"]
	switch name {
	case "test":
		return true
	case "remove":
		return ruleIndexPath.MatchString(path)
	case "add":
		return (path == "/rules/-" || ruleIndexPath.MatchString(path)) && hasValue && rulePasses(value)
	case "replace":
		switch {
		case ruleIndexPath.MatchString(path):
			return hasValue && rulePasses(value)
		case path == "/rules":
			return hasValue && rulesPass(value)
		}
	}
	return false
}
