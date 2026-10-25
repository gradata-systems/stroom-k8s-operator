{{/*
Expand the name of the chart.
*/}}
{{- define "stroom-operator.name" -}}
{{- default .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "stroom-operator.fullname" -}}
{{- $name := .Chart.Name }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "stroom-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Operator image. If a registry is specified, it replaces the registry in the image repository.
*/}}
{{- define "stroom-operator.image" -}}
{{- $repository := .Values.image.repository }}
{{- if .Values.registry }}
{{- $repository = printf "%s/%s" .Values.registry (regexReplaceAll "^[^/]*[.:][^/]*/" $repository "") }}
{{- end }}
{{- printf "%s:%s" $repository (.Values.image.tag | default .Chart.AppVersion) }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "stroom-operator.labels" -}}
helm.sh/chart: {{ include "stroom-operator.chart" . }}
{{ include "stroom-operator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "stroom-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "stroom-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
