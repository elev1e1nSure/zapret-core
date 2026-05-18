# zapret-core

[English](README.md) | [Русский](README.ru.md)

![Go](https://img.shields.io/badge/Go-1.26.3-blue)
![Platform](https://img.shields.io/badge/platform-Windows%20only-lightgrey)
![License](https://img.shields.io/badge/license-MIT-green)
![Release](https://img.shields.io/github/v/release/elev1e1nSure/zapret-core)
![Downloads](https://img.shields.io/github/downloads/elev1e1nSure/zapret-core/total)

> **⚠️ Только Windows** — требует драйвер ядра WinDivert. Linux и macOS не поддерживаются и не будут поддерживаться.

Ядро для обхода DPI на Windows — YouTube и Discord. Само находит рабочую стратегию для твоего провайдера, запоминает, и восстанавливается если провайдер обновит блокировку — никакой ручной настройки.

Предназначено для использования как бэкенд GUI-обёрток через HTTP API.

> 📖 **[Полная документация → Wiki](../../wiki)**

Сделано на базе [zapret](https://github.com/bol-van/zapret) (bol-van) и [zapret-discord-youtube](https://github.com/flowseal/zapret-discord-youtube) (Flowseal).

---

## Как работает

1. Определяет провайдера по ASN
2. Тестирует до 137 комбинаций стратегий обхода DPI
3. Сохраняет рабочую в `data/knowledge.json`
4. При следующем запуске — сразу стартует с лучшей известной стратегией
5. Watchdog замечает когда провайдер обновил блокировку и автоматически находит новую

---

## Требования

- Windows 7 и новее (x64)
- Права администратора — WinDivert устанавливает драйвер ядра
- Интернет

---

## Установка

> **[Скачать последний релиз](https://github.com/elev1e1nSure/zapret-core/releases/latest)**

Распакуй куда угодно. Запускай от администратора.

**Собрать из исходников** (нужен Go 1.26.3 и Windows):

```bash
git clone https://github.com/elev1e1nSure/zapret-core.git
cd zapret-core
go build -o zapret-core.exe .
```

---

## Быстрый старт

```bash
# Найти рабочую стратегию для твоего провайдера
zapret-core.exe --find

# Запустить с лучшей известной стратегией
zapret-core.exe

# Запустить как HTTP API сервер (для GUI-обёрток)
zapret-core.exe --server

# Мониторинг с авто-восстановлением
zapret-core.exe --watch
```

> **`--watch` и `--server` нельзя запускать одновременно** — оба управляют winws независимо и конфликтуют. В режиме сервера используй `POST /api/watchdog` вместо `--watch`.

---

## Спасибо

- [bol-van](https://github.com/bol-van/zapret) — zapret, winws, WinDivert, бинарники фейковых пакетов
- [Flowseal](https://github.com/flowseal/zapret-discord-youtube) — пресеты стратегий и исследование параметров

---

## Лицензия

[MIT](LICENSE) © elev1e1nSure