{{- define "nopsai.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name (.Chart.Version | replace "+" "_") }}
app.kubernetes.io/name: nopsai
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
nopsai.io/release-version: {{ .Values.global.releaseVersion | quote }}
nopsai.io/platform-id: {{ include "nopsai.platformID" . | quote }}
{{- end }}

{{- define "nopsai.selectorLabels" -}}
app.kubernetes.io/name: nopsai
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{- define "nopsai.platformID" -}}
{{- $raw := default .Release.Name .Values.global.platformID | lower -}}
{{- $normalized := regexReplaceAll "[^a-z0-9-]+" $raw "-" | trimAll "-" -}}
{{- if eq $normalized "" -}}
nopsai
{{- else if le (len $normalized) 63 -}}
{{- $normalized -}}
{{- else -}}
{{- printf "%s-%s" (trunc 52 $normalized | trimAll "-") (sha256sum $normalized | trunc 10) -}}
{{- end -}}
{{- end }}

{{- define "nopsai.image" -}}
{{- if .digest -}}
{{ printf "%s@%s" .repository .digest }}
{{- else -}}
{{ printf "%s:%s" .repository .tag }}
{{- end -}}
{{- end }}

{{- define "nopsai.imageWithDefaultTag" -}}
{{- $image := .image -}}
{{- if $image.digest -}}
{{ printf "%s@%s" $image.repository $image.digest }}
{{- else -}}
{{- $tag := default .defaultTag $image.tag -}}
{{- $tag = required "global.releaseVersion is required when image tag is empty" $tag -}}
{{ printf "%s:%s" $image.repository $tag }}
{{- end -}}
{{- end }}

{{- define "nopsai.secretName" -}}
{{ required "secrets.existingSecret must name a Secret containing the NopsAI deployment keys" .Values.secrets.existingSecret }}
{{- end }}

{{- define "nopsai.apiServiceAccountName" -}}
{{- if .Values.api.serviceAccount.create -}}
{{ .Values.api.serviceAccount.name | default "nopsai-api" }}
{{- else -}}
{{ required "api.serviceAccount.name is required when API service-account creation is disabled" .Values.api.serviceAccount.name }}
{{- end -}}
{{- end }}

{{- define "nopsai.runnerServiceAccountName" -}}
{{- if .Values.k8sRunner.serviceAccount.create -}}
{{ .Values.k8sRunner.serviceAccount.name | default "nopsai-runner" }}
{{- else -}}
{{ required "k8sRunner.serviceAccount.name is required when service-account creation is disabled" .Values.k8sRunner.serviceAccount.name }}
{{- end -}}
{{- end }}

{{- define "nopsai.runnerWorkloadServiceAccountName" -}}
{{- $workload := default dict .Values.k8sRunner.workload -}}
{{- $serviceAccount := default dict (index $workload "serviceAccount") -}}
{{- $create := default true (index $serviceAccount "create") -}}
{{- $fallbackName := printf "%s-workload" (include "nopsai.runnerServiceAccountName" .) | trunc 63 | trimSuffix "-" -}}
{{- if $create -}}
{{ default $fallbackName (index $serviceAccount "name") }}
{{- else -}}
{{ required "k8sRunner.workload.serviceAccount.name is required when workload service-account creation is disabled" (index $serviceAccount "name") }}
{{- end -}}
{{- end }}

{{- define "nopsai.runnerServiceID" -}}
{{- default .Values.k8sRunner.runnerID .Values.k8sRunner.serviceID -}}
{{- end }}

{{- define "nopsai.imagePullSecretNames" -}}
{{- $names := list -}}
{{- range . -}}
{{- if kindIs "string" . -}}
{{- $names = append $names . -}}
{{- else if hasKey . "name" -}}
{{- $names = append $names .name -}}
{{- end -}}
{{- end -}}
{{- join "," $names -}}
{{- end }}

{{- define "nopsai.postgresServiceName" -}}
{{- default "postgres" .Values.postgres.service.name -}}
{{- end }}

{{- define "nopsai.postgresSecretName" -}}
{{- default (include "nopsai.secretName" .) .Values.postgres.auth.existingSecret -}}
{{- end }}

{{- define "nopsai.postgresPasswordKey" -}}
{{- default .Values.postgres.auth.passwordKey .Values.secrets.keys.postgresPassword | required "secrets.keys.postgresPassword or postgres.auth.passwordKey is required when postgres.enabled=true" -}}
{{- end }}

{{- define "nopsai.postgresInitConfigMapName" -}}
{{- printf "%s-postgres-init" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{- define "nopsai.systemLogsKubernetesEnabled" -}}
{{- $provider := lower (default "" .Values.systemLogs.provider) -}}
{{- if and .Values.systemLogs.enabled (regexMatch "(^|[,;[:space:]])(kubernetes|k8s)([,;[:space:]]|$)" $provider) -}}true{{- else -}}false{{- end -}}
{{- end }}

{{- define "nopsai.systemLogsKubernetesLabelSelector" -}}
{{- if .Values.systemLogs.kubernetes.labelSelector -}}
{{ .Values.systemLogs.kubernetes.labelSelector }}
{{- else -}}
app.kubernetes.io/name=nopsai,app.kubernetes.io/instance={{ .Release.Name }}
{{- end -}}
{{- end }}

{{- define "nopsai.topology.dispatcherGRPCAddress" -}}
{{- $topology := default dict .Values.topology -}}
{{- default "dispatcher:9090" (index $topology "dispatcherGRPCAddress") -}}
{{- end }}

{{- define "nopsai.topology.nopsaiAPIURL" -}}
{{- $topology := default dict .Values.topology -}}
{{- default "http://nopsai:8080" (index $topology "nopsaiAPIURL") -}}
{{- end }}

{{- define "nopsai.topology.aaaAPIURL" -}}
{{- $topology := default dict .Values.topology -}}
{{- default "http://aaa:8082" (index $topology "aaaAPIURL") -}}
{{- end }}

{{- define "nopsai.topology.gitBotAPIURL" -}}
{{- $topology := default dict .Values.topology -}}
{{- default "http://git-bot:8081" (index $topology "gitBotAPIURL") -}}
{{- end }}

{{- define "nopsai.topology.gotenbergURL" -}}
{{- $topology := default dict .Values.topology -}}
{{- default "http://gotenberg:3000" (index $topology "gotenbergURL") -}}
{{- end }}
