<div align="center">

# sein

### Единый серверный реактор сетевых протоколов для Go

[![License](https://img.shields.io/github/license/lemon4ksan/sein?style=flat-square)](LICENSE)
[![Status](https://img.shields.io/badge/статус-активная%20разработка-blue?style=flat-square)](#)
[![Ecosystem](https://img.shields.io/badge/экосистема-foundation-blueviolet?style=flat-square)](https://github.com/lemon4ksan/foundation)

> _«В бэкендах безумие — это состояние по умолчанию. Пусть **sein** станет вашим светом разума.»_

#### [English](README.md) • Русский • [Концептуальный Манифест](docs/CONCEPT.md)

</div>

## 1. Видение проекта (The One-Sentence Pitch)

**`sein`** — это разрабатываемый суверенный серверный реактор на Go с нулевыми аллокациями памяти (**0 B/op**), объединяющий **HTTP/1.1, HTTP/2, HTTP/3 (QUIC), WebSockets и gRPC на едином порту `:443`** без внешних прокси-серверов, с математически верифицированной безопасностью памяти (`borrow.Scope`) и аппаратным иммунитетом к сетевым DoS-атакам.

## 2. Проблема: Конец трилеммы серверов

Современная серверная разработка на Go разорвана между тремя компромиссами:

1. **`net/http` / `Gin`:** Безопасно и стандартно, но требует множественных аллокаций памяти на запрос, нагружает GC и испытывает конкуренцию за блокировки на многоядерных CPU.
2. **`fasthttp` / `Fiber`:** Высокая скорость на HTTP/1.1, но нет нативной поддержки HTTP/2 и HTTP/3, а ручной рециклинг буферов приводит к Use-After-Free при утечке памяти в горутины.
3. **`grpc-go` / `Nginx`:** Требует поддержки отдельных портов под каждый протокол, выделения глубоких деревьев указателей Protobuf и настройки внешних реверс-прокси для объединения трафика.

**Цель `sein`:** объединить скорость zero-copy исполнения, безопасность типов и полный стек протоколов IETF на одном сокете.

## 3. Четыре фундаментальных инварианта

### I. Single-Port Protocol Matrix (Единый порт `:443`)
* **Единая точка входа:** Одновременное прослушивание TCP `:443` и UDP `:443`.
* **Без внешнего прокси-слоя:** Прямая диспетчеризация входящих соединений без Nginx или Envoy сайдкаров.
* **Быстрый ALPN & Connection ID демультиплексор:** Маршрутизация протоколов на уровне байтовых сигнатур TLS 1.3 ALPN (`h2`, `http/1.1`) и QUIC Connection ID (`h3`).
* **Нативный WebSocket over H2/H3:** Подключение WebSockets через **RFC 8441** и **RFC 9220** (Extended `CONNECT`) внутри существующих мультиплексированных стримов без необходимости перехвата TCP-сокета (`Hijack()`).

### II. Кремниевый детерминизм и безопасность памяти (Цель: 0 B/op)
* **Кольцевые пулы с привязкой к ядрам:** Рециклинг буферов и контекстов через `foundation/silicon/pool` (`PerPStorage`) без межъядерных блокировок.
* **Плоский Хаффман и SIMD:** Декодирование HTTP/2 HPACK через предвычисленные матрицы поиска и быстрое сканирование разделителей `\r\n` с помощью AVX2/BMI2.
* **Compile-Time безопасность заимствований:** Интеграция с `borrow.Scope` и анализатором `vortex check` ($P * Q$ Separation Logic), гарантирующая, что zero-copy срезы памяти невозможно случайно передать в асинхронные горутины без явного копирования.

### III. Аппаратный иммунитет к DoS-атакам
* **Защита Anti-Rapid Reset (CVE-2023-44487):** Токен-бакеты на уровне каждого сокета для ограничения частоты кадров `RST_STREAM`.
* **Anti-Slowloris по реальной скорости:** Контроль физической скорости передачи (`MinTransferRate`) вместо исключительно статических таймаутов.
* **Fair Queuing шедулер стримов:** Динамическое чередование кадров `DATA` между параллельными стримами для предотвращения блокировок head-of-line.
* **Защита от компрессионных бомб:** Ограничение максимального размера распаковки таблиц HPACK и QPACK на лету.

### IV. Декларативный симбиоз с `vortex` (No-Glue Architecture)
* **Единый контракт AST:** Описание схем и сервисов через стандартные интерфейсы Go, OpenAPI 3.1 или Protobuf.
* **Генерация кода без рефлексии:** `vortex gen` компилирует интерфейсы напрямую в Radix Trie роутеры (`foundation/silicon/trie`) и типизированные DTO-биндеры.

## 4. Модель безопасности памяти: Проверка на этапе компиляции

Чтобы исключить ошибки Use-After-Free при сохранении 0 B/op производительности, `sein` опирается на статический анализ:

```go
// ❌ Ошибка статической проверки (B001 - Scoped Borrow Escape):
srv.POST("/api/v1/events", func(c *sein.Context) error {
    data := c.Body() // data — заимствованный срез с временем жизни запроса
    go func() {
        processEvent(data) // vortex check: заимствованная память утекает в несинхронизированную горутину
    }()
    return c.SendStatus(200)
})

// ✅ Безопасный паттерн (Явное копирование при асинхронной передаче):
srv.POST("/api/v1/events", func(c *sein.Context) error {
    data := c.BodyClone() // Выделение памяти только при необходимости передачи в фон
    go func() {
        processEvent(data)
    }()
    return c.SendStatus(200)
})
```

* **Escape Prevention (`B001`):** Контроль того, что заимствованные буферы не покидают пределов контекста запроса.
* **Disjoint Intervals (`B003`):** Доказательство непересекаемости мутаций срезов при zero-copy парсинге.
* **Linear Lifecycle (`B011`):** Соблюдение линейного автомата состояний ($\text{Acquired} \to \text{Frozen} \to \text{Released}$).

## 5. Архитектурные принципы высокой производительности

`sein` разрабатывается с учетом физики современного кремния:

* **Исключение GC Mark-Assist:** Рабочая память запроса не попадает в кучу сборщика мусора.
* **Выравнивание кэш-линий:** Разделяемые структуры выравниваются по 64-байтовым границам кэша L1/L2 (`cpu.CacheLinePad`) для устранения False Sharing.
* **Шардирование без блокировок:** Изолированные структуры очередей для каждого логического процессора (`PerPStorage`).
* **Монотонные часы без системных вызовов:** Прямое атомарное чтение монотонного таймера (`silicon/clock`).

## 6. Целевой API

```go
package main

import (
	"log"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/option"
	"github.com/lemon4ksan/sein/ws"
)

type CreateUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}

type UserResponse struct {
	ID       uint64 `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

func main() {
	srv := sein.NewServer(
		option.WithAddr(":443"),
		option.WithTLS("cert.pem", "key.pem"),
		option.WithHTTP3(true),            // Нативный HTTP/3 QUIC на UDP :443
		option.WithFairQueuing(true),      // Балансировка фреймов DATA между стримами
		option.WithAntiRapidReset(1000),   // Лимит 1,000 RST_STREAM/сек на сокет
		option.WithMinTransferRate(1024),  // Anti-Slowloris: минимум 1 КБ/с реальной скорости
	)

	// 1. REST API (Zero-Alloc распаковка JSON)
	srv.POST("/api/v1/users", func(c *sein.Context) error {
		var req CreateUserRequest
		if err := c.BindJSON(&req); err != nil {
			return c.SendStatus(400)
		}
		return c.SendJSON(201, UserResponse{
			ID:       c.NextID(),
			Username: req.Username,
			Email:    req.Email,
		})
	})

	// 2. WebSockets (Мультиплексированный стрим RFC 8441 H2 / RFC 9220 H3)
	srv.WS("/ws/feed", func(conn ws.Conn) {
		defer conn.Close()
		for {
			msg, err := conn.ReadMessage()
			if err != nil {
				break
			}
			_ = conn.WriteMessage(ws.OpText, msg)
		}
	})

	// 3. gRPC (5-байтный фрейминг на том же сокете)
	srv.GRPC("/UserService/GetUser", func(c *sein.GRPCContext) error {
		return c.SendProto(200, &UserResponse{ID: 1, Username: "alice"})
	})

	log.Println("Реактор sein запускается на порту :443 (TCP+UDP)")
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Ошибка сервера: %v", err)
	}
}
```

## 7. Место в экосистеме

`sein` является серверным компонентом связного сетевого стека:

* **`foundation`** — Кремниевый субстрат (SIMD, Off-Heap, PerP, Radix Trie, Lock-Free кольца, stdlib v2).
* **`aoni`** — Клиентский реактор (Chromium stealth, evasion, uTLS, Happy Eyeballs v3, MASQUE).
* **`sein`** — Серверный реактор (Единый порт `:443`, 0 B/op, Anti-DoS, RFC 8441/9220 WebSockets).
* **`vortex`** — Декларативный AST тулчейн ($P * Q$ Borrow Checker, компилятор контрактов, генерация моков).
* **`porthack`** — Движок нагрузочного тестирования и валидации производительности.
* **`decon`** — Защитник периметра и очиститель трафика.
* **`niko`** — Легковесный суверенный рантайм-оркестратор.

## 8. Структура репозитория

```
sein/
├── option/       // Опции конфигурации сервера (WithAddr, WithTLS, WithHTTP3, WithAntiRapidReset)
├── router/       // Zero-alloc Radix Trie роутер (foundation/silicon/trie)
├── h1/           // Конвейер HTTP/1.1 с keep-alive
├── h2/           // Нативный HTTP/2 фрейм-мультиплексор и HPACK flat LUT
├── quic/         // Суверенный Pure-Go RFC 9000 QUIC транспортный движок
├── h3/           // Нативный HTTP/3 RFC 9114 & QPACK RFC 9204 кодек
├── ws/           // RFC 8441 & RFC 9220 WebSocket движок
├── grpc/         // Zero-copy gRPC / gRPC-Web хэндлеры с 5-байтным фреймингом
├── security/     // Токен-бакеты Anti-Rapid Reset, защита от Slowloris
├── compress/     // Ускоренное сжатие flate, zstd, brotli и gzip
├── context.go    // Zero-alloc контекст запроса с жизненным циклом borrow.Scope
└── server.go     // Единый слушатель Single-Port и быстрый ALPN/CID демультиплексор
```

## 9. Лицензия

Распространяется под лицензией **BSD 3-Clause License**. Подробности в файле [LICENSE](LICENSE).

<div align="center">
  <sub>В бэкендах безумие — это состояние по умолчанию. Пусть <b>sein</b> станет вашим светом разума.</sub>
</div>
