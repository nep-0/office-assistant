#!/usr/bin/env bash
set -euo pipefail

container_engine="${1:-podman}"
if [[ "$container_engine" == "podman" || "$container_engine" == "docker" ]]; then
  shift || true
else
  container_engine="podman"
fi
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

model_dir="/home/jeff/models"
llm_gguf="MiniCPM5-1B-Q4_K_M.gguf"
embedding_gguf="embeddinggemma-300m-qat-Q8_0.gguf"

chat_model_path="$model_dir/$llm_gguf"
embedding_model_path="$model_dir/$embedding_gguf"

if [[ ! -f "$chat_model_path" ]]; then
  echo "Missing chat GGUF: $chat_model_path" >&2
  echo "Edit compose.yaml if the model file lives elsewhere." >&2
  exit 1
fi

if [[ ! -f "$embedding_model_path" ]]; then
  echo "Missing embedding GGUF: $embedding_model_path" >&2
  echo "Edit compose.yaml if the model file lives elsewhere." >&2
  exit 1
fi

echo "Starting local-model stack with $container_engine compose."
echo "Chat GGUF: $chat_model_path"
echo "Embedding GGUF: $embedding_model_path"

cd "$repo_root"
"$container_engine" compose up --build "$@"
