package proposal

import (
	"slices"

	"github.com/morozj01/kaddie/pkg/proposal/fieldpath"
)

// Catalogue group headings (base §6.3).
const (
	groupIntegrity = "System integrity"
	groupSensitive = "Sensitive data"
	groupCode      = "What code runs, and as whom"
	groupDeploy    = "Deployment mechanism"
	groupPreview   = "Previewability"
)

// catalogue is the built-in protection table (base §6.3), in rendering order.
// It is data: nothing reads it from Helm values or config, so no deployment
// can weaken it. Matching code iterates it; the one exception, the
// kaddie-agent-read role, lives in readrole.go. Protections returns copies,
// so a caller can't edit it.
var catalogue = []Protection{
	{
		Reason:      KaddieGroup,
		Group:       groupIntegrity,
		Summary:     "any request in the kaddie.org API group (SRE configuration, triggers, proposals)",
		Explanation: "The request touches a Kaddie resource. That could change Kaddie's own switches, triggers or proposals, or its own record of what it did.",
		Rules:       []Rule{{APIGroups: []string{"kaddie.org"}}},
	},
	{
		Reason:      KaddieNamespace,
		Group:       groupIntegrity,
		Summary:     "any request in the kaddie-system namespace, or on the namespace itself",
		Explanation: "The request touches the namespace Kaddie runs in. That could change Kaddie's workloads, secrets or its safety limits.",
		Rules:       []Rule{{Namespaces: []string{"kaddie-system"}}},
	},
	{
		Reason:      SystemNamespace,
		Group:       groupIntegrity,
		Summary:     "any request in kube-system, kube-public or kube-node-lease, or on those namespaces",
		Explanation: "The request touches a namespace Kubernetes itself depends on. A mistake there can take down cluster control or node membership.",
		Rules:       []Rule{{Namespaces: []string{"kube-system", "kube-public", "kube-node-lease"}}},
	},
	{
		Reason:      RBAC,
		Group:       groupIntegrity,
		Summary:     "any RBAC request (roles, bindings), except a patch to the kaddie-agent-read ClusterRole that only adds, replaces or removes whole read-only rules",
		Explanation: "The request touches RBAC. That could grant someone, including the agent, permissions the cluster's owners did not intend.",
		Rules:       []Rule{{APIGroups: []string{"rbac.authorization.k8s.io"}}},
	},
	{
		Reason:      AdmissionControl,
		Group:       groupIntegrity,
		Summary:     "any request on admission webhooks or admission policies",
		Explanation: "The request touches admission control. That could switch off policy checks, or add a webhook that rewrites or rejects every later change.",
		Rules:       []Rule{{APIGroups: []string{"admissionregistration.k8s.io"}}},
	},
	{
		Reason:      CustomResourceDefinition,
		Group:       groupIntegrity,
		Summary:     "any request on CustomResourceDefinitions",
		Explanation: "The request touches a CustomResourceDefinition. Changing or deleting one changes or deletes every object of that type.",
		Rules:       []Rule{{APIGroups: []string{"apiextensions.k8s.io"}, Resources: []string{"customresourcedefinitions"}}},
	},
	{
		Reason:      APIService,
		Group:       groupIntegrity,
		Summary:     "any request on APIServices",
		Explanation: "The request touches an APIService. That could redirect a part of the Kubernetes API to a different server.",
		Rules:       []Rule{{APIGroups: []string{"apiregistration.k8s.io"}, Resources: []string{"apiservices"}}},
	},
	{
		Reason:      ServiceAccountToken,
		Group:       groupIntegrity,
		Summary:     "any request on the serviceaccounts/token subresource",
		Explanation: "The request mints a ServiceAccount token. The token is a working credential for that account.",
		Rules:       []Rule{{APIGroups: []string{""}, Resources: []string{"serviceaccounts/token"}}},
	},
	{
		Reason:      CSRApproval,
		Group:       groupIntegrity,
		Summary:     "any request on the certificatesigningrequests/approval subresource",
		Explanation: "The request approves or denies a certificate signing request. An approved request yields a credential the cluster trusts.",
		Rules:       []Rule{{APIGroups: []string{"certificates.k8s.io"}, Resources: []string{"certificatesigningrequests/approval"}}},
	},
	{
		Reason:      Secret,
		Group:       groupSensitive,
		Summary:     "any request on Secrets",
		Explanation: "The request touches a Secret. That could expose or replace a credential.",
		Rules:       []Rule{{APIGroups: []string{""}, Resources: []string{"secrets"}}},
	},
	{
		Reason:      SecretDeliveryKind,
		Group:       groupSensitive,
		Summary:     "any request on kinds that deliver Secrets: External Secrets, Sealed Secrets, the Secrets Store CSI driver, the Vault secrets operator, 1Password items",
		Explanation: "The request touches an object that creates or delivers Secrets. That could redirect where a credential comes from, or write one.",
		Rules: []Rule{
			{APIGroups: []string{"external-secrets.io"}, Resources: []string{"externalsecrets", "clusterexternalsecrets", "secretstores", "clustersecretstores", "pushsecrets"}},
			{APIGroups: []string{"bitnami.com"}, Resources: []string{"sealedsecrets"}},
			{APIGroups: []string{"secrets-store.csi.x-k8s.io"}, Resources: []string{"secretproviderclasses"}},
			{APIGroups: []string{"secrets.hashicorp.com"}, Resources: []string{"vaultauths", "vaultauthglobals", "vaultconnections", "vaultdynamicsecrets", "vaultpkisecrets", "vaultstaticsecrets"}},
			{APIGroups: []string{"onepassword.com"}, Resources: []string{"onepassworditems"}},
		},
	},
	{
		Reason:      WorkloadField,
		Group:       groupCode,
		Summary:     "a change to a workload's images, commands, arguments, environment, volumes, ServiceAccount, host access, node or security context",
		Explanation: "The request changes what code a workload runs, or as which identity, or with what access to the node. A change here can run attacker-chosen code with the workload's permissions.",
		Rules: []Rule{
			{APIGroups: []string{""}, Resources: []string{"pods", "replicationcontrollers"}, Fields: workloadFields},
			{APIGroups: []string{"apps"}, Resources: []string{"deployments", "replicasets", "statefulsets", "daemonsets"}, Fields: workloadFields},
			{APIGroups: []string{"batch"}, Resources: []string{"jobs", "cronjobs"}, Fields: workloadFields},
		},
	},
	{
		Reason:      ReconcilerKind,
		Group:       groupDeploy,
		Summary:     "any request on Argo CD and Flux objects",
		Explanation: "The request touches an object that makes a reconciler deploy things. That could change what the reconciler applies, and so what runs, anywhere it has access.",
		Rules: []Rule{
			{APIGroups: []string{"argoproj.io"}, Resources: []string{"applications", "applicationsets", "appprojects"}},
			{APIGroups: []string{"source.toolkit.fluxcd.io"}, Resources: []string{"*"}},
			{APIGroups: []string{"kustomize.toolkit.fluxcd.io"}, Resources: []string{"kustomizations"}},
			{APIGroups: []string{"helm.toolkit.fluxcd.io"}, Resources: []string{"helmreleases"}},
			{APIGroups: []string{"image.toolkit.fluxcd.io"}, Resources: []string{"imagerepositories", "imagepolicies", "imageupdateautomations"}},
		},
	},
	{
		Reason:      DeleteCollection,
		Group:       groupPreview,
		Summary:     "the deletecollection verb",
		Explanation: "The request deletes every object of a kind that a selector matches. A dry run can't show which objects that is, so the effect can't be previewed.",
		Rules:       []Rule{{Verbs: []string{"deletecollection"}}},
	},
	{
		Reason:      ConnectSubresource,
		Group:       groupPreview,
		Summary:     "any request on exec, attach, portforward or proxy subresources",
		Explanation: "The request opens a connection into a pod, service or node. A dry run can't show what runs or is read through it.",
		Rules:       []Rule{{Resources: []string{"*/exec", "*/attach", "*/portforward", "*/proxy"}}},
	},
}

// workloadFields are the protected field paths of every workload kind (base
// §6.3): the spec's list, no additions.
var workloadFields = []fieldpath.Path{
	fieldpath.MustParse("**.containers[*].image"),
	fieldpath.MustParse("**.initContainers[*].image"),
	fieldpath.MustParse("**.ephemeralContainers[*]"),
	fieldpath.MustParse("**.containers[*].command"),
	fieldpath.MustParse("**.containers[*].args"),
	fieldpath.MustParse("**.containers[*].env"),
	fieldpath.MustParse("**.containers[*].envFrom"),
	fieldpath.MustParse("**.containers[*].volumeMounts"),
	fieldpath.MustParse("**.containers[*].securityContext"),
	fieldpath.MustParse("**.initContainers[*].command"),
	fieldpath.MustParse("**.initContainers[*].args"),
	fieldpath.MustParse("**.initContainers[*].env"),
	fieldpath.MustParse("**.initContainers[*].envFrom"),
	fieldpath.MustParse("**.initContainers[*].volumeMounts"),
	fieldpath.MustParse("**.initContainers[*].securityContext"),
	fieldpath.MustParse("**.volumes"),
	fieldpath.MustParse("**.serviceAccountName"),
	fieldpath.MustParse("**.automountServiceAccountToken"),
	fieldpath.MustParse("**.hostNetwork"),
	fieldpath.MustParse("**.hostPID"),
	fieldpath.MustParse("**.hostIPC"),
	fieldpath.MustParse("**.nodeName"),
	fieldpath.MustParse("**.securityContext"),
}

// cloneProtections deep-copies a catalogue, so no caller shares storage with
// the table.
func cloneProtections(src []Protection) []Protection {
	out := make([]Protection, len(src))
	for i, p := range src {
		out[i] = p
		out[i].Rules = make([]Rule, len(p.Rules))
		for j, r := range p.Rules {
			out[i].Rules[j] = Rule{
				APIGroups:  slices.Clone(r.APIGroups),
				Resources:  slices.Clone(r.Resources),
				Verbs:      slices.Clone(r.Verbs),
				Namespaces: slices.Clone(r.Namespaces),
				Names:      slices.Clone(r.Names),
				Fields:     cloneFields(r.Fields),
			}
		}
	}
	return out
}

func cloneFields(paths []fieldpath.Path) []fieldpath.Path {
	if paths == nil {
		return nil
	}
	out := make([]fieldpath.Path, len(paths))
	for i, p := range paths {
		out[i] = fieldpath.Path{AnyDepth: p.AnyDepth, Steps: slices.Clone(p.Steps)}
	}
	return out
}
