# k8s-pod-doctor

> **One command. Sick pod. Real answers.**

A read-only Kubernetes CLI that takes a struggling pod and tells you, in
plain English, what's actually wrong with it — pulled from the pod's status,
its recent events, and its container log tails — and surfaces a verdict
naming the most common failure mode.

[![CI](https://github.com/NoobCoder1209/k8s-pod-doctor/actions/workflows/ci.yml/badge.svg)](https://github.com/NoobCoder1209/k8s-pod-doctor/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/NoobCoder1209/k8s-pod-doctor.svg)](https://pkg.go.dev/github.com/NoobCoder1209/k8s-pod-doctor)
[![Go Report Card](https://goreportcard.com/badge/github.com/NoobCoder1209/k8s-pod-doctor)](https://goreportcard.com/report/github.com/NoobCoder1209/k8s-pod-doctor)
[![codecov](https://codecov.io/gh/NoobCoder1209/k8s-pod-doctor/branch/main/graph/badge.svg)](https://codecov.io/gh/NoobCoder1209/k8s-pod-doctor)
[![Go 1.23+](https://img.shields.io/badge/go-1.23%2B-00ADD8?logo=go)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

## Demo

![demo](docs/screenshots/demo.gif)

The GIF runs `pod-doctor` against three deliberately-broken pods on a local
[`kind`](https://kind.sigs.k8s.io/) cluster: an `ImagePullBackOff`, an
`OOMKilled` container, and a `CrashLoopBackOff` triggered by a failing
command.

## Why?

When you `kubectl describe pod` and `kubectl logs` and `kubectl get events`
to debug a sick pod, you do the same correlation in your head every single
time. `pod-doctor` does it for you and spits out a verdict.

What sets it apart from a `describe + logs + events` mash-up is **priority-
aware verdict suppression**: when a container is OOMKilled and consequently
in `CrashLoopBackOff`, the OOM is the cause and the loop is the symptom.
`pod-doctor` surfaces OOMKilled as the verdict and suppresses the redundant
CrashLoop finding for that container — see [verdict picking](#how-verdict-picking-works).

```text
╔════════════════════════════════════════════════════════════╗
║  ✖  CRITICAL: Container web cannot pull image              ║
║  Image "nginx:notreal" cannot be pulled: ImagePullBackOff  ║
╚════════════════════════════════════════════════════════════╝
```

<details>
<summary>Full output (click to expand)</summary>

```text
╔════════════════════════════════════════════════════════════╗
║  ✖  CRITICAL: Container web cannot pull image              ║
║  Image "nginx:notreal" cannot be pulled: ImagePullBackOff  ║
╚════════════════════════════════════════════════════════════╝

== Status ==
  Pod:         default/web-7d9f-abc
  Phase:       Pending
  Node:        kind-worker
  Started:     2026-06-07T09:00:00Z
  Conditions:
    Ready              False
    ContainersReady    False
    Initialized        True
    PodScheduled       True
  Containers:
    web                  ready=false restarts=0 state=Waiting:ImagePullBackOff

== Recent events ==
  09:31:42  Warning   Failed               Failed to pull image "nginx:notreal": rpc error: not found
  09:31:42  Warning   Failed               Error: ErrImagePull
  09:32:00  Normal    BackOff              Back-off pulling image "nginx:notreal"
  09:32:15  Warning   Failed               Error: ImagePullBackOff

== Logs ==
  -- web -- (no logs: container not yet started)

== Findings ==
  1. [CRITICAL] Container web cannot pull image
     Image "nginx:notreal" cannot be pulled: ImagePullBackOff — Back-off pulling image "nginx:notreal"
     container: web
     suggestions:
       - Verify image exists and tag is correct: docker pull nginx:notreal
       - Check imagePullSecrets and registry credentials in namespace default
```

</details>

## Skills demonstrated

This is a portfolio repo. Each skill below points at the file/region where
it's exercised so reviewers can verify rather than take my word for it.

| Skill | Where it shows up |
|---|---|
| **Idiomatic Go CLI** with Cobra | [`cmd/pod-doctor/root.go`](cmd/pod-doctor/root.go) — `RunE`, `PreRunE` validation, `SilenceUsage`/`SilenceErrors` paired with one-line stderr error mapping in `main.go`. Renderer is dependency-injected through `Run`'s signature, no globals. |
| **Kubernetes API access via `client-go`** | [`internal/doctor/kube.go`](internal/doctor/kube.go) — canonical `clientcmd.NewNonInteractiveDeferredLoadingClientConfig` with `--kubeconfig` > `$KUBECONFIG` > `~/.kube/config` precedence, plus `--context` override. Returns `kubernetes.Interface` for testability. |
| **Read-only contract** | [`internal/doctor/collect.go`](internal/doctor/collect.go) — only `.Get()`, `.List()`, and `.GetLogs().Stream()` are ever called. No `Create`/`Update`/`Patch`/`Delete`/`Exec`/`PortForward` anywhere in the codebase. |
| **Pattern-matching SRE rules** | [`internal/doctor/`](internal/doctor/) — seven failure-mode detectors with priority-based verdict picking and per-container suppression so OOMKilled wins over CrashLoopBackOff for the same container. |
| **Native sidecar awareness** | [`internal/doctor/rules.go`](internal/doctor/rules.go) — pods using `restartPolicy: Always` init containers (Kubernetes 1.29+ native sidecars, e.g. Istio) are correctly classified by the runtime rules instead of being misreported as init failures. |
| **Stable JSON output schema** | [`internal/render/json.go`](internal/render/json.go) — versioned `schemaVersion: "1"`, additive-only fields, `findings: []` (not `null`) for healthy pods. Designed for `\| jq` pipelines. |
| **Friendly CLI UX** | [`internal/doctor/doctor.go`](internal/doctor/doctor.go) — one-line stderr messages for missing kubeconfig, unreadable kubeconfig, missing context, pod-not-found, container-not-yet-started, invalid `--output`, negative `--tail`, and the combined `--all-failing` + positional-args case. No Go stack traces ever leak to the user. |
| **Test discipline** | Every rule has positive AND negative test cases. Integration tests through `Diagnose()` lock the OOM-suppresses-CrashLoop and ProbeFailure-suppresses-CrashLoop verdict resolution. Render layer has section-order tests. `fake.NewSimpleClientset` covers the collect path. |
| **Cross-platform release pipeline** | [`.goreleaser.yaml`](.goreleaser.yaml) — darwin/linux × amd64/arm64, GoReleaser v2 schema, ldflags-injected version constants, Homebrew tap stub ready to enable. |

## Install

### Pre-built binary (`curl | tar`)

```bash
# macOS (Apple Silicon)
curl -fsSL https://github.com/NoobCoder1209/k8s-pod-doctor/releases/latest/download/pod-doctor_Darwin_arm64.tar.gz \
  | tar -xz -C /usr/local/bin pod-doctor

# Linux (x86_64)
curl -fsSL https://github.com/NoobCoder1209/k8s-pod-doctor/releases/latest/download/pod-doctor_Linux_x86_64.tar.gz \
  | tar -xz -C /usr/local/bin pod-doctor
```

Other platforms are listed on the [releases page](https://github.com/NoobCoder1209/k8s-pod-doctor/releases).

### From source (`go install`)

```bash
go install github.com/NoobCoder1209/k8s-pod-doctor/cmd/pod-doctor@latest
```

### Homebrew (tap)

```bash
brew install NoobCoder1209/tap/pod-doctor
```

## Quick start

```bash
# Diagnose one pod
pod-doctor default web-7d9f-abc

# Use a specific kubeconfig and context
pod-doctor --kubeconfig=$HOME/.kube/dev --context staging \
  kube-system coredns-7d4f-xyz

# JSON output for pipelines
pod-doctor default web-7d9f-abc -o json | jq '.verdict.code'

# Diagnose every non-Running pod in the cluster
pod-doctor --all-failing

# Pipe across many pods, surface only the verdicts
pod-doctor --all-failing -o json | jq -r '.[] | "\(.pod.namespace)/\(.pod.name): \(.verdict.code // "healthy")"'
```

## Patterns supported

| Code | What it catches | Severity |
|---|---|---|
| `PendingScheduling` | Scheduler can't place the pod (taints, resources, affinity, topology) | critical |
| `PendingVolume` | PVC not bound, volume can't attach, missing Secret/ConfigMap mount | critical |
| `InitContainerFailure` | Non-sidecar init container blocking pod startup; classifies the underlying cause (image-pull, OOM, crash) | critical |
| `ImagePullBackOff` | `ErrImagePull`, `ImagePullBackOff`, `ImageInspectError`, `InvalidImageName`, `RegistryUnavailable` | critical |
| `OOMKilled` | Container hit its memory limit (state OR last termination, exit 137) | critical |
| `ProbeFailure` | Liveness/readiness/startup probes failing (`Unhealthy` events). Liveness = critical, readiness/startup = warning. | critical / warning |
| `CrashLoopBackOff` | Container repeatedly exiting; surfaced only when OOM/probe doesn't already explain it for the same container | critical |

## How verdict picking works

1. Each rule emits zero or more `Finding`s with a `Priority` (1 = highest).
2. Findings are sorted by `(Priority asc, Severity desc, Container asc)`.
3. For each container, only the highest-priority finding is kept — so an
   `OOMKilled` finding suppresses the `CrashLoopBackOff` finding on the same
   container, because the OOM is the cause and the loop is the symptom.
4. Pod-level findings (no container) are always kept.
5. The first finding after sort + suppression is the verdict.

## JSON output schema

```json
{
  "schemaVersion": "1",
  "tool": "k8s-pod-doctor",
  "toolVersion": "0.1.0",
  "generatedAt": "2026-06-07T10:15:30Z",
  "pod": { "namespace": "default", "name": "web", "uid": "...", "phase": "Pending" },
  "summary": {
    "phase": "Pending",
    "nodeName": "kind-worker",
    "conditions": { "PodScheduled": "True", "Ready": "False" },
    "containerStatuses": [
      { "name": "web", "ready": false, "restartCount": 0, "state": "waiting", "reason": "ImagePullBackOff" }
    ],
    "eventCount": 4
  },
  "findings": [
    {
      "code": "ImagePullBackOff",
      "severity": "critical",
      "title": "Container web cannot pull image",
      "message": "Image \"nginx:notreal\" cannot be pulled: ImagePullBackOff — Back-off pulling image",
      "container": "web",
      "suggestions": ["Verify image exists and tag is correct: docker pull nginx:notreal", "..."],
      "evidence": ["state.waiting.reason=ImagePullBackOff", "..."]
    }
  ],
  "verdict": {
    "code": "ImagePullBackOff",
    "severity": "critical",
    "title": "Container web cannot pull image",
    "message": "Image \"nginx:notreal\" cannot be pulled: ImagePullBackOff — Back-off pulling image"
  },
  "healthy": false
}
```

`schemaVersion` will only bump on a breaking change. New fields are added
without a version bump. Healthy pods omit the `verdict` key entirely. See
[`docs/output.schema.json`](docs/output.schema.json) for the full schema;
the example above elides several optional summary fields (`startTime`,
`initContainerStatuses`, `logErrors`).

## Flags

| Flag | Default | Description |
|---|---|---|
| `--kubeconfig` | `$KUBECONFIG` or `~/.kube/config` | Path to kubeconfig (overrides `$KUBECONFIG`) |
| `--context` | (current) | Kubeconfig context to use |
| `-o, --output` | `text` | `text` or `json` |
| `--tail` | `100` | Lines of recent logs to include per container |
| `--all-failing` | `false` | Diagnose every non-Running pod in the cluster |
| `--no-color` | `false` | Disable ANSI colour (also via `NO_COLOR` env) |

## Build from source

```bash
git clone https://github.com/NoobCoder1209/k8s-pod-doctor
cd k8s-pod-doctor
make build       # produces ./pod-doctor
make test        # race-detector unit tests
make lint        # golangci-lint (requires the binary on $PATH)
```

Requires Go 1.23+.

## What it does NOT do

- It does **not** modify the cluster. Ever. No `exec`, no `delete`, no
  `patch`, no `apply` — verifiable in [`internal/doctor/collect.go`](internal/doctor/collect.go).
- It does not auto-remediate. It tells you what's wrong; you decide what to do.
- It does not require `metrics-server`, `kubectl`, or any other dependency
  beyond a reachable Kubernetes API server.
- It does not follow logs (no `--watch` mode). It's a one-shot diagnosis.

## License

[MIT](LICENSE)
