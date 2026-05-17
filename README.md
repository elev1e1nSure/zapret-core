# zapret-core

Automatic DPI bypass engine for YouTube and Discord on Windows. Finds a working strategy for your ISP on its own, remembers it, and recovers when your ISP updates their blocking — no manual configuration needed.

Built on top of [zapret](https://github.com/bol-van/zapret) by bol-van and inspired by [zapret-discord-youtube](https://github.com/flowseal/zapret-discord-youtube) by Flowseal.

---

## How it works

Most DPI bypass tools give you a list of 80+ strategies and say "try them one by one". zapret-core does that for you: it generates parameter combinations, tests what actually works for your ISP, and remembers the result. Next time it starts with the best known strategy immediately.

If your ISP updates their blocking — watchdog detects it and finds a new working strategy automatically.

---

## Requirements

- Windows 7 or newer
- Administrator rights (required — WinDivert installs a kernel driver)
- Internet connection for ISP detection and testing

---

## Installation

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

> Always run as administrator — WinDivert won't load otherwise.

---

## Usage

### Start with best known strategy

```
zapret-core.exe
```

Detects your ISP, loads the best strategy from the knowledge base, and starts. Runs until Ctrl+C.

If the knowledge base is empty — prompts you to run `--find` first.

---

### Find a working strategy

```
zapret-core.exe --find
```

Tests up to 137 parameter combinations and stops at the first one that works. Progress is shown in real time:

```
[1/137] Testing: auto-1 [fake/ts/file]
  score=0.33  YouTube:FAIL  Discord:FAIL  Google:OK

[4/137] Testing: auto-4 [fake/badseq/file]
  score=1.00  YouTube:OK  Discord:OK  Google:OK

[+] Working strategy found: auto-4 [fake/badseq/file]
```

The result is saved to the knowledge base and used on subsequent runs.

**How long does it take:** best case — a few minutes. Worst case — up to 2 hours if nothing works immediately. In practice most users find a working strategy within the first 10–20 attempts.

---

### Monitor with auto-recovery

```
zapret-core.exe --watch
```

Starts background monitoring. Checks YouTube and Discord every 60 seconds. If three checks in a row fail — automatically finds a new strategy and switches to it.

Stop with Ctrl+C. Both watchdog and winws will shut down cleanly.

---

### Status

```
zapret-core.exe --status
```

Shows whether winws is running and the best known strategy for your ISP. Exits immediately.

---

### Stop

```
zapret-core.exe --stop
```

Stops winws. Exits immediately.

---

### HTTP API

```
zapret-core.exe --server
```

Starts an HTTP server on `127.0.0.1:7432`. Intended for integration with external applications such as a Tauri UI. Stops on Ctrl+C.

---

## HTTP API Reference

All endpoints are local-only (`127.0.0.1:7432`).

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

Start the best known strategy. Returns `404` if no strategies are known yet.

```json
{ "status": "started", "strategy": "auto-4 [fake/badseq/file]" }
```

### POST /api/stop

Stops winws.

```json
{ "status": "stopped" }
```

### POST /api/find

Start strategy search. Returns an SSE stream.

```
event: progress
data: {"current": 3, "total": 137, "strategy": "auto-3 [fake/ts/file]", "score": 0.33}

event: success
data: {"strategy": "auto-4 [fake/badseq/file]", "score": 1.0, "vector": {...}}
```

Returns `409 Conflict` if another operation is already running.

### POST /api/watchdog

Start watchdog in background. Returns immediately.

```json
{ "status": "started", "message": "watchdog running in background" }
```

### DELETE /api/watchdog

```json
{ "status": "stopped" }
```

---

## Configuration

`data/config.json` is created automatically on first run:

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
| `score_threshold` | `0.6` | Minimum test score to accept a strategy (0–1) |
| `fail_threshold` | `3` | Consecutive failures before watchdog triggers recovery |
| `check_interval` | `60` | How often watchdog checks connectivity (seconds) |
| `init_delay` | `5` | How long to wait after starting winws before testing (seconds) |
| `test_timeout` | `8` | Timeout for a single HTTP probe (seconds) |
| `test_runs` | `2` | How many times to repeat each test for reliability |

---

## Knowledge Base

`data/knowledge.json` stores strategies that worked for each ISP (by ASN). On the next run they are tested first — before any full search begins.

Deleting the file causes a full search from scratch. The file does not grow indefinitely — duplicate entries are updated, not added.

---

## Conflict Detection

Before searching, zapret-core checks for software known to interfere with WinDivert:

- GoodbyeDPI
- AdGuardSvc
- discordfix_zapret
- winws1, winws2
- Killer NIC
- Intel Connectivity Network Service
- Check Point (TracSrvWrapper, EPWD)
- SmartByte

If a conflict is found the search stops with a message. Disable the conflicting software and try again.

---

## Logs

Logs are written to both the console and `data/zapret.log`. Levels: `[INFO]`, `[WARN]`, `[ERROR]`.

---

## Troubleshooting

**"No known strategies. Run --find"**
The knowledge base is empty or has no entries for your ISP. Run `zapret-core.exe --find`.

**"No working strategy found"**
No combination passed the score threshold. Check your internet connection or increase `test_timeout` in config.json.

**"Resolve conflicts and try again"**
Conflicting software is running. Stop it and retry.

**"failed to start winws"**
`assets/winws.exe` not found or missing administrator rights.

**409 in API**
Another operation is in progress. Wait for it to finish or stop it via `POST /api/stop`.

---

## Tauri Integration

zapret-core is designed to run as a sidecar process. Start with `--server` and call the API via reqwest:

```rust
use reqwest::Client;

let client = Client::new();

// Status
let status = client
    .get("http://127.0.0.1:7432/api/status")
    .send().await?
    .json::<serde_json::Value>().await?;

// Start strategy
client.post("http://127.0.0.1:7432/api/start").send().await?;

// Find strategy with SSE streaming
let mut stream = client
    .post("http://127.0.0.1:7432/api/find")
    .send().await?
    .bytes_stream();
```

---

## Credits

- [zapret](https://github.com/bol-van/zapret) by bol-van — the core DPI bypass engine (winws, WinDivert integration, fake packet binaries)
- [zapret-discord-youtube](https://github.com/flowseal/zapret-discord-youtube) by Flowseal — strategy presets and parameter research that informed the search space in this project

---

## License

MIT