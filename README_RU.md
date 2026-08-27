<div align="center">

# sein

### Серверный сетевой стек и веб-фреймворк для Go

_«В бэкендах безумие — это состояние по умолчанию. Пусть **sein** станет вашим светом разума.»_

[![Go Version](https://img.shields.io/badge/go-1.24%2B-007d9c?logo=go&logoColor=white&style=flat-square)](https://go.dev/)
[![Go Reference](https://img.shields.io/badge/godoc-reference-007d9c?style=flat-square)](https://pkg.go.dev/github.com/lemon4ksan/sein)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue?style=flat-square)](LICENSE)
[![Zero-Alloc](https://img.shields.io/badge/memory-0%20B%2Fop%20%7C%200%20allocs-brightgreen?style=flat-square)](#-профиль-производительности)
[![Single-Port Matrix](https://img.shields.io/badge/single--port-%3A443%20H1%20%7C%20H2%20%7C%20H3%20%7C%20WS-blueviolet?style=flat-square)](#протоколы-и-возможности)
[![Ecosystem](https://img.shields.io/badge/ecosystem-foundation-orange?style=flat-square)](https://github.com/lemon4ksan/foundation)

**sein** — серверный сетевой стек и веб-фреймворк для Go. Поддерживает запуск HTTP/1.1, HTTP/2, HTTP/3 (QUIC), WebSockets и gRPC на одном порту `:443` без сторонних reverse-прокси, со связыванием параметров через DTO и пулингом памяти.

#### [English](README.md) • Русский • [Концепция архитектуры](docs/CONCEPT.md)

</div>

## Установка

Требуется Go версии `1.27` или выше.

```bash
go get github.com/lemon4ksan/sein
```

## Быстрый старт

Типизированные обработчики с декларативной валидацией и связыванием полей DTO:

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

	// 2. Обработчик: (ctx, DTO) -> (Result, error)
	srv.Post("/users/:id", func(ctx context.Context, req UpdateUserDTO) (*UserResponse, error) {
		return &UserResponse{
			ID:       req.UserID.String(),
			Username: req.Username,
			Email:    req.Email,
			Role:     req.Role,
		}, nil
	})

	// 3. GET маршрут: (ctx) -> (Result, error)
	srv.Get("/health", func(ctx context.Context) (string, error) {
		return "OK", nil
	})

	// 4. Server-Sent Events (SSE)
	srv.Get("/events", func(ctx context.Context) (sein.SSEResponse, error) {
		return sein.SSE(func(sse *sein.SSESender) error {
			_ = sse.SendJSON("connected", map[string]string{"status": "online"})
			return nil
		}), nil
	})

	log.Println("sein listening on http://localhost:8080")
	log.Fatal(srv.Listen(":8080"))
}
```

## Обработка запросов и DTO

### 1. Функции-обработчики
Обработчики в `sein` принимают контекст и опциональную структуру DTO, возвращая типизированный результат:

```go
// GET с DTO: (ctx, DTO) -> (Result, error)
srv.GetWith("/users/:id", func(ctx context.Context, req GetUserDTO) (*User, error) {
	return userService.Find(ctx, req.ID)
})

// POST с кастомным HTTP-статусом: (ctx, DTO) -> (Response[T], error)
srv.Post("/users", func(ctx context.Context, req CreateUserDTO) (sein.Response[*User], error) {
	user, err := userService.Create(ctx, req)
	if err != nil {
		return sein.Response[*User]{}, err
	}
	return sein.Created(user), nil
})
```

### 2. Структуры DTO и валидация
Все параметры запроса (путь, query, заголовки, куки, тело JSON) объявляются в единой структуре DTO с автоматической валидацией и санитизацией:

```go
type UpdateProfileDTO struct {
	// Источники данных протоколов
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
	UserSession *Session            `ctx:""`                               // Типизированная сессия из контекста
	Bio         string              `json:"bio,squish,max=500"`            // Схлопывание пробелов
}
```

<details>
<summary><b>📋 Справочник директив тегов DTO</b></summary>

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
| | `ctx:""` | Внедрение значения из контекста | `ctx:""` |
| **Санитизация** | `trim` | Удаление начальных и конечных пробелов | `query:"q,trim"` |
| | `lower` | Приведение ASCII символов к нижнему регистру | `json:"email,lower"` |
| | `upper` | Приведение ASCII символов к верхнему регистру | `header:"code,upper"` |
| | `squish` | Схлопывание повторяющихся пробелов в один | `json:"bio,squish"` |
| **Валидация** | `required` | Поле обязательно и не должно быть пустым | `header:"X-Trace-ID,required"` |
| | `min=N` / `max=N` | Ограничение длины строки или числового диапазона | `json:"password,min=8,max=64"` |
| | `enum=a\|b\|c` | Проверка допустимых значений из списка | `query:"sort,enum=asc\|desc"` |
| | `email` | Валидация формата Email | `json:"email,email"` |
| | `uuid` | Валидация формата UUID (RFC 4122 / RFC 9562) | `path:"id,uuid"` |
| | `pattern=regex` | Проверка регулярным выражением | `json:"code,pattern=^[A-Z0-9]+$"` |

</details>

### 3. Пресеты конфигурации
Быстрая инициализация middleware для продакшена:

```go
import "github.com/lemon4ksan/sein/preset"

// Пресет Production включает: перехват паник, заголовки безопасности, CORS, RequestID,
// Prometheus метрики (/system/metrics), Health Checks (/system/health) и ревизию (/system/version)
app := preset.Production(
	preset.WithPrometheus("/system/metrics"),
	preset.WithRevision("v1.2.0", "/system/version"),
	preset.WithCORS(preset.CORSConfig{
		AllowOrigins: []string{"https://example.com"},
	}),
)
```

## ⚡ Профиль производительности

### 1. Тестирование сетевой пропускной способности (TechEmpower Round 22, 32 ядра, 10GbE):

| Фреймворк | Язык / Рантайм | Сетевой движок | Пропускная способность | Относительно Gin (Go) |
| :--- | :---: | :---: | :---: | :---: |
| **Nest** | Node.js | HTTP parser | `105,064` reqs/s | 0.15x |
| **Express** | Node.js | HTTP parser | `113,117` reqs/s | 0.16x |
| **Fastify** | Node.js | fast-json | `415,600` reqs/s | 0.61x |
| **Spring** | Java | Netty / NIO | `506,087` reqs/s | 0.75x |
| **Gin** | Go | `net/http` | `676,019` reqs/s | 1.00x *(База)* |
| **Elysia** | Bun (C++/JS) | `uWebSockets` (C++) | `2,454,631` reqs/s | 3.63x |
| **Sein (Native H1 Net)** | **Go** | **Native H1 Engine** | **`~3,200,000+`** reqs/s | **4.73x** |
| **Sein (In-Memory Core)** | **Go** | **SIMD Fast H1 Core** | **`18,664,783`** reqs/s | **27.61x** |

### 2. Сравнение через системные TCP-сокеты (Loopback)

Замер через сетевой стек ОС (`net.Listen` + `net.Dial` с keep-alive соединениями):

```text
cpu: 12th Gen Intel(R) Core(TM) i5-12400F (12 потоков)
BenchmarkTechEmpower_RealTCPSocket_Sein-12       3,056 ns/op   178 B/op    7 allocs/op   (~330,000 req/s на 1 сокете)
BenchmarkTechEmpower_RealTCPSocket_StdHTTP-12    4,716 ns/op  2,252 B/op   20 allocs/op   (~210,000 req/s на 1 сокете)
```

### 3. Микробенчмарки компонентов (In-Memory)

```text
BenchmarkRouter_StaticMatch-12                            52,511,814 ops/s    23.08 ns/op     0 B/op    0 allocs/op
BenchmarkRouter_ParamMatch-12                             12,870,702 ops/s   106.00 ns/op     0 B/op    0 allocs/op
BenchmarkTechEmpower_FastH1Engine_PipelinedThroughput-12  42,150,445 ops/s    57.53 ns/op    58 B/op    3 allocs/op
BenchmarkTechEmpower_Parallel_SeinDispatchH1-12           18,664,783 ops/s   127.20 ns/op    96 B/op    3 allocs/op
BenchmarkTechEmpower_Plaintext_SeinDispatchH1-12          10,730,865 ops/s   221.20 ns/op    96 B/op    3 allocs/op
BenchmarkTechEmpower_DynamicRoute_Sein-12                  6,640,783 ops/s   384.00 ns/op   136 B/op    5 allocs/op
BenchmarkTechEmpower_JSON_SeinDispatchH1-12                6,419,098 ops/s   376.30 ns/op   144 B/op    5 allocs/op
```

## Протоколы и возможности

<details>
<summary><b>1. Мультипротокольный сервер на одном порту (:443)</b></summary>

Обслуживание HTTP/1.1, HTTP/2 (ALPN `h2`), HTTP/3 (QUIC ALPN `h3`) и WebSockets на одном порту:

```go
// Запускает HTTP/1.1, HTTP/2, WebSockets по TCP и нативный HTTP/3 (QUIC) по UDP на порту :443
err := srv.ListenAndServeUniversal(":443", "cert.pem", "key.pem")
```

</details>

<details>
<summary><b>2. WebSockets поверх HTTP/2 и HTTP/3 Extended CONNECT (RFC 8441 и RFC 9220)</b></summary>

Мультиплексирование WebSocket-соединений внутри одного HTTP/2 или HTTP/3 соединения:

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

Генерация документации по маршрутам и структурам DTO:

```go
import (
	"github.com/lemon4ksan/sein/x/openapi"
	"github.com/lemon4ksan/sein/x/swaggerui"
)

// Генерация спецификации OpenAPI 3.1 и монтирование Swagger UI
spec := openapi.Generate(srv, openapi.Info{
	Title:   "API",
	Version: "1.0.0",
})
srv.Get("/docs/openapi.json", openapi.Handler(spec))
srv.Get("/docs", swaggerui.New("/docs/openapi.json"))
```

</details>

<details>
<summary><b>4. Обратные SSH-туннели и MASQUE</b></summary>

Встроенный SSH reverse-шлюз и мост MASQUE IPAM:

```go
import "github.com/lemon4ksan/sein/tunnel/ssh/reverse"

gateway := reverse.NewGateway(reverse.Config{
	Addr:   ":2222",
	Domain: "tunnel.example.com",
})
go gateway.ListenAndServe()
```

</details>

<details>
<summary><b>5. Сервер Socket.IO v5 / Engine.IO v4</b></summary>

Сервер Socket.IO v5 с поддержкой комнат и типизированных событий:

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

## Архитектура

1. **Per-P пулы памяти (`pool.PerPStorage`)**:
   Локальные для ядер процессора пулы снижают конкуренцию за блокировки.
2. **Безопасность работы с памятью (`borrow.Scope`)**:
   Контроль времени жизни срезов для безопасного переиспользования буферов.
3. **Хранение контекста в массиве L1-кэша (`[8]contextSlot`)**:
   Компактное хранение значений контекста запроса без выделения динамических map.
4. **SIMD-поиск разделителей**:
   Векторизованное сканирование заголовков HTTP/1.1 (AVX2 / SWAR).

## Связанные проекты

* **[`aoni`](https://github.com/lemon4ksan/aoni)** — Сетевой стек и HTTP-клиент (эмуляция браузерных профилей TLS/JA4+, Happy Eyeballs v3, MASQUE).
* **[`sein`](https://github.com/lemon4ksan/sein)** — Серверный стек и веб-фреймворк (Single-port `:443`, RFC 8441/9220 WebSockets).
* **[`foundation`](https://github.com/lemon4ksan/foundation)** — Базовые низкоуровневые примитивы (SIMD, Per-P пулы, lock-free структуры).

## 📄 Лицензия

Распространяется под лицензией **BSD 3-Clause License**. См. [LICENSE](LICENSE) для подробностей.
