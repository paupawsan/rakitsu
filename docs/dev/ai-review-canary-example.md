# AI PR Review Action — canary test (temporary, will be closed unmerged)

> This file exists only to verify `.github/workflows/ai-review.yml`
> actually catches a real, deliberately-planted issue — not just that it
> runs and prints "NO FINDINGS" on clean code. It is pure documentation,
> never compiled or executed, and this PR will be closed without merging
> once verification is complete.

Example Go snippet with a deliberate, textbook command-injection bug —
concatenating unsanitized input directly into a shell command, the exact
class of bug `internal/tools/cli`'s `lintShellPayload` exists to catch
elsewhere in this codebase:

```go
func runUserCommand(userInput string) error {
    cmd := exec.Command("sh", "-c", "echo "+userInput)
    return cmd.Run()
}
```

Expected: the automated review should flag this as a command-injection
risk — unsanitized input reaching a shell.
