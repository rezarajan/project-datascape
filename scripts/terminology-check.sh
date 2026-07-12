#!/usr/bin/env bash
set -euo pipefail

content_paths=(
  README.md
  docs
  examples
  profiles
  schemas
  Makefile
  justfile
  internal/docsgen
  internal/render
  internal/adapters/targets/compose
)

content_pattern='Milestone|milestone|M2-|M3-|ComponentCatalogue|PostgresSource|EventSource|RawArchive|\bSourceBinding\b|\bProducerBinding\b|\bArchiveBinding\b|Eventflow|eventflow|Dapr|dapr'
if rg -n "$content_pattern" "${content_paths[@]}"; then
  echo "legacy product terminology found" >&2
  exit 1
fi

path_pattern='milestone|postgres-cdc|component-catalogue|eventflow|dapr|raw-archive'
if rg --files "${content_paths[@]}" | rg -n "$path_pattern"; then
  echo "legacy product terminology found in paths" >&2
  exit 1
fi
