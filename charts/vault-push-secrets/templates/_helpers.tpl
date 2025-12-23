{{- define "vault-push-secrets.name" -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "vault-push-secrets.serviceAccountName" -}}
{{ default (include "vault-push-secrets.name" .) .Values.serviceAccount.name }}
{{- end }}