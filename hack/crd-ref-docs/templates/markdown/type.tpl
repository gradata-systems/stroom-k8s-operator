{{- define "type" -}}
{{- $type := . -}}
{{- if markdownShouldRenderType $type -}}

### {{ $type.Name }}

{{ if $type.IsAlias -}}
_Underlying type:_ {{ markdownRenderTypeLink $type.UnderlyingType }}
{{ end }}
{{ $type.Doc }}

{{ if $type.Validation -}}
_Validation:_ {{ range $i, $v := $type.Validation }}{{ if $i }}, {{ end }}{{ markdownRenderFieldDoc $v | replace ": \\{\\}" "" | replace "\\{" "{" | replace "\\}" "}" }}{{ end }}
{{ end }}
{{ if $type.References -}}
_Appears in:_ {{ range $i, $ref := $type.SortedReferences }}{{ if $i }}, {{ end }}{{ markdownRenderTypeLink $ref }}{{ end }}
{{ end }}
{{ if $type.Members -}}
| Field | Description | Default | Validation |
| --- | --- | --- | --- |
{{ if $type.GVK -}}
| `apiVersion` _string_ | `{{ $type.GVK.Group }}/{{ $type.GVK.Version }}` | | |
| `kind` _string_ | `{{ $type.GVK.Kind }}` | | |
{{ end -}}
{{ range $type.Members -}}
| `{{ .Name }}` _{{ markdownRenderType .Type }}_ | {{ template "type_members" . }} | {{ with .Default }}`{{ . }}`{{ end }} | {{ range $i, $v := .Validation }}{{ if $i }}<br />{{ end }}{{ markdownRenderFieldDoc $v | replace ": \\{\\}" "" | replace "\\{" "{" | replace "\\}" "}" }}{{ end }} |
{{ end -}}
{{ end -}}

{{ if $type.EnumValues -}}
| Value | Description |
| --- | --- |
{{ range $type.EnumValues -}}
| `{{ .Name }}` | {{ markdownRenderFieldDoc .Doc }} |
{{ end -}}
{{ end -}}

{{- end -}}
{{- end -}}
