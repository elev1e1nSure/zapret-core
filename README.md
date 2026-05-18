# zapret-core

[English](README.md) | [Русский](README.ru.md)

![Go](https://img.shields.io/badge/Go-1.21-blue)
![Platform](https://img.shields.io/badge/platform-Windows-lightgrey)
![License](https://img.shields.io/badge/license-MIT-green)
![Release](https://img.shields.io/github/v/release/elev1e1nSure/zapret-core)
![Downloads](https://img.shields.io/github/downloads/elev1e1nSure/zapret-core/total)

DPI bypass tool for YouTube and Discord on Windows. Finds a working strategy for your ISP on its own, remembers it, and recovers when your ISP updates their blocking — no manual configuration needed.

Built on top of [zapret](https://github.com/bol-van/zapret) by bol-van and inspired by [zapret-discord-youtube](https://github.com/flowseal/zapret-discord-youtube) by Flowseal.

---

## Why this exists

Most DPI bypass tools give you a list of 80+ strategies and say "try them one by one". zapret-core does that for you: it tests parameter combinations, finds what actually works for your ISP, and remembers the result. Next run it starts with the best known strategy immediately. If your ISP updates their blocking — watchdog detects it and finds a new one automatically.

---

## Requirements

- Windows 7+
- Administrator rights — WinDivert installs a kernel driver, won't work without it
- Internet connection for ISP detection and testing

---

## Installation

> **[Download latest release](https://github.com/elev1e1nSure/zapret-core/releases/latest)**

Extract the archive. The structure should look like this:

```
zapret-core.exe
assets/
    winws.exe
    WinDivert.dll
    WinDivert64.sys
    cygwin1.dll
    fake/
        *.bin
lists/
    list-general.txt
    list-google.txt
    ipset-all.txt
    ...
```

The `data/` folder is created automatically on first run.

### Verify download

Each release includes a `checksums.txt` file with SHA256 hashes. To verify your download:

```powershell
Get-FileHash zapret-core-v1.0.2-windows-amd64.zip -Algorithm SHA256
```

Compare the output with the hash in `checksums.txt`. If they match, the file is authentic and uncorrupted.

---

### Build from source

<details>
<summary>Instructions</summary>

You need Go 1.21+ and Windows.

```bash
git clone https://github.com/elev1e1nSure/zapret-core.git
cd zapret-core
go build -o zapret-core.exe .
```

Or use the build script — it puts everything into `dist/`:

```bash
build.bat
```

</details>

---

## Usage

### Just run it

```
zapret-core.exe
```

Detects your ISP, loads the best strategy from the knowledge base, and runs until Ctrl+C. If the knowledge base is empty — prompts you to run `--find` first.

---

### Find a working strategy

```
zapret-core.exe --find
```

Tests up to 137 combinations and stops at the first one that works:

```
[1/137] Testing: auto-1 [fake/ts/file]
  score=0.33  YouTube:FAIL  Discord:FAIL  Google:OK

[4/137] Testing: auto-4 [fake/badseq/file]
  score=1.00  YouTube:OK  Discord:OK  Google:OK

[+] Working strategy found: auto-4 [fake/badseq/file]
```

The result is saved and used on subsequent runs.

Best case — a few minutes. Worst case — up to 2 hours. In practice most users find something within the first 10–20 attempts.

---

### Monitor with auto-recovery

```
zapret-core.exe --watch
```

Checks YouTube and Discord every 60 seconds. Three failures in a row — automatically finds a new strategy and switches. Stop with Ctrl+C, everything shuts down cleanly.

---

### Status

```
zapret-core.exe --status
```

Shows whether winws is running and which strategy is in use. Exits immediately.

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

Clears all saved strategies for your current ASN from the knowledge base. Useful when nothing works and you want a clean search.

---

### Export / Import strategies

```
zapret-core.exe --export strategies.json
zapret-core.exe --import strategies.json
```

Share working strategies between machines or back them up.

---

### Update lists

```
zapret-core.exe --updatelists
```

Downloads updated lists from the Flowseal repository:

```
[1/5] Updating ipset-all.txt...
[2/5] Updating ipset-exclude.txt...
[3/5] Updating list-exclude.txt...
[4/5] Updating list-general.txt...
[5/5] Updating list-google.txt...
Lists updated successfully.
```

On download error, existing files are left unchanged.

---

### Self-update

```
zapret-core.exe --update
```

Checks GitHub Releases for a newer version, downloads it, verifies SHA256, and restarts automatically.

---

### HTTP API

```
zapret-core.exe --server
```

Starts an HTTP server on `127.0.0.1:7432` for integration with external applications. Stop with Ctrl+C.

---

## API Reference

<details>
<summary>All endpoints are local-only (127.0.0.1:7432)</summary>

### Conflict handling

Any endpoint that starts a long-running operation (`/api/find`, `/api/update`, `/api/update-self`, `/api/start`, `/api/stop`) returns `409 Conflict` if another operation is already in progress:

```json
{ "error": "operation in progress: find" }
```

Wait for the current operation to finish or call `POST /api/stop` to abort it.

---

### GET /api/version

```json
{ "version": "v1.2.1" }
```

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

### GET /api/provider

```json
{ "ASN": "AS12389", "Org": "Rostelecom", "Region": "Moscow Oblast" }
```

### GET /api/knowledge

```json
{
  "entries": [
    { "asn": "AS12389", "score": 1.0, "hits": 5, "last_seen": "2026-05-17T..." }
  ],
  "total": 1
}
```

### POST /api/start

Starts the best known strategy. Returns `404` if no strategies are known yet.

```json
{ "status": "started", "strategy": "auto-4 [fake/badseq/file]" }
```

### POST /api/stop

```json
{ "status": "stopped" }
```

### POST /api/watchdog

Starts watchdog in the background. Returns immediately.

```json
{ "status": "started", "message": "watchdog running in background" }
```

### DELETE /api/watchdog

```json
{ "status": "stopped" }
```

### POST /api/find — SSE

Starts strategy search. Returns an SSE stream until a result is found or search is exhausted.

```
data: {"type":"progress","current":3,"total":137,"strategy":"auto-3 [fake/ts/file]","score":0.33}

data: {"type":"success","strategy":{...},"score":1.0,"vector":{...}}

data: {"type":"error","error":"no working strategy found"}
```

Returns `409 Conflict` if another operation is already running.

### POST /api/update — SSE

Updates IP/host lists from GitHub. Returns an SSE stream.

```
data: {"type":"progress","current":1,"total":5,"filename":"ipset-all.txt"}

data: {"type":"success","status":"updated","message":"lists updated successfully"}

data: {"type":"error","error":"download ipset-all.txt: ..."}
```

Returns `409 Conflict` if another operation is already running.

### POST /api/update-self — SSE

Checks GitHub Releases for a newer version and applies it. Returns an SSE stream.

Event types in order: `checking` → `found` → `downloading` → `verifying` → `applying` → `success` (or `up_to_date` / `error`).

```
data: {"type":"checking","message":"Checking for updates..."}

data: {"type":"found","message":"New version available: v1.2.0 → v1.2.1"}

data: {"type":"downloading","message":"Downloading zapret-core-v1.2.1-windows-amd64.zip..."}

data: {"type":"verifying","message":"Verifying SHA256..."}

data: {"type":"applying","message":"Applying update..."}

data: {"type":"success","status":"updated","message":"Updated successfully. Restarting..."}
```

On `success` the process restarts automatically. On no update needed:

```
data: {"type":"up_to_date","status":"up_to_date","message":"Already up to date (v1.2.1)"}
```

Returns `409 Conflict` if another operation is already running.

### GET /api/events — SSE (persistent)

Persistent SSE stream. Sends the current status immediately on connect, then pushes events whenever state changes.

**Initial event on connect:**
```
data: {"type":"status","data":{"running":true,"watchdog":false,"strategy":"auto-4 [fake/badseq/file]"}}
```

**Subsequent events** (emitted on start/stop/watchdog state changes):
```
data: {"type":"status","data":{"running":true,"watchdog":false,"strategy":"auto-4 [fake/badseq/file]"}}
```

Keep-alive comments are sent every 15 seconds to prevent proxy timeouts:
```
: ping
```

The connection stays open until the client disconnects. Multiple clients are supported simultaneously.

</details>

---

## Configuration

`data/config.json` is created automatically on first run. You can edit any parameter:

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
| `score_threshold` | `0.6` | Minimum score to accept a strategy (0–1) |
| `fail_threshold` | `3` | Consecutive failures before watchdog triggers recovery |
| `check_interval` | `60` | How often watchdog checks connectivity (seconds) |
| `init_delay` | `5` | How long to wait after starting winws before testing (seconds) |
| `test_timeout` | `8` | Timeout for a single HTTP probe (seconds) |
| `test_runs` | `2` | How many times to repeat each test for reliability |

---

## Knowledge Base

`data/knowledge.json` is the program's memory — strategies that worked for each ISP by ASN. On the next run they're tested first, before any full search.

Delete the file to start from scratch. Duplicates are updated, not appended, so the file doesn't grow indefinitely.

---

## Conflict Detection

Before searching, zapret-core checks for software known to interfere with WinDivert:

- GoodbyeDPI
- AdGuardSvc
- discordfix_zapret
- winws1, winws2
- Killer NIC / Intel Connectivity Network Service
- Check Point (TracSrvWrapper, EPWD)
- SmartByte

If anything is found the search stops with a message. Disable the conflicting software and try again.

---

## Logs

Written to both the console and `data/zapret.log`. Levels: `[INFO]`, `[WARN]`, `[ERROR]`.

---

## Troubleshooting

<details>
<summary>Common issues</summary>

**"No known strategies. Run --find"** — the knowledge base is empty or has no entries for your ISP. Run `--find`.

**"No working strategy found"** — nothing passed the score threshold. Check your connection or increase `test_timeout` in config.json.

**"Resolve conflicts and try again"** — conflicting software is running. Stop it and retry.

**"failed to start winws"** — `assets/winws.exe` not found or missing administrator rights.

**409 in API** — another operation is in progress. Wait or stop it via `POST /api/stop`.

</details>

---

## Credits

- [bol-van](https://github.com/bol-van/zapret) — for zapret itself, winws, WinDivert, and the fake packet binaries
- [Flowseal](https://github.com/flowseal/zapret-discord-youtube) — for strategy presets and parameter research that shaped the search space here

---

## License

[MIT](LICENSE) © elev1e1nSure