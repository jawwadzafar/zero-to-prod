{{/* Common name and labels, shared by every template. */}}
{{- define "snip.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "snip.labels" -}}
app.kubernetes.io/part-of: snip
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end -}}

{{/* Selector labels for one component: api, worker, postgres, redis. */}}
{{- define "snip.selector" -}}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/name: {{ .component }}
{{- end -}}

{{- define "snip.image" -}}
{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}
{{- end -}}

{{- define "snip.dbSecret" -}}
{{- .Values.database.existingSecret | default (printf "%s-db" (include "snip.fullname" .)) -}}
{{- end -}}

{{/* The schema owner's login for migrations; empty = the API migrates with its own. */}}
{{- define "snip.migrateSecret" -}}
{{- if .Values.database.existingMigrateSecret -}}
{{- .Values.database.existingMigrateSecret -}}
{{- else if and (not .Values.database.existingSecret) .Values.postgres.enabled -}}
{{- printf "%s-db-migrate" (include "snip.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "snip.redisSecret" -}}
{{- .Values.redis.existingSecret | default (printf "%s-redis" (include "snip.fullname" .)) -}}
{{- end -}}

{{/* The same hardened settings for every snip container (chapter 9.4). */}}
{{- define "snip.podSecurity" -}}
runAsNonRoot: true
runAsUser: 65532
runAsGroup: 65532
seccompProfile:
  type: RuntimeDefault
{{- end -}}
{{- define "snip.containerSecurity" -}}
allowPrivilegeEscalation: false
readOnlyRootFilesystem: true
capabilities:
  drop: ["ALL"]
{{- end -}}
