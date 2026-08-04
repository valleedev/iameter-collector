# Privacy

This document states exactly what IA METER Collector collects and sends, and — just as importantly — what it never touches. Every claim here is checked against the actual code (file/line references included) rather than written as an aspirational policy.

## What IA METER collects

From the JSON Claude Code passes to its statusLine command, IA METER reads **only**:

- `rate_limits.five_hour.used_percentage`
- `rate_limits.five_hour.resets_at`
- `rate_limits.seven_day.used_percentage`
- `rate_limits.seven_day.resets_at`

Enforced by `internal/providers/claude/claude.go`'s `rawEnvelope`/`rawRateLimits`/`rawWindow` structs, which declare no other field — anything else in the statusLine JSON has nowhere to be unmarshaled into and is discarded by `encoding/json`, not filtered after the fact.

IA METER additionally generates, locally, and includes in the synced payload:

- `device_id` — a locally-generated random identifier (`dev_...`) until pairing succeeds, after which the backend's own assigned `device_id` is used instead (`internal/device`, `internal/cli/pair_cmd.go`).
- `collector_version` — this software's own version string.
- `captured_at` — the time the snapshot was captured, RFC3339 UTC.
- `provider` — currently always the literal string `"claude"`.
- `platform.os` / `platform.arch` — e.g. `"linux"`/`"amd64"`, from Go's `runtime` package.

That's the complete outgoing payload shape (`internal/model.UsageSnapshot`) — six fields, nothing else. There is no field in the Go struct for anything not listed above; adding a seventh field would require a code change to `internal/model/model.go`, not a configuration change.

At **pairing time only** (`iameter pair <CODE>`, section 16), a device *name* is also sent — by default your computer's hostname (`os.Hostname()`, via `internal/device.DefaultName`) — so you can tell your devices apart in your account later. This is sent once, at pairing, not on every sync. **Known limitation:** there is currently no `--name` flag to override this with a custom label before pairing; if you don't want your literal hostname sent, rename your computer first or (once a real backend/dashboard exists) rename the device there afterward.

## What IA METER never collects

Regardless of what appears in Claude Code's statusLine JSON, IA METER never reads, stores, or transmits:

- conversations, prompts, or model responses
- your source code or project files
- file paths or file names (project directories, transcript paths, etc.)
- Git branch names or repository state
- environment variables
- the full/raw statusLine JSON payload, in logs or anywhere else
- Claude's authentication tokens, session cookies, or OAuth credentials
- browser cookies or browsing history
- your system username, local IP address, or full hostname in the *usage sync* payload specifically (the hostname is only ever sent once, at pairing, as the device's human-readable label — see above)

This isn't a promise layered on top of code that could do otherwise — the parser's whitelist is structural (see "What IA METER collects"), and the sync payload type has no field to carry any of the above even if a future bug tried to populate one.

## Local storage

Between capture and successful sync, snapshots sit in a local, plaintext, whitelisted-fields-only queue (`internal/queue`, `<data-dir>/queue.json`) — the same six-field shape as the network payload, nothing more. The device token is stored separately, via the OS credential store (see `SECURITY.md`); it is not usage data and is never included in any synced snapshot.

## Third parties

IA METER talks to exactly one external service: the backend configured via `--api-base-url`/`IAMETER_API_BASE_URL`. There is no analytics SDK, crash reporter, telemetry pipeline, or third-party dependency in this codebase (`go.mod` declares zero external dependencies) that could send data anywhere else.

## Your controls

- `iameter status` / `iameter status --json` shows exactly what's queued/last captured.
- `iameter unpair` deletes the stored device token; the backend can no longer attribute new syncs to this device.
- `iameter uninstall` removes IA METER from Claude Code's `statusLine` configuration.
- Uninstalling does not, by default, delete the local queue/device-id cache — `installers/uninstall.sh`/`.ps1` print the exact paths to remove them by hand if you want a full wipe.
