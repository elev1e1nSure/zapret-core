# zapret-core

[English](README.md) | [Русский](README.ru.md)

![Go](https://img.shields.io/badge/Go-1.21-blue)
![Platform](https://img.shields.io/badge/platform-Windows-lightgrey)
![License](https://img.shields.io/badge/license-MIT-green)
![Release](https://img.shields.io/github/v/release/elev1e1nSure/zapret-core)
![Downloads](https://img.shields.io/github/downloads/elev1e1nSure/zapret-core/total)

Тулза для обхода DPI на Windows — для YouTube и Discord. Сама перебирает стратегии, находит рабочую под ваш провайдер и запоминает. Если провайдер обновит блокировку — сама же найдёт новую.

Сделано на базе [zapret](https://github.com/bol-van/zapret) (bol-van) и [zapret-discord-youtube](https://github.com/flowseal/zapret-discord-youtube) (Flowseal).

---

## Зачем это нужно

Обычно с запретом дают список из 80+ стратегий и говорят "пробуй". Здесь всё автоматически: программа сама тестирует комбинации параметров, находит что работает именно у вашего провайдера, и при следующем запуске сразу стартует с рабочей. Если что-то перестало работать — watchdog заметит и найдёт замену без вашего участия.

---

## Что нужно для запуска

- Windows 7+
- Права администратора — WinDivert ставит драйвер ядра, без админа не запустится
- Интернет — для определения провайдера и тестов

---

## Установка

> **[Скачать последний релиз](https://github.com/elev1e1nSure/zapret-core/releases/latest)**

Распакуйте архив, структура должна быть такой:

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

Папка `data/` появится сама при первом запуске.

---

### Собрать из исходников

<details>
<summary>Инструкции</summary>

Нужен Go 1.21+ и Windows.

```bash
git clone https://github.com/elev1e1nSure/zapret-core.git
cd zapret-core
go build -o zapret-core.exe .
```

Или через скрипт — он соберёт всё в папку `dist/`:

```bash
build.bat
```

</details>

---

## Использование

### Просто запустить

```
zapret-core.exe
```

Определяет провайдера, берёт лучшую стратегию из базы и работает до Ctrl+C. Если база пустая — скажет запустить `--find`.

---

### Найти рабочую стратегию

```
zapret-core.exe --find
```

Тестирует до 137 комбинаций, останавливается на первой рабочей:

```
[1/137] Testing: auto-1 [fake/ts/file]
  score=0.33  YouTube:FAIL  Discord:FAIL  Google:OK

[4/137] Testing: auto-4 [fake/badseq/file]
  score=1.00  YouTube:OK  Discord:OK  Google:OK

[+] Working strategy found: auto-4 [fake/badseq/file]
```

Результат сохраняется и используется при следующих запусках.

В лучшем случае занимает пару минут, в худшем — до двух часов. Но большинство находит рабочую стратегию в первых 10–20 попытках.

---

### Мониторинг с авто-восстановлением

```
zapret-core.exe --watch
```

Каждые 60 секунд проверяет YouTube и Discord. Три неудачи подряд — автоматически ищет новую стратегию и переключается. Остановить через Ctrl+C, всё завершится корректно.

---

### Статус

```
zapret-core.exe --status
```

Показывает, запущен ли winws и какая стратегия используется. Завершается сразу.

---

### Остановить

```
zapret-core.exe --stop
```

Останавливает winws. Завершается сразу.

---

### Обновить списки

```
zapret-core.exe --update
```

Скачивает актуальные списки из репозитория Flowseal:

```
[1/5] Обновление ipset-all.txt...
[2/5] Обновление ipset-exclude.txt...
[3/5] Обновление list-exclude.txt...
[4/5] Обновление list-general.txt...
[5/5] Обновление list-google.txt...
Списки успешно обновлены.
```

Если что-то не скачалось — старые файлы остаются нетронутыми.

---

### HTTP API

```
zapret-core.exe --server
```

Запускает сервер на `127.0.0.1:7432` для интеграции с внешними приложениями. Остановить через Ctrl+C.

---

## Справка по API

<details>
<summary>Все эндпоинты доступны только локально (127.0.0.1:7432)</summary>

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

Запускает лучшую известную стратегию. Если стратегий ещё нет — вернёт `404`.

```json
{ "status": "started", "strategy": "auto-4 [fake/badseq/file]" }
```

### POST /api/stop

```json
{ "status": "stopped" }
```

### POST /api/find

Запускает поиск, отдаёт SSE-поток:

```
event: progress
data: {"current": 3, "total": 137, "strategy": "auto-3 [fake/ts/file]", "score": 0.33}

event: success
data: {"strategy": "auto-4 [fake/badseq/file]", "score": 1.0, "vector": {...}}
```

Если уже что-то выполняется — `409 Conflict`.

### POST /api/update

Обновляет списки, тоже SSE:

```
event: progress
data: {"current": 1, "total": 5, "filename": "ipset-all.txt"}

event: success
data: {"status": "updated", "message": "lists updated successfully"}
```

`409 Conflict` если занято.

### POST /api/watchdog

Запускает watchdog в фоне, отвечает сразу:

```json
{ "status": "started", "message": "watchdog running in background" }
```

### DELETE /api/watchdog

```json
{ "status": "stopped" }
```

</details>

---

## Настройки

`data/config.json` создаётся автоматически, менять можно любой параметр:

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

| Параметр | По умолчанию | Что делает |
|---|---|---|
| `score_threshold` | `0.6` | Минимальный балл чтобы стратегия считалась рабочей (0–1) |
| `fail_threshold` | `3` | Сколько неудач подряд до срабатывания watchdog |
| `check_interval` | `60` | Интервал проверок watchdog в секундах |
| `init_delay` | `5` | Пауза после запуска winws перед первым тестом (секунды) |
| `test_timeout` | `8` | Таймаут одного HTTP-запроса (секунды) |
| `test_runs` | `2` | Сколько раз повторять каждый тест |

---

## База знаний

`data/knowledge.json` — это память программы. Здесь хранятся стратегии, которые сработали для каждого провайдера по ASN. При следующем запуске они тестируются первыми, до полного перебора.

Удалите файл — и программа начнёт искать с нуля. Дубликаты не накапливаются, база не пухнет.

---

## Конфликты

Перед поиском программа проверяет, не запущено ли что-то несовместимое с WinDivert:

- GoodbyeDPI
- AdGuardSvc
- discordfix_zapret
- winws1, winws2
- Killer NIC / Intel Connectivity Network Serviceя  
- Check Point (TracSrvWrapper, EPWD)
- SmartByte

Если найдёт — остановится с сообщением. Отключите конфликтующую программу и попробуйте снова.

---

## Логи

Пишутся в консоль и в `data/zapret.log`. Уровни: `[INFO]`, `[WARN]`, `[ERROR]`.

---

## Если что-то не работает

<details>
<summary>Частые проблемы</summary>

**"No known strategies. Run --find"** — база пустая или нет записей для вашего провайдера. Запустите `--find`.

**"No working strategy found"** — ни одна стратегия не набрала нужный балл. Проверьте интернет или увеличьте `test_timeout` в конфиге.

**"Resolve conflicts and try again"** — запущена несовместимая программа, остановите её.

**"failed to start winws"** — нет `assets/winws.exe` или нет прав администратора.

**409 в API** — уже выполняется другая операция, подождите или остановите через `POST /api/stop`.

</details>

---

## Спасибо

- [bol-van](https://github.com/bol-van/zapret) — за сам zapret, winws, WinDivert и бинарники
- [Flowseal](https://github.com/flowseal/zapret-discord-youtube) — за пресеты стратегий и исследование параметров

---

## Лицензия

[MIT](LICENSE) © elev1e1nSure