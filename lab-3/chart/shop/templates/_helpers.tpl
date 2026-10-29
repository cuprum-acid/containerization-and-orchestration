{{/*
Selector labels of one component (api or worker). Never change them after the
first install: a Deployment's selector is immutable.
Call with: (dict "root" $ "component" "api")
*/}}
{{- define "shop.selectorLabels" -}}
app.kubernetes.io/name: {{ .component }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
{{- end -}}

{{/*
All labels of one component. app.kubernetes.io/name and
app.kubernetes.io/part-of are required by the require-labels policy.
*/}}
{{- define "shop.labels" -}}
{{ include "shop.selectorLabels" . }}
app.kubernetes.io/part-of: shop
app.kubernetes.io/component: {{ .component }}
app.kubernetes.io/version: {{ (index .root.Values .component).image.tag | quote }}
app.kubernetes.io/managed-by: {{ .root.Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .root.Chart.Name .root.Chart.Version | replace "+" "_" }}
{{- end -}}

{{/*
Security settings every container gets. runAsNonRoot is required by the
require-run-as-non-root policy and privileged must not be set; the rest is
what lab 1 showed a container does not need.
*/}}
{{- define "shop.containerSecurityContext" -}}
runAsNonRoot: true
runAsUser: 65534
runAsGroup: 65534
allowPrivilegeEscalation: false
readOnlyRootFilesystem: true
capabilities:
  drop: [ALL]
seccompProfile:
  type: RuntimeDefault
{{- end -}}

{{/* DATABASE_URL from the Secret CloudNativePG creates for the app user. */}}
{{- define "shop.databaseEnv" -}}
{{- if .Values.database.enabled }}
- name: DATABASE_URL
  valueFrom:
    secretKeyRef:
      name: {{ .Release.Name }}-db-app
      key: uri
{{- end }}
{{- end -}}
