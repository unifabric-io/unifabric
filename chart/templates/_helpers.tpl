{{/* vim: set filetype=mustache: */}}
{{- define "unifabric.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "unifabric.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := include "unifabric.name" . -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "unifabric.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" -}}
{{- end -}}

{{- define "unifabric.labels" -}}
helm.sh/chart: {{ include "unifabric.chart" . }}
app.kubernetes.io/name: {{ include "unifabric.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "unifabric.selectorLabels" -}}
app.kubernetes.io/name: {{ include "unifabric.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "unifabric.controllerName" -}}
{{- printf "%s-controller" (include "unifabric.fullname" .) -}}
{{- end -}}

{{- define "unifabric.agentName" -}}
{{- printf "%s-agent" (include "unifabric.fullname" .) -}}
{{- end -}}

{{- define "unifabric.controllerServiceAccountName" -}}
{{- default (include "unifabric.controllerName" .) .Values.controller.serviceAccount.name -}}
{{- end -}}

{{- define "unifabric.agentServiceAccountName" -}}
{{- default (include "unifabric.agentName" .) .Values.agent.serviceAccount.name -}}
{{- end -}}

{{- define "unifabric.render" -}}
{{- if typeIs "string" .value -}}
{{- tpl .value .context -}}
{{- else -}}
{{- tpl (.value | toYaml) .context -}}
{{- end -}}
{{- end -}}
