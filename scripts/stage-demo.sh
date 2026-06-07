#!/usr/bin/env bash
# stage-demo.sh — bring up a kind cluster and three deliberately broken pods
# for the README demo GIF.
#
# Idempotent: re-running deletes and re-creates the cluster. Safe to run
# multiple times.
#
# Usage:
#   ./scripts/stage-demo.sh
#
# Requires: kind, kubectl, docker.

set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-pod-doctor-demo}"

if ! command -v kind >/dev/null; then
  echo "kind not found. Install: https://kind.sigs.k8s.io/" >&2
  exit 1
fi

if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
  echo "==> Removing existing cluster ${CLUSTER_NAME}"
  kind delete cluster --name "${CLUSTER_NAME}"
fi

echo "==> Creating cluster ${CLUSTER_NAME}"
kind create cluster --name "${CLUSTER_NAME}" --wait 60s

KUBECONFIG="$(mktemp)"
export KUBECONFIG
kind get kubeconfig --name "${CLUSTER_NAME}" > "${KUBECONFIG}"

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

echo "==> Waiting ~45s for pods to enter their broken states"
sleep 45

echo "==> Cluster ready. Set:"
echo "  export KUBECONFIG=${KUBECONFIG}"
echo "  pod-doctor default broken-image"
echo "  pod-doctor default broken-oom"
echo "  pod-doctor default broken-crash"
echo "  pod-doctor --all-failing"
