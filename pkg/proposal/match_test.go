package proposal

import (
	"testing"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
)

// tgt is a request target for the field tests; names and namespaces are
// already the candidate lists targetOf would produce.
func tgt(group, resource, sub, verb string, namespaces, names []string) target {
	return target{group: group, resource: resource, subresource: sub, verb: verb, namespaces: namespaces, names: names}
}

// podsCreate is the base target the field tests vary one field at a time.
var podsCreate = tgt("", "pods", "", "create", []string{"team-a"}, []string{"web"})

type fieldCase struct {
	name  string
	rule  Rule
	t     target
	match bool
}

func runFieldCases(t *testing.T, cases []fieldCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.rule.matchesTarget(tc.t); got != tc.match {
				t.Errorf("matchesTarget = %v, want %v", got, tc.match)
			}
		})
	}
}

// TestT12_RuleFieldsMatch checks each match field of base §4.2 with a match
// and a no-match, and that an omitted field matches anything.
func TestT12_RuleFieldsMatch(t *testing.T) {
	sub := func(s string) target { x := podsCreate; x.subresource = s; return x }
	with := func(f func(*target)) target { x := podsCreate; f(&x); return x }
	runFieldCases(t, []fieldCase{
		{"omitted fields match anything", Rule{}, podsCreate, true},

		{"apiGroups core matches core", Rule{APIGroups: []string{""}}, podsCreate, true},
		{"apiGroups core does not match apps", Rule{APIGroups: []string{""}}, with(func(x *target) { x.group = "apps" }), false},
		{"apiGroups named matches", Rule{APIGroups: []string{"apps"}}, with(func(x *target) { x.group = "apps" }), true},
		{"apiGroups named does not match core", Rule{APIGroups: []string{"apps"}}, podsCreate, false},
		{"apiGroups named does not match another group", Rule{APIGroups: []string{"apps"}}, with(func(x *target) { x.group = "batch" }), false},
		{"apiGroups star matches core", Rule{APIGroups: []string{"*"}}, podsCreate, true},
		{"apiGroups star matches any group", Rule{APIGroups: []string{"*"}}, with(func(x *target) { x.group = "kaddie.org" }), true},
		{"apiGroups list matches any entry", Rule{APIGroups: []string{"apps", ""}}, podsCreate, true},

		{"resources exact matches", Rule{Resources: []string{"pods"}}, podsCreate, true},
		{"resources exact does not match another", Rule{Resources: []string{"secrets"}}, podsCreate, false},
		{"bare resource covers a subresource", Rule{Resources: []string{"pods"}}, sub("exec"), true},
		{"resource/sub matches that subresource", Rule{Resources: []string{"pods/exec"}}, sub("exec"), true},
		{"resource/sub does not match another subresource", Rule{Resources: []string{"pods/exec"}}, sub("attach"), false},
		{"resource/sub does not match the main resource", Rule{Resources: []string{"pods/exec"}}, podsCreate, false},
		{"resource/sub does not match another resource", Rule{Resources: []string{"pods/exec"}}, with(func(x *target) { x.resource = "services"; x.subresource = "exec" }), false},
		{"star matches any resource", Rule{Resources: []string{"*"}}, podsCreate, true},
		{"star matches any resource with a subresource", Rule{Resources: []string{"*"}}, sub("exec"), true},
		{"star/sub matches sub on any resource", Rule{Resources: []string{"*/exec"}}, sub("exec"), true},
		{"star/sub matches sub on another resource", Rule{Resources: []string{"*/exec"}}, with(func(x *target) { x.resource = "nodes"; x.subresource = "exec" }), true},
		{"star/sub does not match another subresource", Rule{Resources: []string{"*/exec"}}, sub("log"), false},
		{"star/sub does not match the main resource", Rule{Resources: []string{"*/exec"}}, podsCreate, false},
		{"resources list matches any entry", Rule{Resources: []string{"secrets", "pods"}}, podsCreate, true},

		{"verbs matches", Rule{Verbs: []string{"create"}}, podsCreate, true},
		{"verbs does not match another", Rule{Verbs: []string{"delete"}}, podsCreate, false},
		{"verbs list matches any entry", Rule{Verbs: []string{"delete", "create"}}, podsCreate, true},
		{"verbs star matches any verb", Rule{Verbs: []string{"*"}}, podsCreate, true},

		{"namespaces exact matches", Rule{Namespaces: []string{"team-a"}}, podsCreate, true},
		{"namespaces exact does not match another", Rule{Namespaces: []string{"kube-system"}}, podsCreate, false},
		{"namespaces prefix matches", Rule{Namespaces: []string{"team-*"}}, podsCreate, true},
		{"namespaces prefix does not match", Rule{Namespaces: []string{"kube-*"}}, podsCreate, false},
		{"namespaces never match a cluster-scoped request", Rule{Namespaces: []string{"kube-system"}}, with(func(x *target) { x.namespaces = nil }), false},

		{"names exact matches", Rule{Names: []string{"web"}}, podsCreate, true},
		{"names exact does not match another", Rule{Names: []string{"db"}}, podsCreate, false},
		{"names exact is not a prefix", Rule{Names: []string{"we"}}, podsCreate, false},
		{"names trailing star matches a prefix", Rule{Names: []string{"we*"}}, podsCreate, true},
		{"names trailing star matches the whole name", Rule{Names: []string{"web*"}}, podsCreate, true},
		{"names trailing star does not match another prefix", Rule{Names: []string{"db*"}}, podsCreate, false},
		{"names lone star matches any name", Rule{Names: []string{"*"}}, podsCreate, true},
		{"names never match a request with no name", Rule{Names: []string{"web"}}, with(func(x *target) { x.names = nil }), false},
		{"names list matches any entry", Rule{Names: []string{"db", "web"}}, podsCreate, true},

		{
			"every present field must match",
			Rule{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"create"}, Namespaces: []string{"team-a"}, Names: []string{"web"}},
			podsCreate, true,
		},
		{
			"one field not matching fails the rule",
			Rule{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"delete"}, Namespaces: []string{"team-a"}, Names: []string{"web"}},
			podsCreate, false,
		},
	})
}

func TestPatternsOverlap(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"kube-system", "kube-system", true},
		{"kube-system", "kube-public", false},
		{"kube-*", "kube-system", true},
		{"kube-system", "kube-*", true},
		{"kube-*", "kubernetes", false},
		{"kube-*", "kube-*", true},
		{"kube-*", "kube-s*", true},
		{"kube-s*", "kube-*", true},
		{"kube-*", "team-*", false},
		{"*", "anything", true},
		{"anything", "*", true},
		{"*", "*", true},
		{"", "", true},
		{"", "a", false},
	}
	for _, tc := range tests {
		if got := patternsOverlap(tc.a, tc.b); got != tc.want {
			t.Errorf("patternsOverlap(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func req(verb, apiVersion, resource, namespace, name, body string) v1alpha1.RequestFields {
	return v1alpha1.RequestFields{
		Verb:       v1alpha1.RequestVerb(verb),
		APIVersion: apiVersion,
		Resource:   resource,
		Namespace:  namespace,
		Name:       name,
		Body:       body,
	}
}

func withSubresource(r v1alpha1.RequestFields, sub string) v1alpha1.RequestFields {
	r.Subresource = sub
	return r
}

// TestT14_GenerateNameAndNamespaceResource checks that no spelling of a
// create evades a names or namespaces rule: the body's name, generateName and
// namespace all count, and the namespaces resource is matched by its name.
func TestT14_GenerateNameAndNamespaceResource(t *testing.T) {
	kubeSystemNs := Rule{Namespaces: []string{"kube-system"}}
	kaddieNs := Rule{Namespaces: []string{"kaddie-system"}}
	tests := []struct {
		name string
		req  v1alpha1.RequestFields
		rule Rule
		want bool
	}{
		{
			name: "generateName prefix overlaps a namespace rule",
			req:  req("create", "v1", "configmaps", "", "", `{"metadata":{"generateName":"x-","namespace":"kube-system"}}`),
			rule: kubeSystemNs,
			want: true,
		},
		{
			name: "body namespace is checked even when the request has none",
			req:  req("create", "v1", "configmaps", "", "cm", `{"metadata":{"name":"cm","namespace":"kube-system"}}`),
			rule: kubeSystemNs,
			want: true,
		},
		{
			name: "body namespace is checked even when the request names another",
			req:  req("create", "v1", "configmaps", "default", "cm", `{"metadata":{"name":"cm","namespace":"kaddie-system"}}`),
			rule: kaddieNs,
			want: true,
		},
		{
			name: "request namespace still counts",
			req:  req("create", "v1", "configmaps", "kaddie-system", "cm", `{"metadata":{"name":"cm"}}`),
			rule: kaddieNs,
			want: true,
		},
		{
			name: "an unrelated create does not match",
			req:  req("create", "v1", "configmaps", "default", "cm", `{"metadata":{"name":"cm","namespace":"default"}}`),
			rule: kubeSystemNs,
			want: false,
		},
		{
			name: "body name matches a names rule",
			req:  req("create", "v1", "configmaps", "default", "", `{"metadata":{"name":"kaddie-agent-read"}}`),
			rule: Rule{Names: []string{"kaddie-agent-read"}},
			want: true,
		},
		{
			name: "generateName prefix overlaps an exact names rule",
			req:  req("create", "rbac.authorization.k8s.io/v1", "clusterroles", "", "", `{"metadata":{"generateName":"kaddie-agent-"}}`),
			rule: Rule{Names: []string{"kaddie-agent-read"}},
			want: true,
		},
		{
			name: "generateName prefix overlaps a prefix names rule from either side",
			req:  req("create", "v1", "configmaps", "default", "", `{"metadata":{"generateName":"kad"}}`),
			rule: Rule{Names: []string{"kaddie-*"}},
			want: true,
		},
		{
			name: "generateName prefix that cannot reach the name does not match",
			req:  req("create", "v1", "configmaps", "default", "", `{"metadata":{"generateName":"web-"}}`),
			rule: Rule{Names: []string{"kaddie-agent-read"}},
			want: false,
		},
		{
			name: "generateName kube- overlaps the kube-system namespace on the namespaces resource",
			req:  req("create", "v1", "namespaces", "", "", `{"metadata":{"generateName":"kube-"}}`),
			rule: kubeSystemNs,
			want: true,
		},
		{
			name: "namespaces resource create is matched by its body name",
			req:  req("create", "v1", "namespaces", "", "", `{"metadata":{"name":"kube-system"}}`),
			rule: kubeSystemNs,
			want: true,
		},
		{
			name: "namespaces resource is matched by its request name",
			req:  req("patch", "v1", "namespaces", "", "kaddie-system", `{"metadata":{"labels":{"a":"b"}}}`),
			rule: kaddieNs,
			want: true,
		},
		{
			name: "namespaces resource delete is matched by its name",
			req:  req("delete", "v1", "namespaces", "", "kube-public", ""),
			rule: Rule{Namespaces: []string{"kube-system", "kube-public"}},
			want: true,
		},
		{
			name: "namespaces resource subresource is matched by its name",
			req:  withSubresource(req("patch", "v1", "namespaces", "", "kube-system", `{}`), "finalize"),
			rule: kubeSystemNs,
			want: true,
		},
		{
			name: "namespaces resource in another namespace does not match",
			req:  req("create", "v1", "namespaces", "", "team-a", `{"metadata":{"name":"team-a"}}`),
			rule: kubeSystemNs,
			want: false,
		},
		{
			name: "a namespaced resource named kube-system is not in that namespace",
			req:  req("create", "v1", "configmaps", "default", "kube-system", `{"metadata":{"name":"kube-system"}}`),
			rule: kubeSystemNs,
			want: false,
		},
		{
			name: "namespace in a group other than core is not the namespaces resource",
			req:  req("create", "example.com/v1", "namespaces", "", "kube-system", `{"metadata":{"name":"kube-system"}}`),
			rule: kubeSystemNs,
			want: false,
		},
		{
			name: "patch ignores the body's names",
			req:  req("patch", "v1", "configmaps", "default", "cm", `{"metadata":{"namespace":"kube-system"}}`),
			rule: kubeSystemNs,
			want: false,
		},
		{
			name: "a create body that is not an object adds nothing",
			req:  req("create", "v1", "configmaps", "default", "cm", `["kube-system"]`),
			rule: kubeSystemNs,
			want: false,
		},
		{
			name: "a create body that is not JSON adds nothing but the request fields still count",
			req:  req("create", "v1", "configmaps", "kube-system", "cm", `not json`),
			rule: kubeSystemNs,
			want: true,
		},
		{
			name: "a non-string name adds nothing",
			req:  req("create", "v1", "configmaps", "default", "", `{"metadata":{"name":5,"namespace":["kube-system"]}}`),
			rule: kubeSystemNs,
			want: false,
		},
		{
			name: "an empty generateName adds no catch-all pattern",
			req:  req("create", "v1", "configmaps", "default", "", `{"metadata":{"generateName":""}}`),
			rule: Rule{Names: []string{"kaddie-agent-read"}},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tg, err := targetOf(tc.req)
			if err != nil {
				t.Fatalf("targetOf: %v", err)
			}
			if got := tc.rule.matchesTarget(tg); got != tc.want {
				t.Errorf("matchesTarget(%+v) = %v, want %v", tg, got, tc.want)
			}
		})
	}
}

func TestTargetOf(t *testing.T) {
	t.Run("core group", func(t *testing.T) {
		tg, err := targetOf(req("patch", "v1", "pods", "ns", "p", ""))
		if err != nil || tg.group != "" || tg.resource != "pods" || tg.verb != "patch" {
			t.Errorf("got %+v, %v", tg, err)
		}
	})
	t.Run("named group", func(t *testing.T) {
		tg, err := targetOf(req("patch", "apps/v1", "deployments", "ns", "d", ""))
		if err != nil || tg.group != "apps" {
			t.Errorf("got %+v, %v", tg, err)
		}
	})
	t.Run("subresource carried", func(t *testing.T) {
		r := req("patch", "apps/v1", "deployments", "ns", "d", "")
		r.Subresource = "scale"
		tg, err := targetOf(r)
		if err != nil || tg.subresource != "scale" {
			t.Errorf("got %+v, %v", tg, err)
		}
	})
	t.Run("names and namespaces are deduplicated", func(t *testing.T) {
		tg, err := targetOf(req("create", "v1", "configmaps", "ns", "cm", `{"metadata":{"name":"cm","namespace":"ns"}}`))
		if err != nil || len(tg.names) != 1 || len(tg.namespaces) != 1 {
			t.Errorf("got %+v, %v", tg, err)
		}
	})
	t.Run("generateName is a prefix pattern", func(t *testing.T) {
		tg, err := targetOf(req("create", "v1", "pods", "ns", "", `{"metadata":{"generateName":"web-"}}`))
		if err != nil || len(tg.names) != 1 || tg.names[0] != "web-*" {
			t.Errorf("got %+v, %v", tg, err)
		}
	})
	for _, bad := range []string{"", "v2", "apps", "/v1", "apps/", "a/b/c"} {
		t.Run("unparseable apiVersion "+bad, func(t *testing.T) {
			if tg, err := targetOf(req("create", bad, "pods", "ns", "p", `{}`)); err == nil {
				t.Errorf("targetOf(apiVersion %q) = %+v, want an error", bad, tg)
			}
		})
	}
}
