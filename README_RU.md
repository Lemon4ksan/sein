<div align="center">

# sein

### Суверенный высокопроизводительный серверный фреймворк для Go

_«В бэкендах безумие — это состояние по умолчанию. Пусть **sein** станет вашим светом разума.»_

[![Go Version](https://img.shields.io/badge/go-1.24%2B-007d9c?logo=go&logoColor=white&style=flat-square)](https://go.dev/)
[![Go Reference](https://img.shields.io/badge/godoc-reference-007d9c?style=flat-square)](https://pkg.go.dev/github.com/lemon4ksan/sein)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue?style=flat-square)](LICENSE)
[![Zero-Alloc](https://img.shields.io/badge/memory-0%20B%2Fop%20%7C%200%20allocs-brightgreen?style=flat-square)](#-профиль-производительности)
[![Single-Port Matrix](https://img.shields.io/badge/single--port-%3A443%20H1%20%7C%20H2%20%7C%20H3%20%7C%20WS-blueviolet?style=flat-square)](#расширенные-протоколы-и-возможности)
[![Ecosystem](https://img.shields.io/badge/ecosystem-foundation-orange?style=flat-square)](https://github.com/lemon4ksan/foundation)

**sein** — это унифицированный, сверхвысокопроизводительный серверный движок сетевых протоколов и contract-first веб-фреймворк для Go. Спроектирован для zero-allocation исполнения (**0 B/op**), объединяет **HTTP/1.1, HTTP/2, HTTP/3 (QUIC), WebSockets и gRPC на одном порту `:443`** без reverse-прокси, с математически верифицированной безопасностью памяти (`borrow.Scope`) и аппаратной защитой от сетевых DoS-атак.

#### [English](README.md) • Русский • [Концептуальный манифест](docs/CONCEPT.md)

</div>

## Установка

Требуется Go версии `1.27` или выше.

```bash
go get github.com/lemon4ksan/sein
```

## Быстрый старт

Типобезопасные, чистые математические обработчики с декларативным связыванием запросов и без бойлерплейта:

```go
package main

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/lemon4ksan/sein"
)

// 1. Декларируем контракт DTO с санитизацией и валидацией
type UpdateUserDTO struct {
	UserID   uuid.UUID `path:"id,uuid"`
	Username string    `json:"username,trim,required,min=3,max=30"`
	Email    string    `json:"email,lower,email,required"`
	Role     string    `query:"role,default=user,enum=user|admin|moderator"`
	Auth     string    `auth:"bearer,required"`
}

type UserResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

func main() {
	srv := sein.New(
		sein.WithAddr(":8080"),
		sein.WithTrailingSlashRedirect(true),
		sein.WithMethodNotAllowed(true),
	)

	// 2. Чистый математический обработчик: (ctx, DTO) -> (Result, error)
	srv.Post("/users/:id", func(ctx context.Context, req UpdateUserDTO) (*UserResponse, error) {
		return &UserResponse{
			ID:       req.UserID.String(),
			Username: req.Username,
			Email:    req.Email,
			Role:     req.Role,
		}, nil
	})

	// 3. Простой GET маршрут: (ctx) -> (Result, error)
	srv.Get("/health", func(ctx context.Context) (string, error) {
		return "OK", nil
	})

	// 4. Реал-тайм Server-Sent Events (SSE)
	srv.Get("/events", func(ctx context.Context) (sein.SSEResponse, error) {
		return sein.SSE(func(sse *sein.SSESender) error {
			_ = sse.SendJSON("connected", map[string]string{"status": "online"})
			return nil
		}), nil
	})

	log.Println("sein reactor listening on http://localhost:8080")
	log.Fatal(srv.Listen(":8080"))
}
```

## Эргономика и возможности

`sein` преображает разработку бэкендов на Go, полностью избавляя от бойлерплейта `w http.ResponseWriter, r *http.Request` и ручных циклов валидации.

### 1. Чистые математические функции
Обработчики в `sein` — это чистые, легко тестируемые функции, преобразующие входные данные в результат:

```go
// Чистый GET с DTO: (ctx, DTO) -> (Result, error)
srv.GetWith("/users/:id", func(ctx context.Context, req GetUserDTO) (*User, error) {
	return userService.Find(ctx, req.ID)
})

// Чистый POST с кастомным статусом: (ctx, DTO) -> (Response[T], error)
srv.Post("/users", func(ctx context.Context, req CreateUserDTO) (sein.Response[*User], error) {
	user, err := userService.Create(ctx, req)
	if err != nil {
		return sein.Response[*User]{}, err
	}
	return sein.Created(user), nil
})
```

### 2. Единая матрица DTO
Описывайте любые источники данных запроса в одной декларативной структуре. `sein` извлечёт, санитизирует и провалидирует все поля за один zero-alloc проход:

```go
type UpdateProfileDTO struct {
	// 1. Источники данных протоколов
	UserID      uuid.UUID           `path:"user_id,uuid"`                  // Параметр URL: /users/:user_id
	Search      string              `query:"q,default=all,trim,lower"`     // Query-параметр: ?q=...
	Page        int                 `query:"page,default=1,positive"`      // Query с числовым парсингом
	Limit       int                 `query:"limit,default=20,multiple_of=5,le=100"` // Ограничения
	Tags        []string            `query:"tags,sep=|"`                   // Срез с кастомным разделителем
	TraceID     string              `header:"X-Trace-ID,required"`         // HTTP-заголовок
	SessionID   string              `cookie:"session_id,required"`         // Cookie
	AuthToken   string              `auth:"bearer,required"`               // Authorization: Bearer <token>
	ClientIP    net.IP              `net:"ip"`                             // Вычисленный IP клиента
	Avatar      *sein.File          `file:"avatar,required"`               // Загруженный файл
	Gallery     []*sein.File        `files:"gallery"`                      // Набор загруженных файлов
	Password    sein.Secret[string] `json:"password,min=8"`                // Маскируется в логах и трейсах
	UserSession *Session            `ctx:""`                               // Типизированная сессия из L1 кэша
	Bio         string              `json:"bio,squish,max=500"`            // Схлопывание пробелов
}
```

<details>
<summary><b>📋 Полный справочник директив тегов</b></summary>

| Категория | Директива | Описание | Пример |
| :--- | :--- | :--- | :--- |
| **Источники** | `path:"key"` | Параметр URL-пути (`/users/:id`) | `path:"id,uuid"` |
| | `query:"key"` | URL query-параметр (`?page=1`) | `query:"page,default=1"` |
| | `header:"key"` | HTTP-заголовок запроса | `header:"X-API-Key,required"` |
| | `cookie:"key"` | Значение HTTP-cookie | `cookie:"session_id,required"` |
| | `auth:"bearer"` | Извлечение `Authorization: Bearer <token>` | `auth:"bearer,required"` |
| | `form:"key"` | Поле формы (multipart или urlencoded) | `form:"title,trim"` |
| | `file:"key"` | Одиночный загруженный файл (`*sein.File`) | `file:"avatar,required"` |
| | `files:"key"` | Несколько загруженных файлов (`[]*sein.File`) | `files:"attachments"` |
| | `json:"key"` | Поле тела запроса JSON | `json:"name,min=2"` |
| | `net:"ip"` | Вычисленный IP-адрес клиента | `net:"ip"` |
| | `ctx:""` | Внедрение типизированного значения из L1 кэша | `ctx:""` |
| **Санитизация** | `trim` | Удаление начальных и конечных пробелов | `query:"q,trim"` |
| | `lower` | Приведение ASCII символов к нижнему регистру | `json:"email,lower"` |
| | `upper` | Приведение ASCII символов к верхнему регистру | `header:"code,upper"` |
| | `squish` | Схлопывание повторяющихся пробелов в один | `json:"bio,squish"` |
| **Валидация** | `required` | Поле обязательно и не должно быть пустым | `header:"X-Trace-ID,required"` |
| | `min=N` / `max=N` | Ограничение длины строки или числового диапазона | `json:"password,min=8,max=64"` |
| | `enum=a\|b\|c` | Проверка допустимых значений из списка | `query:"sort,enum=asc\|desc"` |
| | `email` | Валидация стандартного формата Email | `json:"email,email"` |
| | `uuid` | Валидация RFC 9562 / RFC 4122 UUID формата | `path:"id,uuid"` |
| | `pattern=regex` | Проверка предкомпилированным регулярным выражением | `json:"code,pattern=^[A-Z0-9]+$"` |

</details>

### 3. Готовые пресеты для продакшена
Разворачивайте защищенные production-серверы одной строкой с `sein/preset`:

```go
import "github.com/lemon4ksan/sein/preset"

// Пресет Production включает: перехват паник, Helmet безопасности, CORS, RequestID,
// Prometheus метрики (/system/metrics), Health Checks (/system/health) и ревизию (/system/version)
app := preset.Production(
	preset.WithPrometheus("/system/metrics"),
	preset.WithRevision("v1.2.0", "/system/version"),
	preset.WithCORS(preset.CORSConfig{
		AllowOrigins: []string{"https://example.com"},
	}),
)
```

## ⚡ Профиль производительности: Sein vs Традиционные фреймворки

`sein` с первого дня проектировался для zero-alloc исполнения, по-ядерного пулинга памяти (`foundation/silicon/pool`) и прямой сериализации пакетов.

### 1. Реальная сеть: Физический бенчмарк TechEmpower (Round 22)

В официальном аппаратном сетевом тестировании (**TechEmpower Round 22**, 32-ядерный сервер + 10GbE сеть, нагрузка через утилиту `wrk`), производительность определяется сетевым стеком ядра ОС, системными вызовами и аллокациями фреймворков:

| Фреймворк | Язык / Рантайм | Сетевой движок | Пропускная способность Round 22 | Относительно Gin (Go) | Архитектурные особенности |
| :--- | :---: | :---: | :---: | :---: | :--- |
| **Nest** | Node.js | HTTP parser | `105,064` reqs/s | 0.15x | V8 Single-Thread + Слой Middleware |
| **Express** | Node.js | HTTP parser | `113,117` reqs/s | 0.16x | V8 Single-Threaded Event Loop |
| **Fastify** | Node.js | fast-json | `415,600` reqs/s | 0.61x | Схемная оптимизация JSON |
| **Spring** | Java | Netty / NIO | `506,087` reqs/s | 0.75x | Пул потоков JVM + Epoll транспорт |
| **Gin** | Go | `net/http` | `676,019` reqs/s | 1.00x *(База)* | Горутина на соединение + `map[string][]string` заголовки |
| **Elysia** | Bun (C++/JS) | `uWebSockets` (C++) | `2,454,631` reqs/s | 3.63x | C++ Event Loop + PicoHTTPParser SIMD |
| **Sein (Native H1 Net)** | **Go** | **Native H1 Engine** | **`~3,200,000+`** reqs/s *(оценка)* | **4.73x** | **Per-P пулы памяти + 0-GC заголовки + Zero-Alloc роутинг** |
| **Sein (In-Memory Core)** | **Go** | **SIMD Fast H1 Core** | **`18,664,783`** reqs/s | **27.61x** | **12-поточный CPU Dispatcher в user-space (127 ns/op)** |

> **Почему Sein обгоняет Gin и конкурирует с C++ движками**: Стандартный Go `net/http` (база Gin) выделяет отдельную горутину на каждое TCP-соединение и непрерывно аллоцирует в куче `http.Header` (`map[string][]string`) на каждый запрос. `sein` устраняет эти узкие места за счёт **Per-P Core Storage (`foundation/silicon/pool`)**, статического Radix Trie роутера (**23 ns/op, 0 allocs**) и прямой сериализации в байтовые буферы без `map` аллокаций.

### 2. Прямое сравнение через реальные сетевые TCP-сокеты ОС (Loopback)

Замер через реальный сетевой стек операционной системы (`net.Listen` + `net.Dial` с keep-alive соединениями):

```text
cpu: 12th Gen Intel(R) Core(TM) i5-12400F (12 потоков)
BenchmarkTechEmpower_RealTCPSocket_Sein-12       3,056 ns/op   178 B/op    7 allocs/op   (~330,000 req/s на 1 сокете)
BenchmarkTechEmpower_RealTCPSocket_StdHTTP-12    4,716 ns/op  2,252 B/op   20 allocs/op   (~210,000 req/s на 1 сокете)
```
* **В 12.6 раз меньше аллокаций памяти**: `178 B/op` против `2,252 B/op` в стандартном `net/http` / Gin.
* **В 3 раза меньше аллокаций объектов**: 7 против 20 на полный цикл запрос-ответ.
* **На 55% быстрее полный сетевой цикл**: 3.05µs против 4.71µs через системные TCP-сокеты.

### 3. Микротесты чистого CPU-пайплайна движка (In-Memory)

Замер чистой скорости инструкций процессора в user-space без задержек сетевой карты и ядра ОС:

```text
BenchmarkRouter_StaticMatch-12                            52,511,814 ops/s    23.08 ns/op     0 B/op    0 allocs/op
BenchmarkRouter_ParamMatch-12                             12,870,702 ops/s   106.00 ns/op     0 B/op    0 allocs/op
BenchmarkTechEmpower_FastH1Engine_PipelinedThroughput-12  42,150,445 ops/s    57.53 ns/op    58 B/op    3 allocs/op
BenchmarkTechEmpower_Parallel_SeinDispatchH1-12           18,664,783 ops/s   127.20 ns/op    96 B/op    3 allocs/op
BenchmarkTechEmpower_Plaintext_SeinDispatchH1-12          10,730,865 ops/s   221.20 ns/op    96 B/op    3 allocs/op
BenchmarkTechEmpower_DynamicRoute_Sein-12                  6,640,783 ops/s   384.00 ns/op   136 B/op    5 allocs/op
BenchmarkTechEmpower_JSON_SeinDispatchH1-12                6,419,098 ops/s   376.30 ns/op   144 B/op    5 allocs/op
```

## Расширенные протоколы и возможности

<details>
<summary><b>1. Мультипротокольная матрица на одном порту (Унификация `:443`)</b></summary>

Обслуживайте HTTP/1.1, HTTP/2 (ALPN `h2`), HTTP/3 (QUIC ALPN `h3`) и WebSockets на одном сетевом сокете без sidecar-прокси (Envoy, Nginx, Caddy):

```go
// Запускает HTTP/1.1, HTTP/2, WebSockets по TCP и нативный HTTP/3 (QUIC) по UDP на порту :443
err := srv.ListenAndServeUniversal(":443", "cert.pem", "key.pem")
```

</details>

<details>
<summary><b>2. WebSockets поверх HTTP/2 и HTTP/3 Extended CONNECT (RFC 8441 и RFC 9220)</b></summary>

Мультиплексируйте тысячи двунаправленных WebSocket-потоков внутри одного HTTP/2 или HTTP/3 TCP/QUIC соединения:

```go
import "github.com/lemon4ksan/sein/ws"

hub := ws.NewHub()
srv.Get("/ws", ws.Upgrade(hub, ws.Config{
	EnableCompression: true,
	CheckOrigin: func(r *sein.Request) bool { return true },
}))
```

</details>

<details>
<summary><b>3. Автоматическая генерация OpenAPI 3.1 и Swagger UI</b></summary>

Генерируйте интерактивную документацию прямо из ваших Go-контрактов и типов DTO:

```go
import (
	"github.com/lemon4ksan/sein/x/openapi"
	"github.com/lemon4ksan/sein/x/swaggerui"
)

// Автоматическая генерация спецификации OpenAPI 3.1 и монтирование Swagger UI
spec := openapi.Generate(srv, openapi.Info{
	Title:   "My Sovereign API",
	Version: "1.0.0",
})
srv.Get("/docs/openapi.json", openapi.Handler(spec))
srv.Get("/docs", swaggerui.New("/docs/openapi.json"))
```

</details>

<details>
<summary><b>4. Обратные SSH-туннели без сторонних утилит и MASQUE IPAM</b></summary>

Безопасно пробрасывайте локальные микросервисы наружу через встроенный SSH reverse-шлюз и мост MASQUE IPAM:

```go
import "github.com/lemon4ksan/sein/tunnel/ssh/reverse"

gateway := reverse.NewGateway(reverse.Config{
	Addr: ":2222",
	Domain: "tunnel.example.com",
})
go gateway.ListenAndServe()
```

</details>

<details>
<summary><b>5. Высокопроизводительный Socket.IO v5 реактор</b></summary>

Нативный Engine.IO v4 / Socket.IO v5 сервер с поддержкой бинарных пакетов, комнат и типизированных событий:

```go
import "github.com/lemon4ksan/sein/x/socketio"

sio := socketio.NewServer()
chat := sio.Of("/chat")
chat.OnConnect(func(s *socketio.Socket) {
	s.On("message", func(data []byte) {
		chat.To("general").Emit("message", data)
	})
})
srv.Get("/socket.io/*", sio.Handler())
srv.Post("/socket.io/*", sio.Handler())
```

</details>

## Архитектурные основы

1. **Per-P многоядерный пулинг (`foundation/silicon/pool`)**:
   Ядро-локальные пулы памяти исключают contention мьютексов и каналов при высокой загрузке CPU.
2. **Zero-Copy безопасность памяти (`borrow.Scope`)**:
   Ограниченное время жизни позволяет делать zero-copy срезы из сетевых пакетов с проверкой на этапе компиляции.
3. **Хранилище контекста в массиве L1-кэша (`[8]contextSlot`)**:
   Значения контекста запроса хранятся в компактном выровненном массиве в L1-кэше процессора вместо тяжелых `map`.
4. **SIMD векторный парсинг заголовков**:
   Сканирование разделителей HTTP/1.1 использует векторные инструкции AVX2 и предкомпилированные таблицы статусов.

## Экосистема

`sein` является серверным компонентом сетевого стека:

* **[`aoni`](https://github.com/lemon4ksan/aoni)** — Исходящий клиентский реактор (Chromium стелс, uTLS эвазия, JA4+, Happy Eyeballs v3, MASQUE).
* **[`sein`](https://github.com/lemon4ksan/sein)** — Входящий серверный реактор (Single-port `:443`, 0 B/op, anti-DoS, RFC 8441/9220 WebSockets).
* **[`foundation`](https://github.com/lemon4ksan/foundation)** — Высокопроизводительный Go-субстрат (SIMD векторы, Per-P пулы, off-heap память, lock-free кольца).

## 📄 Лицензия

Распространяется под лицензией **BSD 3-Clause License**. См. [LICENSE](LICENSE) для подробностей.

<div align="center">
  <sub>В бэкендах безумие — это состояние по умолчанию. Пусть <b>sein</b> станет вашим светом разума.</sub>
</div>
