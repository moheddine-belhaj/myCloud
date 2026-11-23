{{/* Name = Helm release name ("redis"). */}}
{{- define "redis.name" -}}
{{- .Release.Name -}}
{{- end -}}

{{/* Labels used by the selector. Must never change after the first deploy. */}}
{{- define "redis.selectorLabels" -}}
app.kubernetes.io/name: {{ include "redis.name" . }}
{{- end -}}

{{/* Full label set for every object. */}}
{{- define "redis.labels" -}}
{{ include "redis.selectorLabels" . }}
app.kubernetes.io/part-of: mycloud
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Values.image.tag | quote }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
env: {{ required "environment is required" .Values.environment }}
{{- end -}}
