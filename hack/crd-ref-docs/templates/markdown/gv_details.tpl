{{- define "gvDetails" -}}
{{- $gv := . -}}
{{- $summaries := dict
  "StroomCluster" "A Stroom cluster: one StatefulSet of Stroom nodes per NodeSet, plus the Services, Ingresses and configuration they need."
  "DatabaseServer" "An operator-managed, single-instance MySQL server for a StroomCluster."
  "DatabaseBackup" "A scheduled `mysqldump` of a DatabaseServer or external MySQL database, run as a CronJob."
  "StroomTaskAutoscaler" "Adjusts the task limit of a Stroom job on each processing node, based on Pod CPU usage."
-}}

## Resource types

All resources belong to the API group and version `{{ $gv.GroupVersionString }}`.

| Kind | Description |
| --- | --- |
{{ range $gv.SortedKinds -}}
{{- $type := $gv.TypeForKind . -}}
| {{ markdownRenderTypeLink $type }} | {{ if hasKey $summaries $type.Name }}{{ get $summaries $type.Name }}{{ else }}{{ markdownRenderFieldDoc $type.Doc }}{{ end }} |
{{ end }}

{{- range $gv.SortedKinds }}
{{ template "type" ($gv.TypeForKind .) }}
{{ end }}

## Supporting types

Types used within the resources above, in alphabetical order.

{{ range $gv.SortedTypes -}}
{{- if not .GVK }}
{{ template "type" . }}
{{ end -}}
{{- end }}

{{- end -}}
