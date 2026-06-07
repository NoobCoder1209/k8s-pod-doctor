# `k8s-pod-doctor` — Execution Plan

## How to use this plan

You are the build session for this repo. Read this file end-to-end, then start executing immediately.

**Working agreement:**

1. **Start without waiting.** Begin Phase 1 in the *Subagent playbook* below.
2. **Always ask the user about business decisions and business logic.** Output formatting style, README copy, the deliberately-broken-pod scenario for the GIF, install instructions tone. The "Business decisions" section below lists them.
3. **Ask the user when you are genuinely blocked.**
4. **Do not ask the user about engineering details.** Internal types, package layout, CLI flag mechanics — your call.
5. **Use subagents aggressively.** Default to the playbook below.
6. **TaskCreate / TaskUpdate everything.**
7. **Pattern 3 only.** No hosted demo. README has a GIF/screenshot of a live `kind` cluster diagnosis.
8. **Follow shared standards** (MIT, README, CI, topics, private until verified).
9. **All `Agent` tool calls must pass `model: "opus"`.**
10. **Off-limits forever:** SAP-internal cluster names / labels / namespaces, `~/.claude/`, RCA content. Tool is generic.

## Subagent playbook (this repo)

Go + client-go + cobra. 3 in research, 2 in review.

**Phase 1 — Research (parallel):**
- `Explore` (Opus): "Find the canonical `client-go` (v0.30+) clientset construction pattern from kubeconfig (env + ~/.kube/config + override flag), plus listing pods, listing events with `fieldSelector`, and tailing logs. Return ≤120-line Go skeleton."
- `Explore` (Opus): "Find common Kubernetes pod failure-mode signatures (CrashLoopBackOff, OOMKilled, ImagePullBackOff, FailedScheduling, PVC unbound, probe-failure events). Return a table of (signature → fields/event reasons → how to detect)."
- `Explore` (Opus): "Find the canonical Cobra CLI structure for a small kubectl-style binary with subcommands and `--kubeconfig` / `--context` overrides. Return root.go skeleton."

**Phase 2 — Design (single):**
- `Plan` (Opus): "Given research and this PLAN.md, propose the package layout, the `Snapshot` and `Finding` types, and the rule list with positive/negative test cases each. Return as a checklist."

**Phase 3 — Build:** main session writes Go code. `Explore` for specific client-go API questions on demand.

**Phase 4 — Review (parallel):**
- `code-reviewer` (Opus): "Review for: read-only enforcement (no exec/delete/patch anywhere), kubeconfig precedence correctness, table-driven test coverage of every rule, JSON output schema consistency. High effort."
- `tester` (Opus): "Verify all unit tests pass under `go test ./...`. Add fixtures for any rule missing positive+negative cases. Confirm a `kind`-cluster smoke run produces sensible output."

**Phase 5 — Polish:** record GIF on a kind cluster with a deliberately-broken pod, ask user before flipping public.

---

## Goal

A small CLI that takes a sick Kubernetes pod and produces a **human-readable
diagnostic report**: pod status, recent events, container log tails, and a
pattern-matcher that names the most common failure modes.

**Sells:** Kubernetes, SRE, CLI development, Go, troubleshooting.

## Business decisions to ask the user about

- **Output style for the verdict line** — boxed banner (recommended) vs single line. Big readability impact.
- **Colour usage** — recommend severity-only colour (red critical, yellow warning), respect `NO_COLOR=1`.
- **Demo scenario for the GIF** — recommend `image: nginx:notreal` for ImagePullBackOff (fastest, most visual). Alternatives: OOMKilled (memory hog), CrashLoopBackOff (failing exec).
- **Install path in README** — recommend both `curl | tar` and `go install` snippets. Confirm with user.
- **Whether to set up a Homebrew tap** — defer to v2 unless user wants it now.

## Scope (must-haves)

1. CLI: `pod-doctor <namespace> <pod-name>` plus `--all-failing` flag.
2. Output: plain text default; `-o json` for machine consumption.
3. Sections: Summary, Status, Recent events, Logs, Diagnosis.
4. Pattern matchers:
   - CrashLoopBackOff
   - OOMKilled
   - ImagePullBackOff / ErrImagePull
   - Pending: PVC unbound
   - Pending: scheduling
   - Probe failures
   - Init container failure
5. kubeconfig discovery (`KUBECONFIG`, then `~/.kube/config`, `--kubeconfig` override).
6. Read-only — never `exec`, `delete`, `patch`.

## Production hygiene (must apply, not optional)

Inherits the master plan's "Production hygiene checklist." Repo-specific application:

- **Global error handling at the CLI boundary.** Wrap Cobra `RunE` so any returned error becomes one short, friendly stderr line + non-zero exit. **Never print a Go stack trace to the user** unless `--debug` (deferred).
- **Friendly errors for the obvious miss-cases:** missing kubeconfig, unreachable cluster, pod not found, no events, container not yet started. Each gets its own one-line message with a hint.
- **No secrets.** `.gitignore` blocks `.env*` and `kubeconfig*` defensively even though the binary doesn't read either.

## Out of scope

- No remediation / auto-fix
- No multi-cluster / multi-context selection beyond current context
- No web UI / TUI
- No metrics-server integration
- No Slack / PagerDuty / Webhook integration
- No daemon mode

## Tech stack

- **Language:** Go 1.22+
- **K8s client:** `k8s.io/client-go` v0.30+
- **CLI:** `github.com/spf13/cobra` + `pflag`
- **Colour:** `github.com/fatih/color`
- **Build:** Go modules + Makefile
- **Linting:** `golangci-lint`
- **Testing:** stdlib `testing` + `client-go/kubernetes/fake`
- **Releases:** GoReleaser (cross-compile darwin/linux × amd64/arm64)

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
  cmd/pod-doctor/main.go
  internal/
    diagnose/
      collect.go
      rules.go
      report.go
      types.go
    diagnose_test/
      rules_test.go
      collect_test.go
    fixtures/
      pod_*.yaml
      events_*.yaml
  .goreleaser.yaml
  .github/workflows/
    ci.yml
    release.yml
  docs/screenshots/demo.gif
```

## Step-by-step build

### 1. Bootstrap

```bash
go mod init github.com/NoobCoder1209/k8s-pod-doctor
go get k8s.io/client-go@v0.30 k8s.io/api@v0.30 k8s.io/apimachinery@v0.30
go get github.com/spf13/cobra@latest github.com/fatih/color@latest
```

### 2. `cmd/pod-doctor/main.go`

Cobra root: `pod-doctor [flags] <namespace> <pod-name>` plus `-o`/`--tail`/`--kubeconfig`/`--context`/`--all-failing`/`-v`. Help text with examples.

### 3. `internal/diagnose/collect.go`

Build clientset from kubeconfig. Methods: `GetPod`, `ListEvents` (`fieldSelector="involvedObject.name=<pod>"`, limit 50), `GetLogs(container, tailLines)`. Wrap into `Snapshot`.

### 4. `internal/diagnose/rules.go`

```go
type Finding struct {
    Code, Severity, Message string
    Suggestions []string
}
type Rule func(s *Snapshot) []Finding
var allRules = []Rule{ crashLoopBackOffRule, oomKilledRule, imagePullBackOffRule, pendingPvcRule, pendingSchedulingRule, probeFailureRule, initContainerFailureRule }
```

Each rule small, returns 0 or 1 Finding.

### 5. `internal/diagnose/report.go`

`RenderText(snapshot, findings, w)` and `RenderJSON(snapshot, findings, w)`. Text uses sectioned output with severity colour.

### 6. Tests

Table-driven with YAML fixtures. Every rule: positive + negative test. `collect_test.go` uses `fake.NewSimpleClientset`.

### 7. CI

`go vet ./...`; `golangci-lint run`; `go test ./...`; `go build ./cmd/pod-doctor`.

### 8. Release

GoReleaser cross-compiles to darwin-amd64/arm64, linux-amd64/arm64. Attach binaries + checksums to GitHub Release on `v*` tag.

### 9. README

1. Title — *k8s-pod-doctor — One command to diagnose a sick Kubernetes pod*
2. Demo — `docs/screenshots/demo.gif`
3. What it shows — read-only diagnosis, pattern matchers, JSON output for piping
4. Skills demonstrated — Kubernetes, SRE, Go, CLI development, client-go, GoReleaser
5. Install — `curl | install` + `go install` snippets
6. Quick start examples
7. Patterns supported — table
8. License — MIT

### 10. Polish + flip public

GIF on kind. Topics: `kubernetes`, `sre`, `cli`, `go`, `troubleshooting`, `client-go`. Ask user before flipping.

## Verification

- [ ] `go build ./cmd/pod-doctor` works on Go 1.22
- [ ] Real broken pod in kind cluster produces sensible report
- [ ] Each rule has positive + negative test
- [ ] `-o json` validates against documented schema in `docs/output.schema.json`
- [ ] No SAP-specific assumptions (namespaces, labels, registries)
- [ ] No `~/.claude/` references
- [ ] Missing kubeconfig / unreachable cluster / pod-not-found each produces a one-line friendly error, not a Go stack trace
- [ ] `.gitignore` blocks `kubeconfig*` and `.env*` defensively
- [ ] Release pipeline produces working binaries (darwin, linux × amd64 minimum)
- [ ] Topics + description set

## Stretch (defer)

- `--watch` mode
- `metrics-server` integration ("memory at 92% of limit")
- Slack-friendly markdown output (`-o slack`)
- `--remediate` (prints next-step kubectl commands)
