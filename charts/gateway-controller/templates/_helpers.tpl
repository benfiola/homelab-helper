{{- define "gateway-controller.name" -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}