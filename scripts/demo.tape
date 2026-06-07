# pod-doctor demo tape — run with:  vhs scripts/demo.tape
#
# Prerequisites:
#   1. ./scripts/stage-demo.sh   (kind cluster + 3 broken pods; writes
#                                  /tmp/pod-doctor-demo.kubeconfig)
#   2. make build                (produces ./pod-doctor)
#
# stage-demo.sh and this tape share a deterministic KUBECONFIG path.
#
# Output: docs/screenshots/demo.gif

Output docs/screenshots/demo.gif

Set Theme "Dracula"
Set FontSize 14
Set Width  1100
Set Height 760
Set TypingSpeed 35ms
Set PlaybackSpeed 1.0

Hide
Type "export KUBECONFIG=/tmp/pod-doctor-demo.kubeconfig && clear"
Enter
Sleep 800ms
Show

Type "# 1. ImagePullBackOff — typo'd image tag"
Sleep 800ms
Enter
Type "./pod-doctor default broken-image"
Sleep 400ms
Enter
Sleep 6s

Hide
Type "clear"
Enter
Show

Type "# 2. OOMKilled — container memory limit exceeded"
Sleep 800ms
Enter
Type "./pod-doctor default broken-oom"
Sleep 400ms
Enter
Sleep 6s

Hide
Type "clear"
Enter
Show

Type "# 3. CrashLoopBackOff — failing exec"
Sleep 800ms
Enter
Type "./pod-doctor default broken-crash"
Sleep 400ms
Enter
Sleep 6s

Hide
Type "clear"
Enter
Show

Type "# 4. JSON output for pipelines"
Sleep 800ms
Enter
Type "./pod-doctor default broken-image -o json | jq '.verdict.code'"
Sleep 400ms
Enter
Sleep 4s
