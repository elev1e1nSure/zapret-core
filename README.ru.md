# zapret-core

[English](README.md) | [Русский](README.ru.md)

![Go](https://img.shields.io/badge/Go-1.21-blue)
![Platform](https://img.shields.io/badge/platform-Windows-lightgrey)
![License](https://img.shields.io/badge/license-MIT-green)
![Release](https://img.shields.io/github/v/release/elev1e1nSure/zapret-core)
![Downloads](https://img.shields.io/github/downloads/elev1e1nSure/zapret-core/total)

Инструмент для обхода DPI на Windows — для YouTube и Discord. Сама перебирает стратегии, находит рабочую под твой провайдер и запоминает. Если провайдер обновит блокировку — сама же найдёт новую.

Сделано на базе [zapret](https://github.com/bol-van/zapret) (bol-van) и [zapret-discord-youtube](https://github.com/flowseal/zapret-discord-youtube) (Flowseal).

---

## Зачем это нужно

Обычно с обходом блокировок дают список из 80+ стратегий и говорят "пробуй". Здесь всё автоматически: программа сама тестирует комбинации параметров, находит что работает именно у твоего провайдера, и при следующем запуске сразу стартует с рабочей. Если что-то перестало работать — watchdog заметит и найдёт замену без твоего участия.

---

## Как это работает

1. **Определение провайдера** — запрашивает `ipinfo.io` и получает твой ASN (например `AS12389 Rostelecom`). По ASN ищутся сохранённые стратегии.
2. **Поиск стратегии** — тестируется до 137 комбинаций параметров. Каждая запускает winws с определёнными флагами обхода DPI, ждёт инициализации, потом проверяет YouTube, Discord и Google по HTTP. Оценка от 0 до 1 — сколько целей ответило успешно.
3. **База знаний** — найденная стратегия сохраняется в `data/knowledge.json` с ключом по ASN. При следующем запуске она проверяется первой — полный поиск запускается только если она перестала работать.
4. **Watchdog** — фоновый цикл проверяет YouTube и Discord с заданным интервалом. Если несколько проверок подряд упали — оптимизатор запускается снова и переключается на новую рабочую стратегию.
5. **Самообновление** — сравнивает локальную версию с последним релизом на GitHub, скачивает zip, проверяет SHA256, атомарно заменяет бинарь (переименование через `.old`) и завершается. Следующий запуск уже использует новую версию.

---

## Что нужно для запуска

- Windows 7 и новее (x64)
- Права администратора — WinDivert устанавливает драйвер ядра
- Интернет — для определения провайдера, тестирования и обновлений

---

## Установка

> **[Скачать последний релиз](https://github.com/elev1e1nSure/zapret-core/releases/latest)**

Распакуй архив в любое место. Нужная структура:

```
zapret-core.exe
assets/
    winws.exe
    WinDivert.dll
    WinDivert64.sys
    cygwin1.dll
    fake/
        *.bin          ← пейлоады фейковых пакетов для некоторых стратегий
lists/
    list-general.txt   ← домены для обхода
    list-google.txt
    ipset-all.txt      ← IP-диапазоны
    ipset-exclude.txt
    list-exclude.txt
data/                  ← создаётся автоматически при первом запуске
    config.json
    knowledge.json
    zapret.log
```

> **Запускай от администратора.** Правая кнопка → "Запуск от имени администратора", либо из терминала с повышенными правами.

### Проверить загрузку

Каждый релиз включает `checksums.txt` с SHA256. Чтобы проверить:

```powershell
Get-FileHash zapret-core-v1.2.11-windows-amd64.zip -Algorithm SHA256
```

Сравни с хэшем в `checksums.txt`. Совпадают — файл подлинный.

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

Важно: локальная сборка — консольная (вывод в терминал). Релизные сборки используют `-H windowsgui`, поэтому консольного окна не появляется при запуске без терминала — это намеренно, для использования с UI-оберткой.

Или через скрипт — он соберёт всё в папку `dist/`:

```bash
build.bat
```

</details>

---

## Использование

### Быстрый старт

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

Результат сохраняется в `data/knowledge.json` и используется при каждом следующем запуске.

Обычно занимает несколько минут. В худшем случае — до двух часов. Большинство пользователей находит рабочую стратегию в первых 10–20 попытках.

> Запускай `--find` снова если провайдер обновил блокировку и watchdog не справляется сам.

---

### Мониторинг с авто-восстановлением

```
zapret-core.exe --watch
```

Работает в фоне (без консольного окна в релизных сборках). Проверяет YouTube и Discord каждые 60 секунд. Три неудачи подряд — запускает оптимизатор, находит новую стратегию и переключается без участия пользователя. Остановить через Ctrl+C — winws завершится корректно.

---

### Статус

```
zapret-core.exe --status
```

Показывает, запущен ли winws и какая стратегия активна. Завершается сразу.

---

### Остановить

```
zapret-core.exe --stop
```

Останавливает winws. Завершается сразу.

---

### Сбросить стратегии

```
zapret-core.exe --reset
```

Удаляет все сохранённые стратегии для текущего ASN. Используй когда хочешь принудительно запустить чистый поиск — например после смены провайдера или VPN.

---

### Экспорт / Импорт стратегий

```
zapret-core.exe --export strategies.json
zapret-core.exe --import strategies.json
```

Экспорт сохраняет все стратегии для твоего ASN в JSON-файл. Импорт объединяет их с базой знаний. Удобно для переноса конфига между машинами или восстановления после переустановки.

---

### Обновить списки

```
zapret-core.exe --updatelists
```

Скачивает актуальные IP и доменные списки из репозитория Flowseal:

```
[1/5] Updating ipset-all.txt...
[2/5] Updating ipset-exclude.txt...
[3/5] Updating list-exclude.txt...
[4/5] Updating list-general.txt...
[5/5] Updating list-google.txt...
Lists updated successfully.
```

Используется атомарное обновление — каждый файл сначала скачивается во временный `.tmp`, потом переименовывается. Если скачивание упало — старые файлы остаются нетронутыми.

---

### Обновить программу

```
zapret-core.exe --update
```

Проверяет GitHub Releases на наличие новой версии. Если нашлась:
1. Скачивает `zapret-core-vX.Y.Z-windows-amd64.zip`
2. Проверяет SHA256 по `checksums.txt`
3. Извлекает `zapret-core.exe` из архива
4. Атомарно заменяет текущий бинарь (переименование через `.old`)
5. Завершается с сообщением перезапустить

Старый бинарь остаётся как `zapret-core.exe.old` и удаляется при следующем запуске.

---

### HTTP API сервер

```
zapret-core.exe --server
```

Запускает HTTP-сервер на `127.0.0.1:7432` для интеграции с UI или внешними инструментами. Работает в daemon-режиме (без консольного окна в релизных сборках). Остановить через Ctrl+C.

---

## Daemon-режим

`--server` и `--watch` — **daemon-режимы**. В релизных сборках (скомпилированных с `-H windowsgui`) у процесса вообще нет консольного окна — он работает тихо в фоне. Это намеренно: эти режимы предназначены для управления через UI-обёртку или задачу в планировщике, не для прямого взаимодействия.

В локально собранном бинарнике (без `-H windowsgui`) консольное окно появляется, но сразу скрывается через `ShowWindow(SW_HIDE)` до вывода чего-либо.

---

## Справка по API

<details>
<summary>Все эндпоинты доступны только локально (127.0.0.1:7432)</summary>

### Обработка конфликтов

Если уже выполняется длительная операция (`/api/find`, `/api/update`, `/api/update-self`, `/api/start`, `/api/stop`) — любой новый запрос вернёт `409 Conflict`:

```json
{ "error": "operation in progress: find" }
```

Дождись завершения или отмени через `POST /api/stop`.

---

### GET /api/version

```json
{ "version": "v1.2.11" }
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

### GET /api/knowledge

Возвращает все стратегии для текущего ASN, отсортированные по убыванию оценки.

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

Запускает лучшую известную стратегию для текущего ASN. Вернёт `404` если база пустая.

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

Запускает watchdog в фоне, отвечает сразу:

```json
{ "status": "started", "message": "watchdog running in background" }
```

---

### DELETE /api/watchdog

Останавливает watchdog и winws.

```json
{ "status": "stopped" }
```

---

### POST /api/find — SSE

Запускает поиск стратегий. Стримит прогресс до получения результата или исчерпания вариантов.

```
event: progress
data: {"current":3,"total":137,"strategy":"auto-3 [fake/ts/file]","score":0.33}

event: success
data: {"strategy":{...},"score":1.0,"vector":{...}}

event: error
data: {"error":"no working strategy found"}
```

---

### POST /api/update — SSE

Скачивает обновлённые списки с GitHub. Стримит прогресс.

```
event: progress
data: {"current":1,"total":5,"filename":"ipset-all.txt"}

event: success
data: {"status":"updated","message":"lists updated successfully"}

event: error
data: {"error":"download ipset-all.txt: HTTP 404"}
```

---

### POST /api/update-self — SSE

Проверяет наличие нового релиза и применяет обновление. Стримит прогресс через этапы:
`checking` → `found` → `downloading` → `verifying` → `applying` → `success` или `up_to_date` или `error`.

```
event: progress
data: {"type":"checking","message":"Checking for updates..."}

event: progress
data: {"type":"found","message":"New version available: v1.2.10 → v1.2.11"}

event: progress
data: {"type":"downloading","message":"Downloading zapret-core-v1.2.11-windows-amd64.zip..."}

event: progress
data: {"type":"verifying","message":"Verifying SHA256..."}

event: progress
data: {"type":"applying","message":"Applying update..."}

event: success
data: {"status":"updated","message":"Update installed (v1.2.10 → v1.2.11). Please restart the server."}
```

После `success` серверный процесс завершается (`os.Exit(0)`). Перезапусти его — запустится уже новая версия.

Если обновлений нет:

```
event: success
data: {"status":"up_to_date","message":"Already up to date (v1.2.11)"}
```

---

### GET /api/events — SSE (постоянное соединение)

Долгоживущий SSE-поток. При подключении сразу отправляет текущее состояние, затем пушит событие при каждом изменении (старт, стоп, переключение watchdog).

```
data: {"type":"status","data":{"running":true,"watchdog":false,"strategy":"auto-4 [fake/badseq/file]"}}
```

Keep-alive каждые 15 секунд:

```
: ping
```

Поддерживается несколько клиентов одновременно. Соединение остаётся открытым до отключения клиента.

</details>

---

## Настройки

`data/config.json` создаётся с дефолтными значениями при первом запуске. Все поля опциональны — отсутствующие используют дефолт.

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
| `score_threshold` | `0.6` | Минимальная оценка (0–1) чтобы принять стратегию. Ниже = мягче, выше = строже. |
| `fail_threshold` | `3` | Сколько неудач подряд до срабатывания watchdog. |
| `check_interval` | `60` | Интервал проверок watchdog в секундах. |
| `init_delay` | `5` | Пауза после запуска winws перед первым тестом. Увеличь если получаешь ложные негативы на медленных машинах. |
| `test_timeout` | `8` | Таймаут одного HTTP-запроса в секундах. Увеличь при медленном или высоколатентном соединении. |
| `test_runs` | `2` | Сколько раз повторять каждый тест. Выше = медленнее поиск, но меньше ложных срабатываний. |

---

## База знаний

`data/knowledge.json` хранит стратегии, которые сработали для каждого провайдера, с ключом по ASN. При запуске лучшая стратегия для твоего ASN загружается и проверяется первой — полный поиск запускается только если она не работает или записей нет.

- **Удали файл** — программа начнёт искать с нуля.
- **Экспорт/импорт** — для переноса стратегий на другую машину.
- Файл не растёт бесконечно — дубликаты для одного ASN+вектора обновляются на месте.

---

## Файловая структура

| Путь | Описание |
|---|---|
| `zapret-core.exe` | Основной бинарь |
| `assets/winws.exe` | Движок обхода DPI (из zapret) |
| `assets/WinDivert.dll` / `.sys` | Драйвер перехвата пакетов на уровне ядра |
| `assets/fake/*.bin` | Пейлоады фейковых пакетов для некоторых стратегий |
| `lists/*.txt` | Списки доменов и IP-диапазонов для обхода |
| `data/config.json` | Конфигурация (создаётся автоматически) |
| `data/knowledge.json` | Память стратегий (создаётся автоматически) |
| `data/zapret.log` | Лог-файл (создаётся автоматически) |

---

## Обнаружение конфликтов

Перед поиском программа проверяет, не запущено ли что-то несовместимое с WinDivert на уровне ядра:

- GoodbyeDPI
- AdGuardSvc
- discordfix_zapret
- winws1, winws2 (другие экземпляры)
- Killer NIC / Intel Connectivity Network Service
- Check Point (TracSrvWrapper, EPWD)
- SmartByte

Если что-то найдено — поиск останавливается со списком виновников. Отключи конфликтующую программу и повтори.

---

## Логи

Пишутся одновременно в консоль и в `data/zapret.log`.

| Уровень | Значение |
|---|---|
| `[INFO]` | Штатная работа |
| `[WARN]` | Некритичная проблема (неудачная проверка, потерянное событие) |
| `[ERROR]` | Операция завершилась ошибкой |
| `[OK]` | Подтверждение успеха |

Лог-файл не ротируется — удали вручную если вырастет слишком большим.

---

## Если что-то не работает

<details>
<summary>Частые проблемы</summary>

**"No known strategies. Run --find"**  
База пустая или нет записей для твоего провайдера. Запусти `--find`.

**"No working strategy found"**  
Ни одна комбинация не прошла порог оценки. Возможные причины:
- Медленное соединение → увеличь `test_timeout`
- Порог слишком строгий → снизь `score_threshold` до `0.4`
- Провайдер использует метод блокировки, не покрытый текущим набором стратегий

**"Resolve conflicts and try again"**  
Запущен конфликтующий процесс. Смотри список выше, останови и повтори.

**"failed to start winws"**  
Либо `assets/winws.exe` отсутствует (неполная распаковка), либо программа запущена не от администратора.

**Watchdog срабатывает постоянно**  
Активная стратегия нестабильна. Запусти `--find` снова для более надёжной стратегии, или увеличь `test_runs` в конфиге для более строгого отбора.

**409 Conflict в API**  
Уже выполняется другая операция. Подожди или отправь `POST /api/stop`.

**После `--update` версия не изменилась**  
Бинарь заменён, но ты всё ещё используешь старый процесс. Перезапусти программу. В режиме `--server` сервер автоматически завершается после успешного обновления — просто перезапусти его.

**Двойной клик на exe — ничего не происходит**  
Релизные сборки не имеют консольного окна (`-H windowsgui`). Процесс работает в фоне. Используй терминал (PowerShell / cmd) чтобы видеть вывод, или режим `--server` с подключением через API.

</details>

---

## Спасибо

- [bol-van](https://github.com/bol-van/zapret) — zapret, winws, WinDivert и бинарники фейковых пакетов
- [Flowseal](https://github.com/flowseal/zapret-discord-youtube) — пресеты стратегий и исследование параметров

---

## Лицензия

[MIT](LICENSE) © elev1e1nSure