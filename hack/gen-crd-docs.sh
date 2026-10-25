#!/usr/bin/env bash
#
# Generates the CRD reference documentation (docs/reference/api.md) from the Go API types in api/v1, using
# elastic/crd-ref-docs (https://github.com/elastic/crd-ref-docs).
#
# A pinned version of crd-ref-docs is installed into ./bin (or $LOCALBIN) on first use. Requires Go and bash.
#
# Usage: hack/gen-crd-docs.sh
#
# Re-run this whenever anything under api/v1 changes and commit the result. CI fails if docs/reference is stale.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CRD_REF_DOCS_VERSION="${CRD_REF_DOCS_VERSION:-v0.3.0}"
LOCALBIN="${LOCALBIN:-${REPO_ROOT}/bin}"
CRD_REF_DOCS="${LOCALBIN}/crd-ref-docs-${CRD_REF_DOCS_VERSION}"
CONFIG_DIR="hack/crd-ref-docs"
OUTPUT_FILE="docs/reference/api.md"

cd "${REPO_ROOT}"

if [[ ! -x "${CRD_REF_DOCS}" ]]; then
  echo "Installing crd-ref-docs ${CRD_REF_DOCS_VERSION} into ${LOCALBIN}"
  mkdir -p "${LOCALBIN}"
  tmp_bin="$(mktemp -d)"
  trap 'rm -rf "${tmp_bin}"' EXIT
  GOBIN="${tmp_bin}" go install "github.com/elastic/crd-ref-docs@${CRD_REF_DOCS_VERSION}"
  mv "${tmp_bin}/crd-ref-docs" "${CRD_REF_DOCS}"
fi

mkdir -p "$(dirname "${OUTPUT_FILE}")"

"${CRD_REF_DOCS}" \
  --source-path=./api/v1 \
  --config="${CONFIG_DIR}/config.yaml" \
  --renderer=markdown \
  --templates-dir="${CONFIG_DIR}/templates/markdown" \
  --output-mode=single \
  --output-path="${OUTPUT_FILE}" \
  --log-level=ERROR

echo "Wrote ${OUTPUT_FILE}"
