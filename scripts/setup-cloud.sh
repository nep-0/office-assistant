#!/usr/bin/env bash
set -euo pipefail

container_engine="${CONTAINER_ENGINE:-podman}"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ -z "${CHAT_API_KEY:-${OPENROUTER_API_KEY:-}}" ]]; then
  echo "Set CHAT_API_KEY or OPENROUTER_API_KEY before starting cloud mode." >&2
  exit 1
fi

export FAKE_PROVIDERS="${FAKE_PROVIDERS:-false}"
export CHAT_PROVIDER_BASE_URL="${CHAT_PROVIDER_BASE_URL:-https://openrouter.ai/api/v1}"
export CHAT_MODEL="${CHAT_MODEL:-poolside/laguna-xs.2}"
export CHAT_API_KEY="${CHAT_API_KEY:-${OPENROUTER_API_KEY:-}}"
export EMBEDDING_PROVIDER_BASE_URL="${EMBEDDING_PROVIDER_BASE_URL:-$CHAT_PROVIDER_BASE_URL}"
export EMBEDDING_MODEL="${EMBEDDING_MODEL:-qwen/qwen3-embedding-8b}"
export EMBEDDING_API_KEY="${EMBEDDING_API_KEY:-$CHAT_API_KEY}"

echo "Starting cloud provider stack with $container_engine compose."
echo "Chat model: $CHAT_MODEL"
echo "Embedding model: $EMBEDDING_MODEL"

cd "$repo_root"
"$container_engine" compose up --build "$@"
