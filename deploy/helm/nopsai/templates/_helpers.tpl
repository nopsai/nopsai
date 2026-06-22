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

{{- define "nopsai.runnerServiceAccountName" -}}
{{- if .Values.k8sRunner.serviceAccount.create -}}
{{ .Values.k8sRunner.serviceAccount.name | default "nopsai-runner" }}
{{- else -}}
{{ required "k8sRunner.serviceAccount.name is required when service-account creation is disabled" .Values.k8sRunner.serviceAccount.name }}
{{- end -}}
{{- end }}
