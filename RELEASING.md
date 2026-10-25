# Releasing

A release is published by pushing its version tag, such as `1.2.0` (or `1.3.0-rc.1` for a pre-release), to a commit
on `master`. The [release workflow](.github/workflows/release.yml) then:

1. checks that the tag matches the operator version, that the chart versions agree, that the generated CRDs and chart
   templates are up to date, and that `CHANGELOG.md` has a section for the version, then runs the tests;
2. publishes the operator image to `ghcr.io/gradata-systems/stroom-k8s-operator:<version>`;
3. publishes the `stroom-operator` and `stroom-operator-crds` charts to `oci://ghcr.io/gradata-systems/helm-charts`;
4. creates the GitHub release, whose notes are the version's section of `CHANGELOG.md`, with the chart archives
   attached. A version with a pre-release suffix is marked as a pre-release.

Don't create the release in GitHub first. If one exists, the workflow replaces its notes and files.

## Preparing a release

1. Add a `## <version>` section to the top of `CHANGELOG.md`, with upgrade notes first if the release needs any.
   The release fails without it, and so does CI, for the version in the `Makefile`.
2. Set the versions: `VERSION` in the `Makefile`, `version` in both charts' `Chart.yaml` (the chart version, which
   can differ from the operator's), and `appVersion` in `charts/stroom-operator/Chart.yaml`.
3. Regenerate the CRDs and the `stroom-operator-crds` chart's templates:

    ```sh
    make manifests kustomize
    bin/kustomize build config/crd --output charts/stroom-operator-crds/templates/
    ```

4. Merge to `master`, once the tests pass, then tag the merge commit and push the tag:

    ```sh
    git tag -a <version> -m <version>
    git push origin <version>
    ```

To preview a version's release notes: `hack/release-notes.sh <version>`.
