#!/usr/bin/env bash
# stage-demo.sh — bring up a kind cluster and three deliberately broken pods
# for the README demo GIF.
#
# Idempotent: re-running deletes and re-creates the cluster.
#
# Usage:
#   ./scripts/stage-demo.sh
#
# Then:
#   make build
#   vhs scripts/demo.tape   # uses KUBECONFIG_PATH that this script wrote
#
# Requires: kind, kubectl, docker.

set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-pod-doctor-demo}"
# Keep this path in sync with scripts/demo.tape.
KUBECONFIG_PATH="${KUBECONFIG_PATH:-/tmp/pod-doctor-demo.kubeconfig}"

for cmd in kind kubectl docker; do
  if ! command -v "$cmd" >/dev/null; then
    echo "$cmd not found on \$PATH" >&2
    exit 1
  fi
done
if ! docker info >/dev/null 2>&1; then
  echo "Docker daemon is not running (try Docker Desktop / colima)" >&2
  exit 1
fi

if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
  echo "==> Removing existing cluster ${CLUSTER_NAME}"
  kind delete cluster --name "${CLUSTER_NAME}"
fi

echo "==> Creating cluster ${CLUSTER_NAME}"
kind create cluster --name "${CLUSTER_NAME}" --wait 60s

kind get kubeconfig --name "${CLUSTER_NAME}" > "${KUBECONFIG_PATH}"
export KUBECONFIG="${KUBECONFIG_PATH}"

echo "==> Applying broken pods"

# 1) ImagePullBackOff
kubectl apply -f - <<'YAML'
apiVersion: v1
kind: Pod
metadata:
  name: broken-image
  labels: { demo: "true" }
spec:
  containers:
  - name: web
    image: nginx:notreal-image-tag-12345
YAML

# 2) OOMKilled — allocate ~200Mi, limit at 50Mi.
kubectl apply -f - <<'YAML'
apiVersion: v1
kind: Pod
metadata:
  name: broken-oom
  labels: { demo: "true" }
spec:
  containers:
  - name: hog
    image: polinux/stress
    command: ["stress"]
    args: ["--vm", "1", "--vm-bytes", "200M", "--vm-hang", "0"]
    resources:
      limits: { memory: "50Mi" }
YAML

# 3) CrashLoopBackOff — exit 1 immediately.
kubectl apply -f - <<'YAML'
apiVersion: v1
kind: Pod
metadata:
  name: broken-crash
  labels: { demo: "true" }
spec:
  containers:
  - name: app
    image: busybox
    command: ["sh", "-c", "echo 'starting up...'; sleep 1; echo 'panic: something went wrong'; exit 1"]
YAML

echo "==> Waiting for pods to enter their broken states"

# Each pod has its own deterministic broken state. Polling beats sleep.
kubectl wait --for=jsonpath='{.status.containerStatuses[0].state.waiting.reason}'=ImagePullBackOff \
  pod/broken-image --timeout=180s || true

# OOM and crash both cycle through CrashLoopBackOff once they've been killed
# at least twice. Wait until restartCount >= 2 to be sure.
for pod in broken-oom broken-crash; do
  for _ in $(seq 1 60); do
    rc="$(kubectl get pod "$pod" -o jsonpath='{.status.containerStatuses[0].restartCount}' 2>/dev/null || echo 0)"
    if [[ "$rc" -ge 2 ]]; then break; fi
    sleep 2
  done
done

echo
echo "==> Cluster ready."
echo "    KUBECONFIG written to: ${KUBECONFIG_PATH}"
echo "    Run:  vhs scripts/demo.tape"
