{{/*
Kubernetes version gate (base §13, T53). Chart.yaml's kubeVersion already
makes Helm refuse an older cluster; this repeats the check with a message
that names the requirement. Renderers that template without a live cluster,
such as reconcilers, must pass the cluster's version with --kube-version.
Included from namespace.yaml, which every render contains.
*/}}
{{- define "kaddie.versionGate" -}}
{{- if semverCompare "<1.28.0-0" .Capabilities.KubeVersion.Version -}}
{{- fail (printf "kaddie requires Kubernetes 1.28 or later (SelfSubjectReview must be GA); this cluster reports %s" .Capabilities.KubeVersion.Version) -}}
{{- end -}}
{{- end -}}
