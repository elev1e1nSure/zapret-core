# zapret-core

[English](README.md) | [Русский](README.ru.md)

![Go](https://img.shields.io/badge/Go-1.26.3-blue)
![Platform](https://img.shields.io/badge/platform-Windows%20only-lightgrey)
![License](https://img.shields.io/badge/license-MIT-green)
![Release](https://img.shields.io/github/v/release/elev1e1nSure/zapret-core)
![Downloads](https://img.shields.io/github/downloads/elev1e1nSure/zapret-core/total)

> **⚠️ Windows only** — requires WinDivert kernel driver. Linux and macOS are not supported and will not be.

DPI bypass core for YouTube and Discord on Windows. Finds a working strategy for your ISP automatically, remembers it, and recovers when your ISP updates their blocking — no manual configuration needed.

Designed to be used as a backend for GUI wrappers via its HTTP API.

> 📖 **[Full documentation → Wiki](../../wiki)**

Built on top of [zapret](https://github.com/bol-van/zapret) by bol-van and inspired by [zapret-discord-youtube](https://github.com/flowseal/zapret-discord-youtube) by Flowseal.

---

## How it works

1. Detects your ISP via ASN lookup
2. Tests up to 137 DPI bypass strategy combinations
3. Saves what works to `data/knowledge.json`
4. On next run — starts with the best known strategy immediately
5. Watchdog detects when ISP updates blocking and finds a new strategy automatically

---

## Requirements

- Windows 7 or later (x64)
- Administrator rights — WinDivert installs a kernel driver
- Internet connection

---

## Installation

> **[Download latest release](https://github.com/elev1e1nSure/zapret-core/releases/latest)**

Extract anywhere. Run as Administrator.

**Build from source** (requires Go 1.21+ and Windows):

```bat
git clone https://github.com/elev1e1nSure/zapret-core.git
cd zapret-core

:: Quick dev build
go build -ldflags="-s -w" -o zapret-core.exe ./internal

:: Release build (strips debug info + packages dist zip)
build.bat
```

---

## Quick start

```bash
# Find a working strategy for your ISP
zapret-core.exe --find

# Run with the best known strategy
zapret-core.exe

# Run as HTTP API server (for GUI wrappers)
zapret-core.exe --server

# Monitor with auto-recovery
zapret-core.exe --watch
```

> **`--watch` and `--server` cannot run simultaneously** — both manage winws independently and will conflict. Use `POST /api/watchdog` instead when running in server mode.

---

## Credits

- [bol-van](https://github.com/bol-van/zapret) — zapret, winws, WinDivert, fake packet binaries
- [Flowseal](https://github.com/flowseal/zapret-discord-youtube) — strategy presets and parameter research

---

## License

[MIT](LICENSE) © elev1e1nSure