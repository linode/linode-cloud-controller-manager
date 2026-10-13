#!/usr/bin/env bash

set -euo pipefail

status=0
if ! shellcheck --external-sources --shell=bash --severity=warning e2e/test/scripts/*.sh; then
  status=1
fi

while IFS= read -r -d '' test_file; do
  test_dir=${test_file%/*}
  script_blocks=$(yq -o=json \
    '[.. | select(tag == "!!map" and has("script")) | .script.content | select(. != null)]' \
    "$test_file" | jq -c '.[]')

  if [[ -z $script_blocks ]]; then
    continue
  fi

  block_number=0
  while IFS= read -r script_block; do
    block_number=$((block_number + 1))
    printf 'Checking %s script block %s\n' "$test_file" "$block_number" >&2
    if ! jq -r '.' <<<"$script_block" \
      | (cd "$test_dir" && shellcheck --external-sources --shell=bash --severity=warning -); then
      status=1
    fi
  done <<<"$script_blocks"
done < <(find e2e/test -type f -name chainsaw-test.yaml -print0)

exit "$status"
