{{/*
Shared helpers. Every object lives in the fixed namespace kaddie-system
(base §3.2), whatever namespace the Helm release itself is stored in.
*/}}

{{- define "kaddie.namespace" -}}kaddie-system{{- end -}}

{{- define "kaddie.labels" -}}
app.kubernetes.io/part-of: kaddie
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ .Release.Name }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{/* Component ServiceAccounts, in the order the chart renders them. */}}
{{- define "kaddie.components" -}}
{{- $c := list "kaddie-agent" "kaddie-sessions" "kaddie-tools" "kaddie-ui" -}}
{{- if .Values.gitops.enabled -}}
{{- $c = concat $c (list "kaddie-git-writer" "kaddie-renderer") -}}
{{- end -}}
{{- toJson $c -}}
{{- end -}}
