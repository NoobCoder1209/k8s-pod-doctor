# Guide

> **Last verified:** 2026-06-09 against commit `9dbd19e` (branch `feature/guide-and-demo-verification`). Ran the full demo flow on macOS Apple Silicon. The JSON pipeline in section 1 step 6 returned exactly:
>
> ```
> default/broken-crash: CrashLoopBackOff
> default/broken-image: ImagePullBackOff
> default/broken-oom: OOMKilled
> ```
>
> GIF re-recorded against the post-fix binary (530K, replaces the prior 692K recording from before the crash-loop fix). A frame extracted at t≈18s shows the CRITICAL OOMKilled banner with full Status / Events / Logs / Findings sections.

This guide is for someone who has never touched the repo. It explains every command in order, what every meaningful file does, what env vars and secrets are needed, and what a working demo looks like.

If you only want to install and run the binary, the [README](README.md) covers that. This guide is for *running the demo locally* and *understanding the codebase*.

---

## 1. Run the demo end-to-end

### Prerequisites

Required to run the demo:

| Tool | Why | How to install (macOS) |
|---|---|---|
| Go 1.23+ | Build the binary | `brew install go` |
| Docker (running) | `kind` provisions K8s nodes as Docker containers | [Docker Desktop](https://docs.docker.com/desktop/) — make sure the daemon is running before you start |
| `kind` | Spins up a local Kubernetes cluster | `brew install kind` |
| `kubectl` | Talks to the cluster | `brew install kubectl` |

Optional (only if you want to re-record the GIF or use the JSON pipeline example):

| Tool | Why |
|---|---|
| `vhs` | Records the demo GIF declaratively (`brew install vhs`) |
| `jq` | Parses JSON in the `--all-failing -o json` example (`brew install jq`) |

You don't need a cloud account, a real cluster, or any credentials. Everything runs locally.

### The demo, in order

```bash
# 1. Clone the repo and enter it
git clone https://github.com/NoobCoder1209/k8s-pod-doctor
cd k8s-pod-doctor

# 2. Build the binary into ./pod-doctor
make build

# 3. Bring up a kind cluster + 3 deliberately broken pods.
#    Idempotent: re-running tears the cluster down and recreates it.
#    Writes the kubeconfig to /tmp/pod-doctor-demo.kubeconfig.
./scripts/stage-demo.sh

# 4. Point your shell at the cluster.
export KUBECONFIG=/tmp/pod-doctor-demo.kubeconfig

# 5. Run pod-doctor against each broken pod.
./pod-doctor default broken-image     # expect: ImagePullBackOff verdict
./pod-doctor default broken-oom       # expect: OOMKilled verdict
./pod-doctor default broken-crash     # expect: CrashLoopBackOff verdict

# 6. Diagnose every failing pod at once, in JSON.
./pod-doctor --all-failing -o json | jq -r '.[] | "\(.pod.namespace)/\(.pod.name): \(.verdict.code // "healthy")"'
```

Step 5 is also what the demo GIF captures. If you want to re-record the GIF after making changes:

```bash
vhs scripts/demo.tape
# writes docs/screenshots/demo.gif
```

### When you're done

```bash
kind delete cluster --name pod-doctor-demo
rm -f /tmp/pod-doctor-demo.kubeconfig
```

---

## 2. What every meaningful directory and file does

```
k8s-pod-doctor/
├── README.md                    # Public-facing landing page (badges, demo GIF, install, quick-start)
├── guide.md                     # THIS FILE — for contributors / someone debugging the repo itself
├── PLAN.md                      # The original execution plan (kept for transparency about how this got built)
├── LICENSE                      # MIT
├── Makefile                     # build / test / lint / fmt / tidy / clean / run / help targets
├── go.mod / go.sum              # Go module file. Pinned to k8s.io/* v0.32.1, go 1.23+
├── .gitignore                   # Defensive — blocks kubeconfig*, .env*, *.key, *.pem etc., even though pod-doctor reads no secrets
├── .golangci.yml                # Lint config: errcheck, govet, ineffassign, staticcheck, unused, goimports, misspell, unconvert
├── .goreleaser.yaml             # Cross-compile darwin/linux × amd64/arm64 + Homebrew tap formula on `v*` tags
│
├── cmd/pod-doctor/
│   ├── main.go                  # Tiny entrypoint: signal-context + ExecuteContext + one-line stderr error mapping
│   └── root.go                  # All Cobra wiring — flags, arg validation, `version` subcommand. NO business logic
│
├── internal/doctor/             # The library. Tests can import this directly without going through cmd/.
│   ├── options.go               # Options struct — the single boundary type between the CLI and the library
│   ├── doctor.go                # Run(ctx, opts, renderer, out) — the library entry point. Wires kube + collect + diagnose + render
│   ├── kube.go                  # BuildClient — kubeconfig precedence: --kubeconfig > $KUBECONFIG > ~/.kube/config; --context override
│   ├── collect.go               # GetPod / ListPodEvents (sorted client-side) / TailLogs (30s sub-context) / CollectSnapshot
│   ├── types.go                 # Severity, Snapshot, Finding, Rule, Report, ContainerBrief — the data model
│   ├── errors.go                # Sentinel errors: ErrPodNotFound, ErrContainerNotReady
│   ├── rules.go                 # Rule registry + helpers (isSidecarInit, eventsForContainer, allRunnableContainerStatuses)
│   ├── priority.go              # resolveVerdict — sort + per-container suppression
│   ├── rule_pending_scheduling.go
│   ├── rule_pending_volume.go   # (was rule_pending_pvc.go — renamed to match function name)
│   ├── rule_init_container.go
│   ├── rule_image_pull.go
│   ├── rule_oom_killed.go
│   ├── rule_probe_failure.go
│   ├── rule_crashloop.go        # Fallback rule. Recently fixed to also detect State.Terminated{ExitCode!=0} with restartCount>=2
│   ├── rules_test.go            # Per-rule positive + negative tests (table-driven where it helps)
│   ├── sidecar_test.go          # Native-sidecar (restartPolicy: Always init container) classification tests
│   ├── priority_test.go         # Verdict picking + suppression tests (synthesised findings)
│   └── doctor_test.go           # End-to-end tests through Run with fake.NewSimpleClientset
│
├── internal/render/             # Output formatters. Importable by doctor only via the Renderer interface (no import cycle)
│   ├── adapter.go               # Adapter — implements doctor.Renderer over the package-level Render* functions
│   ├── banner.go                # Boxed Unicode verdict banner, severity-coloured, NO_COLOR-aware
│   ├── text.go                  # Sectioned text output — Status / Recent events / Logs / Findings
│   ├── json.go                  # BuildReport + RenderJSON — versioned, additive-only schema
│   ├── banner_test.go           # NO_COLOR strips ANSI; wrap respects width; padRight truncates
│   ├── text_test.go             # All sections present + section ORDER (catches reordering)
│   └── json_test.go             # Stable schema-key check + healthy pod has findings: [] (not null)
│
├── internal/version/
│   └── version.go               # ldflags-injected Version, Commit, Date + String() accessor
│
├── docs/
│   ├── output.schema.json       # JSON Schema 2020-12 doc locking the -o json output. schemaVersion is "1"
│   └── screenshots/
│       └── demo.gif             # 1100×760, ~692K, 939 frames. Recorded against a real kind cluster
│
├── scripts/
│   ├── stage-demo.sh            # Idempotent kind-cluster + 3-broken-pods setup. Required env: nothing. Writes /tmp/pod-doctor-demo.kubeconfig
│   └── demo.tape                # vhs tape file — declarative GIF recording
│
└── .github/workflows/
    ├── ci.yml                   # PR + main: vet + lint + race tests + build + codecov upload
    └── release.yml              # On v* tags: GoReleaser cross-compile + GitHub Release + Homebrew formula
```

---

## 3. Env vars and secrets

### Local development — none required

`pod-doctor` itself reads no secrets. The demo cluster is unauthenticated locally. You don't need to put any token anywhere.

### CI — none required

The default `GITHUB_TOKEN` that GitHub Actions provides is enough for the CI workflow. Codecov uploads via the public-repo tokenless path; if you fork into a private repo and want coverage uploads, add a `CODECOV_TOKEN` repo secret.

### Releases (`v*` tag pushes) — one secret required

To publish to the Homebrew tap on a release tag, you need:

| Secret name | Where to set it | What scope |
|---|---|---|
| `HOMEBREW_TAP_GITHUB_TOKEN` | https://github.com/NoobCoder1209/k8s-pod-doctor/settings/secrets/actions | A Personal Access Token with `repo` scope on `NoobCoder1209/homebrew-tap` |

To create the PAT:

1. Go to https://github.com/settings/tokens
2. *Generate new token (classic)*
3. Scope: `repo` (full control). Note: classic PAT is the simplest; fine-grained tokens also work but require explicit per-repo permissions.
4. Copy the token, paste it as the `HOMEBREW_TAP_GITHUB_TOKEN` secret value in the repo settings.

Without this secret, the release workflow's brews step fails. Everything else (binaries, checksums, GitHub Release page) still works.

### Optional Codecov account

The README has a Codecov badge. For it to populate, the repo needs to be added at https://app.codecov.io after the first push to `main`. Tokenless upload works for public repos, so no secret needed.

---

## 4. How to verify the demo actually worked

After step 5 in section 1, you should see four boxed banners. Each one:

- Has a `╔═...═╗` top border, a `║...║` middle, a `╚═...═╝` bottom.
- Shows a severity glyph (`✖` red for critical, `▲` yellow for warning, `✔` green for healthy).
- Names a verdict code from the seven supported patterns.

**Expected verdicts on the staged cluster:**

| Pod | Verdict code | Severity | Why |
|---|---|---|---|
| `broken-image` | `ImagePullBackOff` | critical | The image tag `nginx:notreal-image-tag-12345` does not exist on Docker Hub |
| `broken-oom` | `OOMKilled` | critical | Container allocates 200Mi under a 50Mi memory limit; the kernel OOM-kills it |
| `broken-crash` | `CrashLoopBackOff` | critical | Container runs `sh -c "...; exit 1"` → kubelet restarts it repeatedly |

**JSON pipeline expected output:**

```text
default/broken-crash: CrashLoopBackOff
default/broken-image: ImagePullBackOff
default/broken-oom: OOMKilled
```

If you see this, the demo works.

---

## 5. Common failure modes and their fixes

### `Cannot connect to the Docker daemon`

`kind` provisions K8s nodes as Docker containers. Start Docker Desktop (or `colima`, or whatever Docker runtime you use). The first lines of `stage-demo.sh` already check this and print a clear message — but if you skipped that script, this is the cause.

### `kind: command not found`

```bash
brew install kind
```

### `error: stat /tmp/pod-doctor-demo.kubeconfig: no such file or directory`

You ran `pod-doctor` before `./scripts/stage-demo.sh`. The kubeconfig is written by the staging script. Run it first.

### `pod-doctor: kubeconfig path does not exist`

Same as above, but you also passed `--kubeconfig=...` and the path is wrong. Verify with `ls -l /tmp/pod-doctor-demo.kubeconfig`.

### The verdict says `HEALTHY` but `kubectl get pods` shows `Error` or `CrashLoopBackOff`

If you're on a checkout from before commit `9dbd19e`, the crash-loop rule was too narrow — it only matched pods sitting in `State.Waiting.Reason == "CrashLoopBackOff"` and missed the brief windows when kubelet has the container in `State.Terminated`. That bug is fixed on `main`. Pull and rebuild:

```bash
git pull && make build
```

If you see this on a checkout that already includes `9dbd19e` or later, please open an issue — there's a new edge case in the rule.

### `pod-doctor default broken-image` works but the banner has no colour

You're running with `NO_COLOR=1` in your environment, or your terminal claims it doesn't support colour, or you passed `--no-color`. This is intentional. Unset `NO_COLOR` and try a colour-capable terminal.

### `vhs scripts/demo.tape` produces a GIF showing only the prompt

The tape's `Hide` block exports `KUBECONFIG`. If `/tmp/pod-doctor-demo.kubeconfig` doesn't exist, every `pod-doctor` invocation fails with the friendly error message — the GIF "works" (it records something) but isn't useful. Re-run `./scripts/stage-demo.sh` first.

### `golangci-lint not found` in CI but tests pass locally

CI installs `golangci-lint v1.60.3` automatically via `golangci/golangci-lint-action@v6`. Locally you'd need `brew install golangci-lint` to run `make lint`. The `make build` and `make test` targets don't need it.

### `ImagePullBackOff` for `polinux/stress` itself (not the test pod)

If your network can't reach Docker Hub, `broken-oom` will get stuck pulling its own image and never enter the OOM state. Check `docker pull polinux/stress` works first. As a workaround, pre-pull the image into the kind cluster:

```bash
kind load docker-image polinux/stress --name pod-doctor-demo
```

### `kind create cluster` hangs or times out

`--wait 60s` in `scripts/stage-demo.sh` is a *timeout*, not a delay — it tells kind how long to wait for the control plane to report Ready before giving up. If the control plane is genuinely slow, bumping it (e.g. to `--wait 180s`) gives a slow daemon more grace. But if it hangs forever, the underlying cause is usually Docker resource exhaustion: open Docker Desktop → Settings → Resources and confirm at least 2 CPU and 2Gi RAM are allocated. The control plane needs ~1Gi RAM minimum.

### Tests fail with `cannot find package "k8s.io/client-go/..."`

Run `go mod download` (or just `go test ./...` — Go fetches deps automatically). If that still fails, your `GOPROXY` may be blocked; try `GOPROXY=https://proxy.golang.org,direct go mod download`.

### Release workflow fails on the `brews` step

Either the `HOMEBREW_TAP_GITHUB_TOKEN` secret isn't set, or the PAT has expired, or the PAT doesn't have `repo` scope on `NoobCoder1209/homebrew-tap`. See section 3.

---

## Screenshot / GIF

The README embeds `docs/screenshots/demo.gif` at the top — recorded against a real `kind` cluster running the three deliberately-broken pods. Re-recorded on 2026-06-09 to capture the corrected `CrashLoopBackOff` verdict (see commit `9dbd19e`).

To re-record after changing the renderer or adding scenarios:
1. Edit `scripts/stage-demo.sh` to add new pod manifests if needed.
2. Edit `scripts/demo.tape` to add the corresponding `pod-doctor` invocations.
3. Run `./scripts/stage-demo.sh && make build && vhs scripts/demo.tape`.
