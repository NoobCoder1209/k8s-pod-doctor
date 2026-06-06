# `k8s-pod-doctor` — Execution Plan

> Tier 2 shelf — built opportunistically, not pinned. Inherits shared standards
> from the master plan.

## Goal

A small CLI that takes a sick Kubernetes pod and produces a **human-readable
diagnostic report**: pod status, recent events, container log tails, and a
pattern-matcher that names the most common failure modes
(CrashLoopBackOff, OOMKilled, ImagePullBackOff, readiness/liveness probe
failures, PVC stuck pending, container exited non-zero).

This is the kind of tool an SRE writes once, copies forever. Public version =
proof Aleksandar's SRE claim is real.

**Sells:** Kubernetes, SRE, CLI development, Go, troubleshooting.

## Scope (must-haves)

1. CLI invocation: `pod-doctor <namespace> <pod-name>` (and a `--all-failing`
   flag that scans the namespace).
2. Output is **plain text** by default, **`-o json`** flag for machine consumption.
3. Sections in the report (in order):
   - **Summary** — one-sentence verdict (healthy / sick + matched pattern)
   - **Status** — phase, conditions, containers (image, restarts, last termination reason)
   - **Recent events** — last 10 events for that pod, oldest first
   - **Logs** — last 20 lines per container (or `--tail N` override)
   - **Diagnosis** — list of matched patterns with suggested next steps
4. Pattern matchers (rules.go):
   - `CrashLoopBackOff` — `restartCount > 0` AND last termination reason in `{Error, OOMKilled}`
   - `OOMKilled` — last termination reason exactly `OOMKilled`
   - `ImagePullBackOff` / `ErrImagePull` — container waiting reason
   - `Pending: PVC` — pod phase Pending AND event mentioning unbound PVC
   - `Pending: scheduling` — pod phase Pending AND event mentioning `FailedScheduling`
   - `Probe failures` — events with reason `Unhealthy` mentioning probe
   - `Init container failure` — initContainer status non-zero
5. Uses **kubeconfig** discovery (`KUBECONFIG` env, then `~/.kube/config`).
6. Read-only: never `kubectl exec`, never `delete`, never patch.

## Out of scope

- No remediation / auto-fix actions.
- No multi-cluster / multi-context selection beyond the kubeconfig's current context.
- No web UI / TUI.
- No metrics-server integration (keep it simple — just events, status, logs).
- No Slack / PagerDuty / Webhook integration.
- No auto-discovery loops, no daemonset deployment.

## Tech stack

- **Language:** Go 1.22+
- **Kubernetes client:** `k8s.io/client-go` v0.30+
- **CLI framework:** `github.com/spf13/cobra` + `pflag`
- **Colors:** `github.com/fatih/color` (auto-disabled when not a TTY or `NO_COLOR=1`)
- **Output:** stdout for humans, JSON for `-o json` (encoding/json stdlib)
- **Build:** Go modules + `Makefile`
- **Linting:** `golangci-lint`
- **Testing:** standard `testing` + table-driven; mock the K8s API via
  `client-go/kubernetes/fake`
- **Releases:** GoReleaser (cross-compile to darwin/linux on amd64/arm64)

## File tree

```
k8s-pod-doctor/
  README.md
  PLAN.md
  LICENSE
  .gitignore
  go.mod
  go.sum
  Makefile
  cmd/
    pod-doctor/
      main.go
  internal/
    diagnose/
      collect.go         ← talks to the API: pod, events, logs
      rules.go           ← pattern matchers
      report.go          ← formatters (text + json)
      types.go           ← Diagnosis, Finding, Severity
    diagnose_test/
      rules_test.go      ← table tests using fixtures
      collect_test.go    ← uses fake.Clientset
    fixtures/
      pod_crashloop.yaml
      pod_oomkilled.yaml
      pod_imagepullbackoff.yaml
      pod_pending_pvc.yaml
      events_*.yaml
  .goreleaser.yaml
  .github/
    workflows/
      ci.yml             ← lint + test + build
      release.yml        ← runs on tag, publishes binaries via GoReleaser
  docs/
    screenshots/
      demo.gif           ← terminal cast of pod-doctor diagnosing a broken pod
```

## Step-by-step build

### 1. Bootstrap

```bash
go mod init github.com/NoobCoder1209/k8s-pod-doctor
go get k8s.io/client-go@v0.30 k8s.io/api@v0.30 k8s.io/apimachinery@v0.30
go get github.com/spf13/cobra@latest github.com/fatih/color@latest
```

### 2. `cmd/pod-doctor/main.go`

Cobra root command:
```
pod-doctor [flags] <namespace> <pod-name>
  -o, --output       string   "text"|"json" (default text)
  --tail             int      log lines per container (default 20)
  --kubeconfig       string   override kubeconfig path
  --context          string   override kubeconfig context
  --all-failing               diagnose all non-Running pods in namespace
  -v, --verbose               include full container env (default false)
```

`pod-doctor --help` shows examples (a healthy pod, a CrashLoopBackOff pod).

### 3. `internal/diagnose/collect.go`

Builds a Kubernetes clientset from kubeconfig, then:
- `GetPod(namespace, name)`
- `ListEvents(namespace, fieldSelector="involvedObject.name=<pod>", limit=50)`
- `GetLogs(namespace, pod, container, tailLines)`

Wraps these into a `Snapshot` struct passed downstream.

### 4. `internal/diagnose/rules.go`

```go
type Finding struct {
    Code        string   // "CrashLoopBackOff"
    Severity    string   // "critical"|"warning"|"info"
    Message     string
    Suggestions []string
}

type Rule func(s *Snapshot) []Finding

var allRules = []Rule{
    crashLoopBackOffRule,
    oomKilledRule,
    imagePullBackOffRule,
    pendingPvcRule,
    pendingSchedulingRule,
    probeFailureRule,
    initContainerFailureRule,
}
```

Each rule is small and returns 0 or 1 Finding. Apply all rules → list of findings.

### 5. `internal/diagnose/report.go`

`RenderText(snapshot, findings, w io.Writer)` and
`RenderJSON(snapshot, findings, w io.Writer)`.

Text uses sectioned output with optional colour for severity:

```
╭─ pod/myapp-7c... in default ─────────────────────────────╮
│ Verdict: CrashLoopBackOff (critical)                      │
╰───────────────────────────────────────────────────────────╯

Status
  Phase: Running
  Containers:
    - api: image=myrepo/api:v1.2.3 restarts=12 lastTerminated=OOMKilled

Recent events (last 10)
  2m   Warning  BackOff       Back-off restarting failed container
  2m   Normal   Pulled        Successfully pulled image "myrepo/api:v1.2.3"
  ...

Logs (api, last 20)
  2026-06-05T14:01 fatal error: runtime: out of memory
  ...

Diagnosis
  • CrashLoopBackOff (critical)
    The container has restarted 12 times. Last exit was OOMKilled.
    Suggestions:
      - Bump memory limits or fix the leak
      - Check `kubectl top pod` for trend
      - Inspect logs above for stack traces
```

JSON output is the same data shaped for `jq`.

### 6. Tests

Table-driven rules tests using YAML fixtures (`fixtures/pod_*.yaml`) parsed
into pod/events fakes. Goal: every rule has at least one positive and one
negative test.

`collect_test.go` uses `kubernetes/fake.NewSimpleClientset(pod, event,...)`
to verify Snapshot construction.

### 7. CI

`.github/workflows/ci.yml`:
- Setup Go 1.22
- `go vet ./...`
- `golangci-lint run`
- `go test ./...`
- `go build ./cmd/pod-doctor` (sanity)

### 8. Release

`.github/workflows/release.yml` runs on `v*` tags and uses GoReleaser to:
- Cross-compile to darwin-amd64, darwin-arm64, linux-amd64, linux-arm64
- Attach binaries + checksums to the GitHub Release
- (Optional) push a Homebrew tap formula — defer; v1 ships GitHub Releases only

`.goreleaser.yaml`: minimal config naming the binary `pod-doctor`.

### 9. README

1. **Title** — *k8s-pod-doctor — One command to diagnose a sick Kubernetes pod*
2. **Demo** — `docs/screenshots/demo.gif`
3. **What it shows** — read-only diagnosis, pattern matchers, JSON output for piping
4. **Skills demonstrated** — Kubernetes, SRE, Go, CLI development, client-go, GoReleaser
5. **Install**:
   ```bash
   curl -sSL https://github.com/NoobCoder1209/k8s-pod-doctor/releases/latest/download/pod-doctor-linux-amd64 -o /usr/local/bin/pod-doctor
   chmod +x /usr/local/bin/pod-doctor
   # or: go install github.com/NoobCoder1209/k8s-pod-doctor/cmd/pod-doctor@latest
   ```
6. **Quick start**:
   ```bash
   pod-doctor default broken-pod-7c
   pod-doctor --all-failing default
   pod-doctor default broken-pod-7c -o json | jq .findings
   ```
7. **Patterns supported** — table of matchers + suggestions
8. **License** — MIT

### 10. Polish + flip public

Record GIF on a kind cluster with a deliberately broken pod (e.g.
`image: nginx:notreal` for ImagePullBackOff, OR a memory-hungry container
for OOMKilled). Topics: `kubernetes`, `sre`, `cli`, `go`, `troubleshooting`,
`client-go`. Flip public.

## Verification

- [ ] `go build ./cmd/pod-doctor` works on Go 1.22
- [ ] `pod-doctor default broken-pod` produces a sensible report against a
      real broken pod in a kind cluster
- [ ] Each rule has at least one passing positive and one negative test
- [ ] `-o json` validates against a documented schema (just publish the schema
      in `docs/output.schema.json`)
- [ ] No SAP-specific assumptions (namespaces, label keys, image registries)
- [ ] No `~/.claude/` references, nothing imported from Aleksandar's tools
- [ ] Release pipeline produces working binaries for darwin and linux on at least amd64
- [ ] Topics + description set

## Stretch (defer)

- `--watch` mode (re-runs on tick)
- `metrics-server` integration ("memory at 92% of limit, crashed soon after")
- Slack-friendly markdown output (`-o slack`)
- A `--remediate` flag that prints the exact `kubectl` commands to try next

v2 — out of v1 scope.
