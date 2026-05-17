# zapret-core

[English](README.md) | [Русский](README.ru.md)

Автоматический движок обхода DPI для YouTube и Discord на Windows. Сам находит рабочую стратегию для вашего провайдера, запоминает её и восстанавливается, когда провайдер обновляет блокировку — ручная настройка не требуется.

Создан на основе [zapret](https://github.com/bol-van/zapret) от bol-van и вдохновлён [zapret-discord-youtube](https://github.com/flowseal/zapret-discord-youtube) от Flowseal.

---

## Как это работает

Большинство инструментов обхода DPI дают вам список из 80+ стратегий и говорят "пробуйте их одну за другой". zapret-core делает это за вас: генерирует комбинации параметров, тестирует что реально работает для вашего провайдера, и запоминает результат. При следующем запуске сразу начинается с лучшей известной стратегии.

Если ваш провайдер обновляет блокировку — watchdog обнаруживает это и автоматически находит новую рабочую стратегию.

---

## Требования

- Windows 7 или новее
- Права администратора (обязательно — WinDivert устанавливает драйвер ядра)
- Интернет-соединение для определения провайдера и тестирования

---

## Установка

Распакуйте архив. Структура должна выглядеть так:

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

Папка `data/` создаётся автоматически при первом запуске.

> Всегда запускайте от имени администратора — иначе WinDivert не загрузится.

---

## Использование

### Запуск с лучшей известной стратегией

```
zapret-core.exe
```

Определяет вашего провайдера, загружает лучшую стратегию из базы знаний и запускается. Работает до Ctrl+C.

Если база знаний пуста — предлагает сначала запустить `--find`.

---

### Поиск рабочей стратегии

```
zapret-core.exe --find
```

Тестирует до 137 комбинаций параметров и останавливается на первой, которая работает. Прогресс показывается в реальном времени:

```
[1/137] Testing: auto-1 [fake/ts/file]
  score=0.33  YouTube:FAIL  Discord:FAIL  Google:OK

[4/137] Testing: auto-4 [fake/badseq/file]
  score=1.00  YouTube:OK  Discord:OK  Google:OK

[+] Working strategy found: auto-4 [fake/badseq/file]
```

Результат сохраняется в базу знаний и используется при последующих запусках.

**Сколько это занимает:** в лучшем случае — несколько минут. В худшем — до 2 часов, если ничего сразу не работает. На практике большинство пользователей находят рабочую стратегию в течение первых 10–20 попыток.

---

### Мониторинг с авто-восстановлением

```
zapret-core.exe --watch
```

Запускает фоновый мониторинг. Каждые 60 секунд проверяет YouTube и Discord. Если три проверки подряд не удаются — автоматически находит новую стратегию и переключается на неё.

Остановка через Ctrl+C. И watchdog, и winws корректно завершат работу.

---

### Статус

```
zapret-core.exe --status
```

Показывает, запущен ли winws, и лучшую известную стратегию для вашего провайдера. Завершается немедленно.

---

### Остановка

```
zapret-core.exe --stop
```

Останавливает winws. Завершается немедленно.

---

### HTTP API

```
zapret-core.exe --server
```

Запускает HTTP сервер на `127.0.0.1:7432`. Предназначен для интеграции с внешними приложениями, например Tauri UI. Остановка по Ctrl+C.

---

## Справка HTTP API

Все эндпоинты только локальные (`127.0.0.1:7432`).

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

Запускает лучшую известную стратегию. Возвращает `404`, если стратегии ещё не известны.

```json
{ "status": "started", "strategy": "auto-4 [fake/badseq/file]" }
```

### POST /api/stop

Останавливает winws.

```json
{ "status": "stopped" }
```

### POST /api/find

Запускает поиск стратегии. Возвращает SSE поток.

```
event: progress
data: {"current": 3, "total": 137, "strategy": "auto-3 [fake/ts/file]", "score": 0.33}

event: success
data: {"strategy": "auto-4 [fake/badseq/file]", "score": 1.0, "vector": {...}}
```

Возвращает `409 Conflict`, если уже выполняется другая операция.

### POST /api/watchdog

Запускает watchdog в фоне. Возвращает немедленно.

```json
{ "status": "started", "message": "watchdog running in background" }
```

### DELETE /api/watchdog

```json
{ "status": "stopped" }
```

---

## Конфигурация

`data/config.json` создаётся автоматически при первом запуске:

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

| Параметр | По умолчанию | Описание |
|---|---|---|
| `score_threshold` | `0.6` | Минимальный тестовый балл для принятия стратегии (0–1) |
| `fail_threshold` | `3` | Количество последовательных неудач перед срабатыванием восстановления watchdog |
| `check_interval` | `60` | Как часто watchdog проверяет соединение (секунды) |
| `init_delay` | `5` | Сколько ждать после запуска winws перед тестированием (секунды) |
| `test_timeout` | `8` | Тайм-аут одного HTTP запроса (секунды) |
| `test_runs` | `2` | Сколько раз повторять каждый тест для надёжности |

---

## База знаний

`data/knowledge.json` хранит стратегии, которые сработали для каждого провайдера (по ASN). При следующем запуске они тестируются первыми — до начала полного поиска.

Удаление файла вызывает полный поиск с нуля. Файл не растёт бесконечно — дубликаты обновляются, не добавляются.

---

## Обнаружение конфликтов

Перед поиском zapret-core проверяет наличие программ, известных как конфликтующие с WinDivert:

- GoodbyeDPI
- AdGuardSvc
- discordfix_zapret
- winws1, winws2
- Killer NIC
- Intel Connectivity Network Service
- Check Point (TracSrvWrapper, EPWD)
- SmartByte

Если конфликт найден, поиск останавливается с сообщением. Отключите конфликтующую программу и попробуйте снова.

---

## Логи

Логи пишутся и в консоль, и в `data/zapret.log`. Уровни: `[INFO]`, `[WARN]`, `[ERROR]`.

---

## Устранение проблем

**"No known strategies. Run --find"**
База знаний пуста или не содержит записей для вашего провайдера. Запустите `zapret-core.exe --find`.

**"No working strategy found"**
Ни одна комбинация не прошла порог score_threshold. Проверьте интернет-соединение или увеличьте `test_timeout` в config.json.

**"Resolve conflicts and try again"**
Запущена конфликтующая программа. Остановите её и повторите попытку.

**"failed to start winws"**
`assets/winws.exe` не найден или отсутствуют права администратора.

**409 в API**
Выполняется другая операция. Подождите её завершения или остановите через `POST /api/stop`.

---

## Интеграция с Tauri

zapret-core разработан для работы как sidecar процесс. Запустите с `--server` и вызывайте API через reqwest:

```rust
use reqwest::Client;

let client = Client::new();

// Статус
let status = client
    .get("http://127.0.0.1:7432/api/status")
    .send().await?
    .json::<serde_json::Value>().await?;

// Запуск стратегии
client.post("http://127.0.0.1:7432/api/start").send().await?;

// Поиск стратегии с SSE стримингом
let mut stream = client
    .post("http://127.0.0.1:7432/api/find")
    .send().await?
    .bytes_stream();
```

---

## Благодарности

- [zapret](https://github.com/bol-van/zapret) от bol-van — основной движок обхода DPI (winws, интеграция WinDivert, бинарники fake пакетов)
- [zapret-discord-youtube](https://github.com/flowseal/zapret-discord-youtube) от Flowseal — пресеты стратегий и исследование параметров, которое определило пространство поиска в этом проекте

---

## Лицензия

[MIT](LICENSE) © elevenSure
