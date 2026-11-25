#!/usr/bin/env bash
# Deletes the e2e kind cluster, its mock OIDC issuer and generated files.
set -euo pipefail
CLUSTER="${1:-kaddie-e2e}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
docker rm -f "$CLUSTER-oidc" >/dev/null 2>&1 || true
kind delete cluster --name "$CLUSTER"
rm -rf "$ROOT/test/infra/.generated/$CLUSTER"
