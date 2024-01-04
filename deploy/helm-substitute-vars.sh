#!/bin/bash

operator_image_tag="$1"
version="$2"

chart_yaml='./charts/stroom-operator/Chart.yaml'
operator_yaml='./charts/stroom-operator/templates/operator.yaml'

sed -i -E "s/appVersion: \"(.*)\"/appVersion: \"$version\"/" $chart_yaml

registry="{{ if .Values.registry }}{{ printf \"%s\/\" .Values.registry }}{{ end }}"
sed -i -E "s|(image: .*$operator_image_tag.*):.+$|\1:{{ .Values.image.tag \| default .Chart.AppVersion }}|" $operator_yaml
sed -i -E "s/(image): (.+):/\1: $registry{{ \"\2\" }}:/" $operator_yaml

sed -i 's/HELM_NAMESPACE/{{ .Release.Namespace }}/' $operator_yaml
sed -i 's/HELM_LABELS: ""/{{ include "stroom-operator.labels" . | nindent 4 }}/' $operator_yaml
sed -i 's/HELM_IMAGE_PULL_POLICY/{{ .Values.image.pullPolicy }}/' $operator_yaml
sed -i 's/HELM_RESOURCES/{{ toYaml .Values.resources | nindent 10 }}/' $operator_yaml
sed -i 's/HELM_NODE_SELECTOR/{{ toYaml .Values.nodeSelector | nindent 8 }}/' $operator_yaml
sed -i 's/HELM_TOLERATIONS/{{ toYaml .Values.tolerations | nindent 8 }}/' $operator_yaml
sed -i 's/HELM_AFFINITY/{{ toYaml .Values.affinity | nindent 8 }}/' $operator_yaml
sed -i 's/HELM_POD_SECURITY_CONTEXT/{{ toYaml .Values.securityContext | nindent 8 }}/' $operator_yaml
