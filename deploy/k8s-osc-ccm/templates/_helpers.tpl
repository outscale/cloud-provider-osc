{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "osc-cloud-controller-manager.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "osc-cloud-controller-manager.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "osc-cloud-controller-manager.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "osc-cloud-controller-manager.labels" -}}
app.kubernetes.io/name: {{ include "osc-cloud-controller-manager.name" . }}
helm.sh/chart: {{ include "osc-cloud-controller-manager.chart" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Create the name of the service account to use
*/}}
{{- define "osc-cloud-controller-manager.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
    {{ default (include "osc-cloud-controller-manager.fullname" .) .Values.serviceAccount.name }}
{{- else -}}
    {{ default "default" .Values.serviceAccount.name }}
{{- end -}}
{{- end -}}

{{/*
Convert the `--extra-loadbalancer-tags` command line arg from a map.
*/}}
{{- define "osc-cloud-controller-manager.extra-loadbalancer-tags" -}}
{{- $result := dict "pairs" (list) -}}
{{- range $key, $value := .Values.extraLoadBalancerTags -}}
{{- $noop := printf "%s=%s" $key $value | append $result.pairs | set $result "pairs" -}}
{{- end -}}
{{- if gt (len $result.pairs) 0 -}}
{{- printf "%s=%s" "- --extra-loadbalancer-tags" (join "," $result.pairs) -}}
{{- end -}}
{{- end -}}


{{/*
Convert the `--extra-node-labels` command line arg from a map.
*/}}
{{- define "osc-cloud-controller-manager.extra-node-labels" -}}
{{- $result := dict "pairs" (list) -}}
{{- range $key, $value := .Values.extraNodeLabels -}}
{{- $noop := printf "%s=%s" $key $value | append $result.pairs | set $result "pairs" -}}
{{- end -}}
{{- if gt (len $result.pairs) 0 -}}
{{- printf "%s=%s" "- --extra-node-labels" (join "," $result.pairs) -}}
{{- end -}}
{{- end -}}

