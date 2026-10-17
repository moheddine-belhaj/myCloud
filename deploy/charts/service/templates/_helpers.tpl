{{/* Service name = Helm release name (auth, space, ...). */}}
{{- define "service.name" -}}
{{- .Release.Name -}}
{{- end -}}

{{/* Labels used by the selector. Must never change after the first deploy. */}}
{{- define "service.selectorLabels" -}}
app.kubernetes.io/name: {{ include "service.name" . }}
{{- end -}}

{{/* Full label set for every object. */}}
{{- define "service.labels" -}}
{{ include "service.selectorLabels" . }}
app.kubernetes.io/part-of: mycloud
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Values.image.tag | quote }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
env: {{ required "environment is required" .Values.environment }}
{{- end -}}