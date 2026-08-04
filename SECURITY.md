# Security

This document describes IA METER Collector's threat model, what it does and doesn't collect, how credentials are handled, and how to report a vulnerability. It is written to match the actual code in this repository — every claim below was checked against the implementation while writing it, not written in advance and hoped to be true.

## Threat model

IA METER Collector runs on the same machine as Claude Code, with the same user's privileges. It is designed to defend against:

- **A malicious or buggy statusLine payload.** The JSON Claude Code writes to the collector's stdin is treated as untrusted input: size-limited (1 MiB, `internal/capture`), strictly whitelist-parsed (`internal/providers/claude`), and never executed or interpreted as anything other than data.
- **A malicious or unreachable backend.** HTTP responses are size-limited (1 MiB, `internal/httpclient`), have a fixed timeout (15s), and unexpected status codes never crash the collector — they're logged and the sync attempt stops until the next scheduled try.
- **A compromised or manipulated release binary.** The installers (`installers/install.sh`, `install.ps1`) verify a SHA-256 checksum before installing anything, and abort without touching the system if it doesn't match.
- **Path traversal and symlink attacks** against files IA METER writes: every file is written via an atomic temp-file-then-rename (`internal/fsutil.AtomicWriteFile`), which replaces whatever is at the destination path rather than following it — a symlink placed at a target path is replaced, never followed and written through. `internal/settings` additionally refuses outright to touch Claude Code's `settings.json` if it's a symlink (`ErrSymlink`). The credential file-fallback store rejects any key containing a path separator.
- **Command injection.** No command line is ever built by concatenating untrusted strings into a shell template. The one place a full shell command is executed (`internal/statusline.RunChained`, section 13's statusLine-chaining feature) re-invokes exactly the command Claude Code itself already had configured and already trusted as *its own* statusLine command — IA METER doesn't add a new trust boundary there, it re-runs an existing one, with the child process's environment stripped of any `IAMETER_`-prefixed variables and a hard timeout (3s) enforced via process-group termination so a hung child can't hang the collector.

IA METER Collector does **not** defend against a fully compromised local user account or machine — if an attacker already has your user's code-execution privileges, no local application-level control here changes that.

## Data collected

See `PRIVACY.md` for the full list. In security terms, the relevant fact is that the outgoing sync payload (`model.UsageSnapshot`, `internal/model/model.go`) is a fixed Go struct with exactly six top-level fields — `device_id`, `provider`, `collector_version`, `captured_at`, `platform`, `rate_limits` — and the parser that produces `rate_limits` (`internal/providers/claude`) only ever unmarshal-populates a struct that itself only declares `rate_limits.{five_hour,seven_day}.{used_percentage,resets_at}`. Any other field in Claude Code's statusLine JSON (model name, cost, context window, session id, transcript path, git branch, working directory, environment variables, ...) has no field to be unmarshaled into and is silently discarded by `encoding/json` — this is enforced at the type level, not by a filter that could be forgotten. `internal/providers/claude/claude_test.go`'s `TestParseExtraSensitiveFieldsIgnored` and the `testdata/statusline/extra-sensitive-fields.json` fixture exercise this directly.

## Credential storage

Device tokens (obtained by `iameter pair`) are stored via `internal/credentials.Store`:

| OS | Backing store | Mechanism |
|---|---|---|
| Linux | Secret Service (GNOME Keyring, KWallet, ...) | shells out to `secret-tool`, if present and reachable |
| macOS | Keychain | shells out to `security` |
| Windows | DPAPI (`CryptProtectData`/`CryptUnprotectData`) | `syscall.NewLazyDLL`, stdlib only, no CGO |
| Fallback (any OS) | A file per credential, `0600` permissions | used only when no OS-native store is available/reachable |

`Store.IsFallback()` is surfaced to the user by `iameter doctor` and `iameter pair` as an explicit `[WARN]` whenever the fallback is in use, per the requirement that a weaker storage mechanism never be used silently.

Tokens are:
- never written to Claude Code's `settings.json`,
- never passed as a command-line argument to any process,
- never included in any log line — no code path in this project logs a token, in any form, redacted or not,
- never exposed to a chained statusLine child process (explicit environment filtering, see above).

The one-time pairing code itself is never persisted anywhere; it exists only in memory for the duration of the `POST /v1/devices/pair` call.

## Transport

All backend communication is `net/http` over whatever scheme `--api-base-url`/`IAMETER_API_BASE_URL` specifies. The default value points at `http://127.0.0.1:8787`, the local development mock server — **not a production endpoint** (`iameter doctor`/`status` warn explicitly when this default is still in effect). A real deployment must configure an `https://` URL; the collector does not force TLS itself because it has no way to distinguish "the operator configured a real HTTPS backend" from "the operator is deliberately testing against plaintext localhost" — that judgment is left to whoever configures `IAMETER_API_BASE_URL`. Every request carries `Idempotency-Key` (usage sync) so a retried request is never double-counted server-side (verified with `internal/mockserver`'s idempotent-replay test).

## Revocation

There is no remote-revocation endpoint implemented in this MVP (out of scope, section 34 — no dashboard/backend beyond the local mock server). Revoking a device's access today means the real backend (once built) invalidating its stored device token; the collector will then get `401`/`403` on its next sync attempt, which `internal/syncer`/`internal/daemon` treat as a hard stop — no further automatic retries — until `iameter unpair` and `iameter pair` are run again.

## Updates

There is no auto-update mechanism. Re-running the install script (`installers/install.sh`/`install.ps1`) downloads and checksum-verifies the current release and replaces the binary; the CLI itself never modifies its own executable.

## Reporting a vulnerability

This is a sample/MVP project without a public release yet. If you find a security issue while evaluating this code, please open an issue in this repository describing the concern rather than a working exploit, so it can be triaged before public details are posted.

## Known limitations

- **No code signing.** Windows binaries are unsigned; macOS binaries are unsigned and unnotarized. A public distribution of this software would need both (section 28) — SmartScreen/Gatekeeper will warn users until then. This is explicitly out of scope for the MVP (section 34).
- **The credential fallback store is not encrypted at rest**, only permission-restricted (`0600`). This matches the spec's explicit requirement ("archivo con permisos restrictivos") but is weaker than an OS keychain; it only activates when no native store is reachable, and is always flagged as a `[WARN]`.
- **`secret-tool`/`security`/DPAPI availability was not exhaustively tested on real macOS/Windows machines** in the environment this was built in (Linux, without `secret-tool` installed) — the Linux fallback path was verified for real (this sandbox has no D-Bus Secret Service daemon, so `credentials.New()` was confirmed to fall back correctly); the macOS/Windows implementations were verified by cross-compilation and `go vet` only, not runtime execution. See `IMPLEMENTATION_PLAN.md` Phase 0/8 for the full list of environment-driven testing limitations.
- **Service registration (`systemd --user`/`launchctl`/`schtasks`) was deliberately not exercised against a real live session** during development, to avoid mutating the developer machine's actual service state as a side effect of building this project — see `IMPLEMENTATION_PLAN.md` Phase 6 for the reasoning. The unit/plist/task generation logic itself is unit-tested.
- **No rate limiting or abuse protection** on the collector side beyond the backend's own `429`/`Retry-After` handling — a local attacker with code execution could enqueue arbitrary (but still schema-valid) snapshots; this is not considered a meaningful additional attack surface given the "already has your user's privileges" threshold in the threat model above.
