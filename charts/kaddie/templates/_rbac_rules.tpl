{{/*
RBAC rule templates (base §5, Architecture §6.1).

Every rule of every Role and ClusterRole the chart ships comes from exactly
one of these named templates; no rule is written into a role directly, and
test/chart fails the build if one is (T4). Each template renders a YAML list
of rules. Names match the template column of base §5's table, prefixed with
"kaddie.rules.".

Never grant: secrets, pods/exec, pods/attach, pods/portforward, pods/proxy,
services/proxy, nodes/proxy, serviceaccounts/token, impersonate, escalate,
bind, any write on RBAC, admission, CRDs or sreconfigs (except the human
role's config-create/config-update), certificatesigningrequests/approval, or
any "*" (base §5).
*/}}

{{/* The shipped investigation allowlist, as data (base §5.1). The
agent-read-allowlist template renders it, and NOTES.txt compares it with the
live role on upgrade to list new shipped entries. */}}
{{- define "kaddie.agentRead.allowlist" -}}
- apiGroups: [""]
  resources: [pods, pods/log, services, endpoints, events, nodes, namespaces, persistentvolumeclaims, persistentvolumes, replicationcontrollers, resourcequotas, limitranges, configmaps, serviceaccounts]
  verbs: [get, list, watch]
- apiGroups: [apps]
  resources: [deployments, replicasets, statefulsets, daemonsets, controllerrevisions]
  verbs: [get, list, watch]
- apiGroups: [batch]
  resources: [jobs, cronjobs]
  verbs: [get, list, watch]
- apiGroups: [autoscaling]
  resources: [horizontalpodautoscalers]
  verbs: [get, list, watch]
- apiGroups: [policy]
  resources: [poddisruptionbudgets]
  verbs: [get, list, watch]
- apiGroups: [networking.k8s.io]
  resources: [ingresses, ingressclasses, networkpolicies]
  verbs: [get, list, watch]
- apiGroups: [discovery.k8s.io]
  resources: [endpointslices]
  verbs: [get, list, watch]
- apiGroups: [events.k8s.io]
  resources: [events]
  verbs: [get, list, watch]
- apiGroups: [storage.k8s.io]
  resources: [storageclasses, volumeattachments, csidrivers, csinodes, csistoragecapacities]
  verbs: [get, list, watch]
- apiGroups: [scheduling.k8s.io]
  resources: [priorityclasses]
  verbs: [get, list, watch]
- apiGroups: [node.k8s.io]
  resources: [runtimeclasses]
  verbs: [get, list, watch]
- apiGroups: [coordination.k8s.io]
  resources: [leases]
  verbs: [get, list, watch]
- apiGroups: [admissionregistration.k8s.io]
  resources: [validatingwebhookconfigurations, mutatingwebhookconfigurations, validatingadmissionpolicies, validatingadmissionpolicybindings]
  verbs: [get, list, watch]
- apiGroups: [apiregistration.k8s.io]
  resources: [apiservices]
  verbs: [get, list, watch]
- apiGroups: [flowcontrol.apiserver.k8s.io]
  resources: [flowschemas, prioritylevelconfigurations]
  verbs: [get, list, watch]
- apiGroups: [certificates.k8s.io]
  resources: [certificatesigningrequests]
  verbs: [get, list, watch]
- apiGroups: [metrics.k8s.io]
  resources: [pods, nodes]
  verbs: [get, list]
- apiGroups: [apiextensions.k8s.io]
  resources: [customresourcedefinitions]
  verbs: [get, list, watch]
- apiGroups: [kaddie.org]
  resources: [sreproposals]
  verbs: [get, list, watch]
{{- end -}}

{{/* agent-read-allowlist: get/list/watch on the shipped allowlist. */}}
{{- define "kaddie.rules.agent-read-allowlist" -}}
{{ include "kaddie.agentRead.allowlist" . }}
{{- end -}}

{{/* agent-read-extra: the installer's agentRead.extraResources, as read
rules. Anything the read-role rule (base §5.4.4) would flag is refused. */}}
{{- define "kaddie.rules.agent-read-extra" -}}
{{- range .Values.agentRead.extraResources }}
{{- $parts := splitList "/" .resource }}
{{- if or (has .group (list "rbac.authorization.k8s.io" "kaddie.org")) (has .resource (list "secrets" "serviceaccounts/token" "certificatesigningrequests/approval")) (and (gt (len $parts) 1) (has (last $parts) (list "exec" "attach" "portforward" "proxy"))) (contains "*" .group) (contains "*" .resource) }}
{{- fail (printf "agentRead.extraResources: %s/%s would break the read-role rule (base §5.4.4)" .group .resource) }}
{{- end }}
- apiGroups: [{{ .group | quote }}]
  resources: [{{ .resource | quote }}]
  verbs: [get, list, watch]
{{- end }}
{{- end -}}

{{- define "kaddie.rules.config-read" -}}
- apiGroups: [kaddie.org]
  resources: [sreconfigs]
  verbs: [get, list, watch]
{{- end -}}

{{- define "kaddie.rules.proposal-read" -}}
- apiGroups: [kaddie.org]
  resources: [sreproposals]
  verbs: [get, list, watch]
{{- end -}}

{{- define "kaddie.rules.trigger-read" -}}
- apiGroups: [kaddie.org]
  resources: [sretriggers]
  verbs: [get, list, watch]
{{- end -}}

{{/* reconciler-read(<adapter>): the adapter's configuration kinds (GitOps
spec Appendix A). Rendered only with GitOps on. The argocd entry is the
placeholder adapter list until implementation plan T12.3. */}}
{{- define "kaddie.rules.reconciler-read" -}}
{{- if eq . "argocd" -}}
- apiGroups: [argoproj.io]
  resources: [applications, applicationsets, appprojects]
  verbs: [get, list, watch]
{{- else -}}
{{- fail (printf "unknown reconciler adapter %q" .) -}}
{{- end -}}
{{- end -}}

{{- define "kaddie.rules.proposal-create" -}}
- apiGroups: [kaddie.org]
  resources: [sreproposals]
  verbs: [create]
{{- end -}}

{{- define "kaddie.rules.proposal-prune" -}}
- apiGroups: [kaddie.org]
  resources: [sreproposals]
  verbs: [delete]
{{- end -}}

{{/* RBAC can't limit which status block a holder writes (base §5.2). */}}
{{- define "kaddie.rules.proposal-status-write" -}}
- apiGroups: [kaddie.org]
  resources: [sreproposals/status]
  verbs: [update, patch]
{{- end -}}

{{- define "kaddie.rules.trigger-status-write" -}}
- apiGroups: [kaddie.org]
  resources: [sretriggers/status]
  verbs: [update, patch]
{{- end -}}

{{- define "kaddie.rules.config-create" -}}
- apiGroups: [kaddie.org]
  resources: [sreconfigs]
  verbs: [create]
{{- end -}}

{{/* No role grants delete on sreconfigs: deleting it halts the system and
loses every setting (base §4.3). */}}
{{- define "kaddie.rules.config-update" -}}
- apiGroups: [kaddie.org]
  resources: [sreconfigs]
  verbs: [update, patch]
{{- end -}}

{{- define "kaddie.rules.trigger-manage" -}}
- apiGroups: [kaddie.org]
  resources: [sretriggers]
  verbs: [create, update, patch, delete]
{{- end -}}

{{- define "kaddie.rules.read-scope-edit" -}}
- apiGroups: [rbac.authorization.k8s.io]
  resources: [clusterroles]
  resourceNames: [kaddie-agent-read]
  verbs: [get, update, patch]
{{- end -}}

{{/* sresessions is a virtual resource no CRD backs; kaddie-sessions checks
it with SubjectAccessReview (Architecture §4.4). */}}
{{- define "kaddie.rules.session-use" -}}
- apiGroups: [kaddie.org]
  resources: [sresessions]
  verbs: [create, get, list]
{{- end -}}

{{- define "kaddie.rules.session-delete" -}}
- apiGroups: [kaddie.org]
  resources: [sresessions]
  verbs: [delete]
{{- end -}}

{{- define "kaddie.rules.token-review" -}}
- apiGroups: [authentication.k8s.io]
  resources: [tokenreviews]
  verbs: [create]
{{- end -}}

{{- define "kaddie.rules.access-review" -}}
- apiGroups: [authorization.k8s.io]
  resources: [subjectaccessreviews]
  verbs: [create]
{{- end -}}

{{/* lease(<name>): create can't be restricted by name, so it applies to any
lease; get/update only the component's own lease, so no component can renew
another's (base §5). The lease itself lives in kaddie-system. */}}
{{- define "kaddie.rules.lease" -}}
- apiGroups: [coordination.k8s.io]
  resources: [leases]
  verbs: [create]
- apiGroups: [coordination.k8s.io]
  resources: [leases]
  resourceNames: [{{ . }}]
  verbs: [get, update]
{{- end -}}
