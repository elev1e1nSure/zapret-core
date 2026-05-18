# zapret-core

[English](README.md) | [Русский](README.ru.md)

![Go](https://img.shields.io/badge/Go-1.21-blue)
![Platform](https://img.shields.io/badge/platform-Windows-lightgrey)
![License](https://img.shields.io/badge/license-MIT-green)
![Release](https://img.shields.io/github/v/release/elev1e1nSure/zapret-core)
![Downloads](https://img.shields.io/github/downloads/elev1e1nSure/zapret-core/total)

DPI bypass tool for YouTube and Discord on Windows. Finds a working strategy for your ISP automatically, remembers it, and recovers when your ISP updates their blocking — no manual configuration needed.

Built on top of [zapret](https://github.com/bol-van/zapret) by bol-van and inspired by [zapret-discord-youtube](https://github.com/flowseal/zapret-discord-youtube) by Flowseal.

---

## Why this exists

Most DPI bypass tools give you a list of 80+ strategies and say "try them one by one". zapret-core does that for you: it tests parameter combinations, finds what actually works for your ISP, and remembers the result. Next run it starts with the best known strategy immediately. If your ISP updates their blocking — watchdog detects it and finds a new one automatically.

---

## How it works

1. **ISP detection** — queries `ipinfo.io` to get your ASN (e.g. `AS12389 Rostelecom`). The ASN is used as a key to look up known-good strategies.
2. **Strategy search** — up to 137 parameter combinations are tested against YouTube, Discord, and Google. Each combination runs winws (from zapret) with specific DPI bypass flags, waits for it to initialize, then probes the targets over HTTP. A score from 0 to 1 is computed from how many targets responded successfully.
3. **Knowledge base** — once a strategy passes the score threshold, it's saved to `data/knowledge.json` keyed by ASN. On every subsequent run that strategy is tried first — search only happens if it stopped working.
4. **Watchdog** — a background loop probes YouTube and Discord on a configurable interval. When consecutive failures exceed the threshold, the optimizer runs again and seamlessly switches to a new working strategy.
5. **Self-update** — compares local version against the latest GitHub release, downloads the zip, verifies SHA256, atomically swaps the binary via a rename-over trick (`.old` → new), and exits. The next run uses the new binary.

---

## Requirements

- Windows 7 or later (x64)
- Administrator rights — WinDivert installs a kernel driver
- Internet connection for ISP detection, probing, and updates

---

## Installation

> **[Download latest release](https://github.com/elev1e1nSure/zapret-core/releases/latest)**

Extract the archive anywhere. Required layout:

```
zapret-core.exe
assets/
    winws.exe
    WinDivert.dll
    WinDivert64.sys
    cygwin1.dll
    fake/
        *.bin          ← fake packet payloads used by some strategies
lists/
    list-general.txt   ← domains to bypass
    list-google.txt
    ipset-all.txt      ← IP ranges
    ipset-exclude.txt
    list-exclude.txt
data/                  ← created automatically on first run
    config.json
    knowledge.json
    zapret.log
```

> **Run as Administrator.** Right-click → "Run as administrator", or launch from an elevated terminal.

### Verify download

Each release includes `checksums.txt` with SHA256 hashes. To verify:

```powershell
Get-FileHash zapret-core-v1.2.11-windows-amd64.zip -Algorithm SHA256
```

Compare the output against `checksums.txt`. Matching means the file is authentic and uncorrupted.

---

### Build from source

<details>
<summary>Instructions</summary>

Requires Go 1.21+ and Windows.

```bash
git clone https://github.com/elev1e1nSure/zapret-core.git
cd zapret-core
go build -o zapret-core.exe .
```

Note: local builds are console-mode binaries (show output in terminal). Release builds use `-H windowsgui` so no console window appears when launched outside a terminal — this is intentional for use with a UI wrapper.

Or use the build script which puts everything into `dist/`:

```bash
build.bat
```

</details>

---

## Usage

### Quick start

```
zapret-core.exe
```

Detects your ISP, loads the best strategy from the knowledge base, and runs until you press Ctrl+C. If the knowledge base is empty — tells you to run `--find` first.

---

### Find a working strategy

```
zapret-core.exe --find
```

Tests up to 137 combinations and stops at the first one that passes the score threshold:

```
[1/137] Testing: auto-1 [fake/ts/file]
  score=0.33  YouTube:FAIL  Discord:FAIL  Google:OK

[4/137] Testing: auto-4 [fake/badseq/file]
  score=1.00  YouTube:OK  Discord:OK  Google:OK

[+] Working strategy found: auto-4 [fake/badseq/file]
```

The result is saved to `data/knowledge.json` and used on every subsequent run.

Typical time: a few minutes. Worst case: up to 2 hours if nothing works early. Most users find something within the first 10–20 attempts.

> Run `--find` again any time if your ISP changes their blocking and the watchdog can't recover automatically.

---

### Monitor with auto-recovery

```
zapret-core.exe --watch
```

Runs in the background (no console window in release builds). Probes YouTube and Discord every 60 seconds. Three failures in a row triggers the optimizer — it finds a new strategy and switches without any interaction. Stop with Ctrl+C; winws is stopped cleanly.

---

### Status

```
zapret-core.exe --status
```

Shows whether winws is running and what strategy is active. Exits immediately.

---

### Stop

```
zapret-core.exe --stop
```

Stops winws. Exits immediately.

---

### Reset strategies

```
zapret-core.exe --reset
```

Removes all saved strategies for your current ASN. Use this when you want to force a clean search — for example after switching ISP or VPN.

---

### Export / Import strategies

```
zapret-core.exe --export strategies.json
zapret-core.exe --import strategies.json
```

Export saves all strategies for your ASN to a JSON file. Import merges them into the knowledge base. Useful for sharing a working config between machines or restoring after a reinstall.

---

### Update lists

```
zapret-core.exe --updatelists
```

Downloads the latest IP and domain lists from the Flowseal repository:

```
[1/5] Updating ipset-all.txt...
[2/5] Updating ipset-exclude.txt...
[3/5] Updating list-exclude.txt...
[4/5] Updating list-general.txt...
[5/5] Updating list-google.txt...
Lists updated successfully.
```

Uses atomic updates — each file is downloaded to a `.tmp` copy first, then renamed. If a download fails, existing files stay untouched.

---

### Self-update

```
zapret-core.exe --update
```

Checks GitHub Releases for a newer version. If found:
1. Downloads `zapret-core-vX.Y.Z-windows-amd64.zip`
2. Verifies SHA256 against `checksums.txt`
3. Extracts `zapret-core.exe` from the zip
4. Atomically replaces the current binary (rename `.old` → new)
5. Exits with a message to restart

The old binary is left as `zapret-core.exe.old` and cleaned up the next time the program starts.

---

### HTTP API server

```
zapret-core.exe --server
```

Starts an HTTP server on `127.0.0.1:7432` for integration with a UI or external tools. Runs in daemon mode (no console window in release builds). Stop with Ctrl+C.

---

## Daemon mode

`--server` and `--watch` are **daemon modes**. In release builds (compiled with `-H windowsgui`) the process has no console window at all — it runs silently in the background. This is intentional: these modes are meant to be managed by a UI wrapper or a startup task, not interacted with directly.

In a locally-built binary (without `-H windowsgui`) the console window appears but is immediately hidden via `ShowWindow(SW_HIDE)` before any output is printed.

---

## API Reference

<details>
<summary>All endpoints are local-only (127.0.0.1:7432)</summary>

### Conflict handling

Any endpoint that triggers a long-running operation (`/api/find`, `/api/update`, `/api/update-self`, `/api/start`, `/api/stop`) returns `409 Conflict` if another operation is already running:

```json
{ "error": "operation in progress: find" }
```

Wait for it to finish or call `POST /api/stop` to abort.

---

### GET /api/version

```json
{ "version": "v1.2.14" }
```

---

### GET /api/status

```json
{
  "winws_running": true,
  "watchdog_running": false,
  "current_strategy": "auto-4 [fake/badseq/file]",
  "provider": { "ASN": "AS12389", "Org": "Rostelecom", "Region": "Moscow Oblast" },
  "operation_in_progress": false,
  "operation_type": ""
}
```

---

### GET /api/provider

```json
{ "ASN": "AS12389", "Org": "Rostelecom", "Region": "Moscow Oblast" }
```

---

### GET /api/health

Always returns `200`. Use this to check if the server is up after launching.

```json
{ "ok": true, "version": "v1.2.14" }
```

---

### GET /api/knowledge

Returns all strategies saved for the current ASN, ordered by score descending.

```json
{
  "entries": [
    { "asn": "AS12389", "vector": {...}, "score": 1.0, "hits": 5, "last_seen": "2026-05-17T..." }
  ],
  "total": 1
}
```

---

### POST /api/start

Starts the best known strategy for the current ASN. Returns `404` if the knowledge base has no entries yet.

```json
{ "status": "started", "strategy": "auto-4 [fake/badseq/file]" }
```

---

### POST /api/stop

```json
{ "status": "stopped" }
```

---

### POST /api/watchdog

Starts watchdog in the background. Returns immediately.

```json
{ "status": "started", "message": "watchdog running in background" }
```

---

### DELETE /api/watchdog

Stops watchdog and winws.

```json
{ "status": "stopped" }
```

---

### SSE event format

All SSE endpoints use a unified envelope:

```json
{ "type": "...", "message": "...", "data": { ... } }
```

- `type` — event kind (`progress`, `success`, `error`, `up_to_date`, `shutdown`, `log`, `status`)
- `message` — human-readable description
- `data` — optional structured payload (omitted when null)

---

### POST /api/find — SSE

Starts strategy search. Streams progress until a result is found or all combinations are exhausted.

```
data: {"type":"progress","message":"[3/137] Testing: auto-3 [fake/ts/file]","data":{"current":3,"total":137,"strategy":"auto-3 [fake/ts/file]","score":0.33}}

data: {"type":"success","message":"Strategy found","data":{"strategy":{...},"score":1.0,"vector":{...}}}

data: {"type":"error","message":"no working strategy found"}
```

---

### POST /api/update — SSE

Downloads updated lists from GitHub. Streams progress.

```
data: {"type":"progress","message":"[1/5] Updating ipset-all.txt...","data":{"current":1,"total":5,"filename":"ipset-all.txt"}}

data: {"type":"success","message":"lists updated successfully"}

data: {"type":"error","message":"download ipset-all.txt: HTTP 404"}
```

---

### POST /api/update-self — SSE

Checks for a newer release and applies it. Stages: `checking` → `found` → `downloading` → `verifying` → `applying` → `success` or `up_to_date` or `error`.

```
data: {"type":"checking","message":"Checking for updates..."}
data: {"type":"found","message":"New version available: v1.2.13 → v1.2.14"}
data: {"type":"downloading","message":"Downloading zapret-core-v1.2.14-windows-amd64.zip..."}
data: {"type":"verifying","message":"Verifying SHA256..."}
data: {"type":"applying","message":"Applying update..."}
data: {"type":"success","message":"Update installed (v1.2.13 → v1.2.14). Please restart the server."}
data: {"type":"shutdown","message":"Server is shutting down for update. Restart to apply."}
```

After `shutdown` the server process calls `os.Exit(0)`. The client should detect the closed connection and restart the process, then poll `GET /api/health` until `200`.

When already up to date:

```
data: {"type":"up_to_date","message":"Already up to date (v1.2.14)"}
```

---

### GET /api/events — SSE (persistent)

Long-lived SSE stream. Sends current state immediately on connect, then pushes an event on every state change (start, stop, watchdog toggle).

```
data: {"type":"status","data":{"running":true,"watchdog":false,"strategy":"auto-4 [fake/badseq/file]"}}
```

Keep-alive comment every 15 seconds:

```
: ping
```

Multiple concurrent clients are supported. The stream stays open until the client disconnects.

---

### GET /api/logs — SSE

Streams log output. On connect, sends the last N lines as a backlog, then tails new lines as they are written.

Query param: `?lines=N` (default `100`). Use `?lines=0` for live-only (no backlog).

```
data: {"type":"log","message":"[INFO] ℹ winws started with strategy auto-4 [fake/badseq/file]"}
data: {"type":"log","message":"[OK] ✓ YouTube: OK  Discord: OK  Google: OK"}
```

New lines arrive within ~250ms of being written. The stream stays open until the client disconnects.

</details>

---

## Configuration

`data/config.json` is created with defaults on first run. All fields are optional — missing ones fall back to defaults.

```json
{
  "score_threshold": 0.6,
  "fail_threshold": 3,
  "check_interval": 60,
  "init_delay": 5,
  "test_timeout": 8,
  "test_runs": 2
}
```

| Parameter | Default | Description |
|---|---|---|
| `score_threshold` | `0.6` | Minimum score (0–1) to accept a strategy. Lower = more permissive, higher = stricter. |
| `fail_threshold` | `3` | How many consecutive probe failures before watchdog triggers recovery. |
| `check_interval` | `60` | Watchdog probe interval in seconds. |
| `init_delay` | `5` | Seconds to wait after winws starts before probing. Increase if you get false negatives on slow machines. |
| `test_timeout` | `8` | Timeout per HTTP probe in seconds. Increase on slow or high-latency connections. |
| `test_runs` | `2` | How many times to repeat each probe for stability. Higher = slower search but fewer false positives. |

---

## Knowledge Base

`data/knowledge.json` stores the strategies that worked for each ISP, keyed by ASN. On startup the best-scored strategy for your ASN is loaded and tried first — a full search only runs if it fails or if no entry exists.

- **Delete the file** to force a clean search from scratch.
- **Export/import** to move strategies to another machine.
- The file doesn't grow indefinitely — duplicate entries for the same ASN+vector are updated in place.

---

## File layout

| Path | Description |
|---|---|
| `zapret-core.exe` | Main binary |
| `assets/winws.exe` | DPI bypass engine (from zapret) |
| `assets/WinDivert.dll` / `.sys` | Kernel packet capture driver |
| `assets/fake/*.bin` | Fake packet payloads used by certain strategies |
| `lists/*.txt` | Domain and IP range lists for bypass |
| `data/config.json` | Runtime configuration (auto-created) |
| `data/knowledge.json` | Strategy memory (auto-created) |
| `data/zapret.log` | Log file (auto-created) |

---

## Conflict Detection

Before searching, zapret-core checks for software that interferes with WinDivert at the kernel level:

- GoodbyeDPI
- AdGuardSvc
- discordfix_zapret
- winws1, winws2 (other instances)
- Killer NIC / Intel Connectivity Network Service
- Check Point (TracSrvWrapper, EPWD)
- SmartByte

If any are found, the search stops with a list of what was detected. Disable the conflicting software and retry.

---

## Logs

Logs are written simultaneously to the console and `data/zapret.log`.

| Level | Meaning |
|---|---|
| `[INFO]` | Normal operation |
| `[WARN]` | Non-fatal issue (probe failure, dropped event) |
| `[ERROR]` | Operation failed |
| `[OK]` | Success confirmation |

The log file is not rotated — delete it manually if it grows too large.

---

## Troubleshooting

<details>
<summary>Common issues</summary>

**"No known strategies. Run --find"**  
The knowledge base is empty or has no entry for your ISP. Run `--find`.

**"No working strategy found"**  
No combination passed the score threshold. Possible causes:
- Your connection is too slow → increase `test_timeout`
- Score threshold is too strict → lower `score_threshold` to `0.4`
- ISP uses a blocking method not covered by the current strategy set

**"Resolve conflicts and try again"**  
A conflicting process is running. Check the list above, stop it, then retry.

**"failed to start winws"**  
Either `assets/winws.exe` is missing (incomplete extraction) or the process isn't running as Administrator.

**Watchdog triggers constantly**  
The active strategy is borderline. Run `--find` again to get a more stable one, or increase `test_runs` in config for stricter selection.

**409 Conflict in API**  
Another operation is in progress. Wait for it to finish or send `POST /api/stop`.

**After `--update` the version didn't change**  
The binary was replaced but you're still running the old process. Restart the application. In `--server` mode, the server exits automatically after a successful update — just restart it.

**Double-clicking the exe does nothing**  
Release builds have no console window (`-H windowsgui`). The process runs in the background. Use a terminal (PowerShell / cmd) to see output, or use the `--server` mode and connect via the API.

</details>

---

## Credits

- [bol-van](https://github.com/bol-van/zapret) — zapret, winws, WinDivert, and the fake packet binaries
- [Flowseal](https://github.com/flowseal/zapret-discord-youtube) — strategy presets and parameter research that shaped the search space

---

## License

[MIT](LICENSE) © elev1e1nSure