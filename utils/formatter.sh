#!/usr/bin/env bash

set -euo pipefail

# Initialize variables
ALEJANDRA_ARGS=()
PRETTIER_ARGS=()
SHFMT_ARGS=("-i" "2")
GOPLS_ARGS=()

# Check if "-c" is present in any argument
CHECK_MODE=false
for arg in "$@"; do
  if [ "$arg" = "-c" ]; then
    CHECK_MODE=true
    break
  fi
done

# Adjust arguments based on CHECK_MODE
if $CHECK_MODE; then
  ALEJANDRA_ARGS+=("-c")
  PRETTIER_ARGS+=("--check")
  SHFMT_ARGS+=("-d") # Use diff mode (don't write changes)
else
  PRETTIER_ARGS+=("--write")
  SHFMT_ARGS+=("-w")
  GOPLS_ARGS+=("-w")
fi

# Format all nix files
find . -type f -name "*.nix" -exec alejandra "${ALEJANDRA_ARGS[@]}" {} +

# Format all markdown files using Prettier
find . -type f -name "*.md" -exec prettier "${PRETTIER_ARGS[@]}" {} +

# Format all yaml files using Prettier
find . -type f -name "*.yml" -exec prettier "${PRETTIER_ARGS[@]}" {} +

# Format all json files using Prettier
find . -type f -name "*.json" -exec prettier "${PRETTIER_ARGS[@]}" {} +

# Format all shell files
find . -type f -name "*.sh" -exec shfmt "${SHFMT_ARGS[@]}" {} +

# Format go files using gopls
readarray -d '' GO_FILES < <(find . -type f -name '*.go' -print0)

if ((${#GO_FILES[@]})); then
  if $CHECK_MODE; then
    if gopls format -d "${GO_FILES[@]}" | grep -q .; then
      echo "Go files not formatted"
      exit 1
    fi
  else
    gopls format -w "${GO_FILES[@]}"
  fi
fi
