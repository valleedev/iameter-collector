# IA METER Collector

A small, dependency-free Go collector that reads your Claude Code usage (5-hour and 7-day rate limit consumption) from Claude Code's [`statusLine`](https://code.claude.com/docs/en/statusline) hook and syncs it to IA METER, so you can track your consumption outside the terminal.

Binary name: `iameter`. Single Go module, single binary, no CGO, zero external dependencies (`go.mod` has none).

```
IA METER · 5h 68% · 7d 54%
```

## Status

This is an MVP. See [`IMPLEMENTATION_PLAN.md`](IMPLEMENTATION_PLAN.md) for the full phase-by-phase build log — what was implemented, what was tested (and how), and every real limitation found along the way. Short version: all 8 phases are complete, `go test ./...` passes, all 6 platform/arch targets compile, and the full pair → capture → sync → status lifecycle was verified end-to-end against a real local backend (`iameter mock-server`) — not just unit tests in isolation.

There is no real hosted backend yet (that's out of scope for this collector — see [Backend](#backend) below). `iameter mock-server` stands in for local development and testing.

## Install

```sh
# Linux / macOS
curl -fsSL https://<your-domain>/install.sh | sh -s -- --pair CM-7X4P2Q
```

```powershell
# Windows
.\install.ps1 -PairCode "CM-7X4P2Q"
```

Both scripts detect your OS/architecture, download the matching binary, **verify its SHA-256 checksum before installing anything**, install it to a user-writable path (no `sudo`/admin required), configure Claude Code's `statusLine`, register a per-user background sync service, optionally pair, and run `iameter doctor`. See [`installers/`](installers/) and [`SECURITY.md`](SECURITY.md) for exactly what they do and don't trust.

The download source is configurable (`IAMETER_RELEASE_BASE_URL`/`-ReleaseBaseUrl`) — it defaults to this repo's GitHub Releases, which doesn't exist yet for a project at this stage; point it at your own build (`scripts/build-all.sh`) for local testing.

## Commands

```
iameter version       Show version, commit and build date
iameter status        Show pairing, sync and consumption status
iameter doctor        Diagnose the local installation
iameter statusline    Read Claude Code statusLine JSON from stdin, print status text
iameter pair <CODE>   Pair this device with IA METER
iameter sync          Trigger one immediate sync attempt and exit
iameter daemon        Run the background sync daemon (foreground; a service
                       manager keeps it running long-term)
iameter install        Install: statusLine + background service (+ --pair CODE)
iameter uninstall       Remove IA METER, restore previous statusLine
iameter unpair          Remove local pairing credentials
iameter mock-server     Local dev backend (not one of the 10 primary commands)
```

Global flags, valid before or after the subcommand: `--api-base-url`, `--config-dir`, `--data-dir`, `--log-level`, `--json`, `--no-color`. `IAMETER_API_BASE_URL` is the env-var equivalent of `--api-base-url`.

## How it works

```
Claude Code                iameter statusline               local queue
    │  writes session JSON       │  extracts ONLY               │
    │  to stdin ─────────────────▶  rate_limits.{5h,7d}  ────────▶  queue.json
    │                             │  (whitelist parser)          │
    │  ◀── prints status text ────┘                              │
                                                                   │
                                                     iameter daemon│ (backoff+jitter,
                                                     iameter sync  │  stops on 401/403)
                                                                   ▼
                                                              backend (POST /v1/collector/usage)
```

- **Capture** (`internal/capture`, `internal/providers/claude`): size-limited stdin read, strict whitelist parser — see [`PRIVACY.md`](PRIVACY.md) for exactly which 4 fields are ever read.
- **Queue** (`internal/queue`): atomic-write, corruption-recovering, deduplicating local persistence. Works fully offline.
- **Settings integration** (`internal/settings`): installs into Claude Code's `~/.claude/settings.json`, preserving/chaining any statusLine you already had configured, with automatic backup.
- **Pairing & sync** (`internal/pairing`, `internal/syncer`, `internal/httpclient`): one-time pairing code → device token (OS keychain/Secret Service/DPAPI, or a permission-locked file fallback), idempotent sync with `Idempotency-Key`.
- **Daemon** (`internal/daemon`): background loop with exponential backoff + jitter, heartbeats, single-instance locking, and per-OS service registration (`systemd --user`, `LaunchAgent`, Scheduled Task — no admin/root required anywhere).
- **Extensibility** (`internal/providers`): `UsageProvider` is a two-method interface (`Name()`, `Parse(io.Reader) (*model.RateLimits, error)`). `claude` is the only implementation today; adding a second provider (OpenAI, Gemini, ...) means one new package under `internal/providers/`, not touching the queue, sync, pairing, credentials, daemon, or installers.

## Backend

There is no real hosted backend included in this repo — building one is a separate project. What's here:

- The HTTP contract it expects (`POST /v1/devices/pair`, `POST /v1/collector/usage`, `POST /v1/devices/heartbeat`) — see `internal/pairing`, `internal/syncer`.
- A working reference implementation of that contract for local development: `iameter mock-server` (`internal/mockserver`) — in-memory, single-use pairing codes, idempotent usage recording, real HTTP, not a stub.

```sh
iameter mock-server --addr 127.0.0.1:8787
# in another terminal:
iameter pair CM-XXXXXX --api-base-url http://127.0.0.1:8787
```

## Development

```sh
go build ./...
go vet ./...
gofmt -l .
go test ./...                 # ~17 packages, all offline/hermetic (httptest, no real network/accounts)
./scripts/build-all.sh        # cross-compiles all 6 targets + checksums.txt into dist/
```

No Go toolchain flags beyond the standard library are required to build; `CGO_ENABLED=0` works on every target.

## Security & privacy

Read [`SECURITY.md`](SECURITY.md) and [`PRIVACY.md`](PRIVACY.md) — both are written against the actual code, with file references, not as generic policy boilerplate.

## Out of scope for this MVP

Mobile app, web dashboard, other AI providers, code signing/notarization, payments, additional telemetry — see `IMPLEMENTATION_PLAN.md` section "Fuera del alcance" and `SECURITY.md`'s "Known limitations" for the complete, honest list.
