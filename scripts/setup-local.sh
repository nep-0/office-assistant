#!/usr/bin/env bash
set -euo pipefail

container_engine="${CONTAINER_ENGINE:-podman}"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

export MODEL_DIR="${MODEL_DIR:-/home/jeff/models}"
export LLM_GGUF="${LLM_GGUF:-MiniCPM5-1B-Q4_K_M.gguf}"
export EMBEDDING_GGUF="${EMBEDDING_GGUF:-embeddinggemma-300m-qat-Q8_0.gguf}"
export LLAMA_CPP_IMAGE="${LLAMA_CPP_IMAGE:-ghcr.io/ggml-org/llama.cpp:server-b9717}"

chat_model_path="$MODEL_DIR/$LLM_GGUF"
embedding_model_path="$MODEL_DIR/$EMBEDDING_GGUF"

if [[ ! -f "$chat_model_path" ]]; then
  echo "Missing chat GGUF: $chat_model_path" >&2
  echo "Set MODEL_DIR and LLM_GGUF, or copy the file to that path." >&2
  exit 1
fi

if [[ ! -f "$embedding_model_path" ]]; then
  echo "Missing embedding GGUF: $embedding_model_path" >&2
  echo "Set MODEL_DIR and EMBEDDING_GGUF, or copy the file to that path." >&2
  exit 1
fi

export FAKE_PROVIDERS="${FAKE_PROVIDERS:-false}"
export CHAT_PROVIDER_BASE_URL="${CHAT_PROVIDER_BASE_URL:-http://llm:8083/v1}"
export CHAT_MODEL="${CHAT_MODEL:-local-chat}"
export CHAT_API_KEY="${CHAT_API_KEY:-}"
export EMBEDDING_PROVIDER_BASE_URL="${EMBEDDING_PROVIDER_BASE_URL:-http://embedding:8084/v1}"
export EMBEDDING_MODEL="${EMBEDDING_MODEL:-local-embedding}"
export EMBEDDING_API_KEY="${EMBEDDING_API_KEY:-}"

echo "Starting local-model stack with $container_engine compose."
echo "llama.cpp image: $LLAMA_CPP_IMAGE"
echo "Chat GGUF: $chat_model_path"
echo "Embedding GGUF: $embedding_model_path"

cd "$repo_root"
"$container_engine" compose --profile local-models up --build "$@"
