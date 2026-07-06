{{/* vim: set filetype=mustache: */}}
{{- define "unifabric.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "unifabric.fullname" -}}
{{- $name := include "unifabric.name" . -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
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

{{- define "unifabric.switchMTLS.enabled" -}}
{{- if and .Values.switchTopologyDiscovery.enabled .Values.switchTopologyDiscovery.mtls.enabled -}}
true
{{- end -}}
{{- end -}}

{{- define "unifabric.switchMTLS.controllerSecretName" -}}
{{- default (printf "%s-switch-controller-mtls-controller" (include "unifabric.fullname" .)) .Values.switchTopologyDiscovery.mtls.controllerSecretName -}}
{{- end -}}

{{- define "unifabric.switchMTLS.switchAgentSecretName" -}}
{{- default (printf "%s-switch-controller-mtls-agent" (include "unifabric.fullname" .)) .Values.switchTopologyDiscovery.mtls.switchAgentSecretName -}}
{{- end -}}

{{- define "unifabric.render" -}}
{{- if typeIs "string" .value -}}
{{- tpl .value .context -}}
{{- else -}}
{{- tpl (.value | toYaml) .context -}}
{{- end -}}
{{- end -}}

{{- define "unifabric.image" -}}
{{- $root := .root -}}
{{- $image := .image -}}
{{- $defaultRegistry := .defaultRegistry | default "" -}}
{{- $defaultRepository := .defaultRepository | default "" -}}
{{- $defaultTag := .defaultTag | default $root.Chart.AppVersion -}}
{{- $registry := $root.Values.global.registry | default $image.registry | default $defaultRegistry -}}
{{- $repository := $image.repository | default $defaultRepository -}}
{{- $tag := $image.tag | default $defaultTag -}}
{{- printf "%s/%s:%s" $registry $repository $tag -}}
{{- end -}}

{{- define "unifabric.nvidiaTopograph.topographName" -}}
{{- printf "%s-nvidia-topograph" (include "unifabric.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "unifabric.nvidiaTopograph.nodeObserverName" -}}
{{- printf "%s-nvidia-node-observer" (include "unifabric.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "unifabric.nvidiaTopograph.nodeDataBrokerName" -}}
{{- printf "%s-nvidia-node-data-broker" (include "unifabric.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "unifabric.nvidiaTopograph.topographServiceAccountName" -}}
{{- include "unifabric.nvidiaTopograph.topographName" . -}}
{{- end -}}

{{- define "unifabric.nvidiaTopograph.nodeObserverServiceAccountName" -}}
{{- include "unifabric.nvidiaTopograph.nodeObserverName" . -}}
{{- end -}}

{{- define "unifabric.nvidiaTopograph.nodeDataBrokerServiceAccountName" -}}
{{- include "unifabric.nvidiaTopograph.nodeDataBrokerName" . -}}
{{- end -}}

{{- define "unifabric.nvidiaTopograph.selectorLabels" -}}
app.kubernetes.io/name: {{ .name }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "unifabric.nvidiaTopograph.labels" -}}
helm.sh/chart: {{ include "unifabric.chart" .root }}
{{ include "unifabric.nvidiaTopograph.selectorLabels" . }}
app.kubernetes.io/version: {{ .root.Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .root.Release.Service }}
app.kubernetes.io/part-of: {{ include "unifabric.name" .root }}
{{- end -}}

{{- define "unifabric.nvidiaTopograph.url" -}}
{{- $serviceName := default (include "unifabric.nvidiaTopograph.topographName" .) .Values.nvidiaTopograph.topograph.serviceName -}}
{{- printf "http://%s.%s.svc.cluster.local:49021" $serviceName .Release.Namespace -}}
{{- end -}}

{{- define "unifabric.nvidiaTopograph.nodeObserverTrigger" -}}
{{- toYaml (.Values.nvidiaTopograph.nodeObserver.topograph.trigger | default dict) -}}
{{- end -}}

{{- define "unifabric.nvidiaTopograph.nodeDataBrokerNodeSelector" -}}
{{- toYaml (.Values.nvidiaTopograph.nodeDataBroker.nodeSelector | default dict) -}}
{{- end -}}

{{- define "unifabric.nvidiaTopograph.useGpuCliqueLabel" -}}
{{- $providerParams := default dict .Values.nvidiaTopograph.provider.params -}}
{{- if and (eq .Values.nvidiaTopograph.provider.name "infiniband-k8s") (eq (lower (toString (get $providerParams "useGpuCliqueLabel"))) "true") -}}
true
{{- end -}}
{{- end -}}
