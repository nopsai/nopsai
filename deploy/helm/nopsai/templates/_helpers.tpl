{{- define "nopsai.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name (.Chart.Version | replace "+" "_") }}
app.kubernetes.io/name: nopsai
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
nopsai.io/release-version: {{ .Values.global.releaseVersion | quote }}
{{- end }}

{{- define "nopsai.selectorLabels" -}}
app.kubernetes.io/name: nopsai
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{- define "nopsai.image" -}}
{{- if .digest -}}
{{ printf "%s@%s" .repository .digest }}
{{- else -}}
{{ printf "%s:%s" .repository .tag }}
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

{{- define "nopsai.systemLogsKubernetesEnabled" -}}
{{- if and .Values.systemLogs.enabled (or (eq .Values.systemLogs.provider "kubernetes") (eq .Values.systemLogs.provider "k8s")) -}}true{{- else -}}false{{- end -}}
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
