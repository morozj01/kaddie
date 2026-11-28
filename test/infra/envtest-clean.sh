#!/usr/bin/env bash
# Stops etcd and kube-apiserver processes that envtest started from ./bin/k8s
# and never stopped: a run ended by Ctrl+C, a test timeout or a panic skips
# env.Stop(). On Windows they outlive go test and hold test/envtest open as
# their working directory, so switching to a branch without it fails.
set -euo pipefail
BIN="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/bin/k8s"

case "${OSTYPE:-}" in
  msys* | cygwin*)
    ENVTEST_BIN="$(cygpath -w "$BIN")\\" powershell.exe -NoProfile -NonInteractive -Command '
      $p = @(Get-Process etcd, kube-apiserver -ErrorAction SilentlyContinue |
        Where-Object { $_.Path -and $_.Path.StartsWith($env:ENVTEST_BIN, [StringComparison]::OrdinalIgnoreCase) })
      foreach ($x in $p) { "envtest-clean: stopping $($x.Name) $($x.Id)" }
      $p | Stop-Process -Force
      $p | Wait-Process -Timeout 10 -ErrorAction SilentlyContinue'
    ;;
  *)
    pids="$(pgrep -f "^$BIN/" || true)"
    if [ -n "$pids" ]; then
      echo "envtest-clean: stopping" $pids
      kill $pids
    fi
    ;;
esac
