# Contributing to the docs

This site is built with [MkDocs](https://www.mkdocs.org/) and
[Material for MkDocs](https://squidfunk.github.io/mkdocs-material/) from the Markdown files in
[`docs/`](https://github.com/gradata-systems/stroom-k8s-operator/tree/master/docs), and published to GitHub Pages
from the `master` branch. Every page has an edit button that opens the file on GitHub.

!!! note "Material for MkDocs end of life"
    Material for MkDocs reaches end of life on 5 May 2027, after which it gets no fixes. Its maintainers' successor,
    [Zensical](https://zensical.org/), reads the existing `mkdocs.yml`, so the site should move to it before then.

## Building the site locally

You need Python 3.11 or later.

```shell
python3 -m venv .venv
. .venv/bin/activate
pip install -r docs/requirements.txt

mkdocs serve            # preview at http://127.0.0.1:8000/, reloading as you edit
mkdocs build --strict   # what CI runs: warnings, broken links and anchors are errors
```

## Writing guidelines

- **Write for people running Stroom**, not for operator developers. Explain what to set, what happens and what to
  check, with commands they can run.
- **Check statements against the code.** The Go types in `api/v1` and the controller in `internal/controller` are
  the source of truth, not older documentation. If you can't verify something, say so or leave it out.
- **Keep YAML examples short and valid** against the CRDs. Link to the full examples in
  [`samples/`](https://github.com/gradata-systems/stroom-k8s-operator/tree/master/samples) instead of copying them.
- **Link between pages with relative paths to the `.md` file**, for example `[Storage](configuration/storage.md)`.
  Link to files elsewhere in the repository with full GitHub URLs; relative links outside `docs/` break the
  build.
- Use [admonitions](https://squidfunk.github.io/mkdocs-material/reference/admonitions/) (`!!! note`,
  `!!! warning`) sparingly, for things readers must not miss.
- Add new pages to `nav` in `mkdocs.yml`; the build warns about pages that aren't in it.

## CRD reference

[CRD reference](reference/api.md) (`docs/reference/api.md`) is generated from the Go types and doc comments in
`api/v1` by [crd-ref-docs](https://github.com/elastic/crd-ref-docs). Don't edit it by hand.

**Whenever anything in `api/v1` changes, regenerate it and commit the result:**

```shell
hack/gen-crd-docs.sh
git add docs/reference
```

The script needs Go and bash. It installs a pinned crd-ref-docs version into `./bin` on first use. The templates
and configuration are in
[`hack/crd-ref-docs/`](https://github.com/gradata-systems/stroom-k8s-operator/tree/master/hack/crd-ref-docs).

To improve a field's description, edit its doc comment in `api/v1` and regenerate. The CI workflow regenerates the
reference and fails if the result differs from what's committed.

## Publishing

The [docs workflow](https://github.com/gradata-systems/stroom-k8s-operator/blob/master/.github/workflows/docs.yml):

- on pull requests that touch the docs, `mkdocs.yml`, `api/` or the generator, checks that the CRD reference is up
  to date and builds the site with `mkdocs build --strict`;
- on pushes to `master`, does the same and deploys the site to GitHub Pages.

## Updating the tooling

- Python packages are pinned in `docs/requirements.txt`.
- The crd-ref-docs version is `CRD_REF_DOCS_VERSION` in `hack/gen-crd-docs.sh`. After changing it, regenerate the
  reference and review the diff.
- The Kubernetes version that core types link to is `render.kubernetesVersion` in
  `hack/crd-ref-docs/config.yaml`. Keep it in step with `k8s.io/api` in `go.mod`.
