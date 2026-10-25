#!/bin/bash
# Prepares an operator installed from the all-in-one manifest for Helm to adopt, with the commands in
# docs/operations/upgrading.md#from-the-all-in-one-manifest, which are copied below. Keep the two the same.
# Usage: adopt-manifest-install.sh <kubectl context>
set -euo pipefail
context="$1"
kubectl() { command kubectl --context "$context" "$@"; }

ns=stroom-operator-system

# The CRDs belong to the stroom-operator-crds release
for crd in stroomclusters databaseservers databasebackups stroomtaskautoscalers; do
  kubectl annotate --overwrite "crd/$crd.stroom.gchq.github.io" \
    meta.helm.sh/release-name=stroom-operator-crds meta.helm.sh/release-namespace="$ns"
  kubectl label --overwrite "crd/$crd.stroom.gchq.github.io" app.kubernetes.io/managed-by=Helm
done

# The controller and its RBAC belong to the stroom-operator release
for resource in $(kubectl get clusterroles,clusterrolebindings -o name | grep '/stroom-operator-'); do
  kubectl annotate --overwrite "$resource" meta.helm.sh/release-name=stroom-operator meta.helm.sh/release-namespace="$ns"
  kubectl label --overwrite "$resource" app.kubernetes.io/managed-by=Helm
done
for resource in serviceaccount/stroom-operator-controller-manager role/stroom-operator-leader-election-role \
    rolebinding/stroom-operator-leader-election-rolebinding deployment/stroom-operator-controller-manager; do
  kubectl -n "$ns" annotate --overwrite "$resource" meta.helm.sh/release-name=stroom-operator meta.helm.sh/release-namespace="$ns"
  kubectl -n "$ns" label --overwrite "$resource" app.kubernetes.io/managed-by=Helm
done
