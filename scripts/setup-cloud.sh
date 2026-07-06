#!/usr/bin/env bash
set -euo pipefail

container_engine="${1:-podman}"
if [[ "$container_engine" == "podman" || "$container_engine" == "docker" ]]; then
  shift || true
else
  container_engine="podman"
fi
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "Starting cloud provider stack with $container_engine compose."
echo "Using provider settings from compose.remote.yaml. Edit compose.remote.yaml before first boot to set API keys or model choices."

cd "$repo_root"
"$container_engine" compose -f compose.yaml -f compose.remote.yaml up --build "$@"
