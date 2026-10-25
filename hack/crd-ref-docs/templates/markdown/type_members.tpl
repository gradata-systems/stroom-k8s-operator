{{- define "type_members" -}}
{{- $field := . -}}
{{- if eq $field.Name "metadata" -}}
Standard Kubernetes object metadata.
{{- else if $field.Embedded -}}
_Fields of this embedded type are included inline._ {{ markdownRenderFieldDoc $field.Doc }}
{{- else -}}
{{ markdownRenderFieldDoc $field.Doc }}
{{- end -}}
{{- end -}}
