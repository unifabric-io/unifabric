#!/usr/bin/env bash

set -euo pipefail

copyright='// Copyright 2026 Authors of unifabric-io'
spdx='// SPDX-License-Identifier: Apache-2.0'

missing=()

while IFS= read -r -d '' file; do
	line1="$(sed -n '1p' "$file")"
	line2="$(sed -n '2p' "$file")"
	line3="$(sed -n '3p' "$file")"
	line4="$(sed -n '4p' "$file")"

	if [[ "$line1" == "$copyright" && "$line2" == "$spdx" ]]; then
		continue
	fi

	if [[ "$line1" == //go:build* && "$line3" == "$copyright" && "$line4" == "$spdx" ]]; then
		continue
	fi

	missing+=("$file")
done < <(find . \
	-name .git -prune -o \
	-name vendor -prune -o \
	-name '*.go' -print0)

if (( ${#missing[@]} > 0 )); then
	echo "The following Go files are missing the required license header:"
	printf '  %s\n' "${missing[@]}"
	echo
	echo "Expected header:"
	echo "$copyright"
	echo "$spdx"
	exit 1
fi

echo "All Go files have the required license header."
