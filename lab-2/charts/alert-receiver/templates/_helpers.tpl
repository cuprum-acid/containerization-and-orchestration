{{/* Selector labels: never change after the first install (immutable selector). */}}
{{- define "alert-receiver.selectorLabels" -}}
app.kubernetes.io/name: alert-receiver
{{- end -}}

{{/* Standard labels for every object. */}}
{{- define "alert-receiver.labels" -}}
{{ include "alert-receiver.selectorLabels" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}
