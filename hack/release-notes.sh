#!/usr/bin/env bash
# Prints the release notes for a version: its section of CHANGELOG.md, without the version heading. Fails if the
# changelog has no section, or an empty one, for the version, so that every release has notes.
#
# Usage: hack/release-notes.sh <version> [changelog]
set -euo pipefail

version="${1:?usage: $0 <version> [changelog]}"
changelog="${2:-CHANGELOG.md}"

# The section runs from "## <version>" to the next "## " heading. Headings inside fenced code blocks are ignored.
notes=$(awk -v heading="## $version" '
  { sub(/\r$/, "") }
  /^```/ { fenced = !fenced }
  !fenced && /^## / { if (found) exit; if ($0 == heading) { found = 1; next } }
  found { print }
' "$changelog")

# Trim leading and trailing blank lines
notes=$(printf '%s\n' "$notes" | sed -e '/./,$!d' | sed -e ':a' -e '/^\n*$/{$d;N;ba' -e '}')

if [ -z "$notes" ]; then
  echo "error: $changelog has no notes for version $version. Add a '## $version' section before releasing." >&2
  exit 1
fi
printf '%s\n' "$notes"
