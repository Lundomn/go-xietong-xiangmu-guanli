{{- define "go-xietong.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "go-xietong.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else if contains (include "go-xietong.name" .) .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name (include "go-xietong.name" .) | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{- define "go-xietong.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "go-xietong.labels" -}}
helm.sh/chart: {{ include "go-xietong.chart" . }}
app.kubernetes.io/name: {{ include "go-xietong.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "go-xietong.selectorLabels" -}}
app.kubernetes.io/name: {{ include "go-xietong.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "go-xietong.secretName" -}}
{{- default (printf "%s-secrets" (include "go-xietong.fullname" .)) .Values.secrets.existingSecret | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "go-xietong.apiServiceName" -}}
{{- default "api" .Values.api.service.name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "go-xietong.projectServiceName" -}}
{{- printf "%s-project" (include "go-xietong.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "go-xietong.userServiceName" -}}
{{- printf "%s-project-user" (include "go-xietong.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "go-xietong.mysqlServiceName" -}}
{{- printf "%s-mysql" (include "go-xietong.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "go-xietong.redisServiceName" -}}
{{- printf "%s-redis" (include "go-xietong.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "go-xietong.etcdServiceName" -}}
{{- printf "%s-etcd" (include "go-xietong.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "go-xietong.mysqlHost" -}}
{{- if .Values.mysql.enabled -}}
{{- include "go-xietong.mysqlServiceName" . -}}
{{- else -}}
{{- required "external.mysql.host is required when mysql.enabled=false" .Values.external.mysql.host -}}
{{- end -}}
{{- end }}

{{- define "go-xietong.redisHost" -}}
{{- if .Values.redis.enabled -}}
{{- include "go-xietong.redisServiceName" . -}}
{{- else -}}
{{- required "external.redis.host is required when redis.enabled=false" .Values.external.redis.host -}}
{{- end -}}
{{- end }}

{{- define "go-xietong.etcdHost" -}}
{{- if .Values.etcd.enabled -}}
{{- include "go-xietong.etcdServiceName" . -}}
{{- else -}}
{{- required "external.etcd.host is required when etcd.enabled=false" .Values.external.etcd.host -}}
{{- end -}}
{{- end }}
