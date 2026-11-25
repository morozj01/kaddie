#!/usr/bin/env bash
# Creates the e2e kind cluster with audit logging and a mock OIDC issuer the
# kube-apiserver trusts (implementation plan T0.2, T0.3).
#
#   kind-up.sh <1.28|latest> [cluster-name]
#
# Writes test/infra/.generated/<cluster>/oidc.json, which the e2e tests read
# to mint tokens. Works under Git Bash on Windows and bash on Linux.
set -euo pipefail

K8S="${1:-1.28}"
CLUSTER="${2:-kaddie-e2e}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
INFRA="$ROOT/test/infra"
GEN="$INFRA/.generated/$CLUSTER"
ISSUER_CONTAINER="$CLUSTER-oidc"
ISSUER_IMAGE="kaddie-oidc-issuer:dev"

case "$K8S" in
  1.28) TEMPLATE="$INFRA/kind-1.28.yaml" ;;
  latest) TEMPLATE="$INFRA/kind-latest.yaml" ;;
  *) echo "kind-up: K8S must be 1.28 or latest, got '$K8S'" >&2; exit 2 ;;
esac

# Docker Desktop on Windows wants C:/... paths in bind mounts.
hostpath() { if command -v cygpath >/dev/null 2>&1; then cygpath -m "$1"; else echo "$1"; fi; }

if kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then
  echo "kind-up: cluster $CLUSTER already exists; run kind-down first to recreate it" >&2
  exit 0
fi

rm -rf "$GEN" && mkdir -p "$GEN/image"
echo "kind-up: generating issuer certificates"
go run "$ROOT/test/mocks/oidc/cmd/certgen" -out "$GEN"
cp "$INFRA/audit-policy.yaml" "$GEN/audit-policy.yaml"
CA_PEM="$(sed 's/^/        /' "$GEN/ca.crt")"
export CA_PEM
awk -v ca="$CA_PEM" '{ if ($0 == "${CA_PEM}") print ca; else print }' "$INFRA/auth-config.yaml" > "$GEN/auth-config.yaml"
GEN_DIR="$(hostpath "$GEN")"
sed "s#\${GEN_DIR}#$GEN_DIR#g" "$TEMPLATE" > "$GEN/kind.yaml"

echo "kind-up: building the mock issuer image"
case "$(docker info --format '{{.Architecture}}')" in
  aarch64|arm64) GOARCH=arm64 ;;
  *) GOARCH=amd64 ;;
esac
CGO_ENABLED=0 GOOS=linux GOARCH=$GOARCH go build -trimpath -o "$GEN/image/oidc-issuer" "$ROOT/test/mocks/oidc/cmd/oidc-issuer"
printf 'FROM scratch\nCOPY oidc-issuer /oidc-issuer\nENTRYPOINT ["/oidc-issuer"]\n' > "$GEN/image/Dockerfile"
docker build -q -t "$ISSUER_IMAGE" "$(hostpath "$GEN/image")" >/dev/null

echo "kind-up: creating cluster $CLUSTER ($K8S)"
kind create cluster --name "$CLUSTER" --config "$(hostpath "$GEN/kind.yaml")" --wait 120s

echo "kind-up: starting the mock issuer on the kind network"
docker rm -f "$ISSUER_CONTAINER" >/dev/null 2>&1 || true
MSYS_NO_PATHCONV=1 docker run -d --name "$ISSUER_CONTAINER" --network kind --network-alias kaddie-oidc \
  -p 127.0.0.1::8443 -v "$GEN_DIR:/certs:ro" "$ISSUER_IMAGE" >/dev/null
PORT="$(docker port "$ISSUER_CONTAINER" 8443/tcp | head -1 | sed 's/.*://')"
HOST_URL="https://127.0.0.1:$PORT"
for _ in $(seq 1 30); do
  if curl -sf --cacert "$GEN/ca.crt" "$HOST_URL/healthz" >/dev/null; then break; fi
  sleep 1
done
curl -sf --cacert "$GEN/ca.crt" "$HOST_URL/healthz" >/dev/null || { echo "kind-up: issuer not reachable at $HOST_URL" >&2; exit 1; }

cat > "$GEN/oidc.json" <<JSON
{
  "issuer": "https://kaddie-oidc:8443",
  "hostURL": "$HOST_URL",
  "caFile": "$(hostpath "$GEN/ca.crt")",
  "clientID": "kaddie-ui",
  "usernamePrefix": "oidc:",
  "groupsPrefix": "oidc:"
}
JSON
echo "kind-up: ready. kubectl context kind-$CLUSTER; issuer at $HOST_URL"
