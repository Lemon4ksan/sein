<div align="center">

# sein

### Серверный сетевой стек и веб-фреймворк для Go

_«В бэкендах безумие — это состояние по умолчанию. Пусть **sein** станет вашим светом разума.»_

[![Go Version](https://img.shields.io/badge/go-1.27%2B-007d9c?logo=go&logoColor=white&style=flat-square)](https://go.dev/)
[![Go Reference](https://img.shields.io/badge/godoc-reference-007d9c?style=flat-square)](https://pkg.go.dev/github.com/lemon4ksan/sein)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue?style=flat-square)](LICENSE)
[![Zero-Alloc](https://img.shields.io/badge/memory-0%20B%2Fop%20%7C%200%20allocs-brightgreen?style=flat-square)](#-профиль-производительности)
[![Single-Port Matrix](https://img.shields.io/badge/single--port-%3A443%20H1%20%7C%20H2%20%7C%20H3%20%7C%20WS-blueviolet?style=flat-square)](#протоколы-и-возможности)
[![Ecosystem](https://img.shields.io/badge/ecosystem-foundation-orange?style=flat-square)](https://github.com/lemon4ksan/foundation)

**sein** — серверный сетевой стек и веб-фреймворк для Go. Поддерживает запуск HTTP/1.1, HTTP/2, HTTP/3 (QUIC), WebSockets и gRPC на одном порту `:443` без сторонних reverse-прокси, с универсальной компиляцией хэндлеров, декларативным DTO-биндингом и табличной трансляцией доменных ошибок.

#### [English](README.md) • Русский • [Концепция архитектуры](docs/CONCEPT.md)

</div>

## Установка

Требуется Go версии `1.27` или выше.

```bash
go get github.com/lemon4ksan/sein
```

## Быстрый старт

Универсальные обработчики, декларативная валидация и полное отсутствие клея:

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
	UserID   uuid.UUID `path:"id" validate:"uuid"`
	Username string    `json:"username" validate:"required,min=3,max=30" sanitize:"trim"`
	Email    string    `json:"email" validate:"required,email" sanitize:"lower"`
	Role     string    `query:"role,default=user" validate:"enum=user|admin|moderator"`
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

	// 2. Универсальные хэндлеры: чистые функции Go подключаются напрямую
	srv.Get("/health", func(ctx context.Context) (string, error) {
		return "OK", nil
	})

	srv.Post("/users/:id", func(ctx context.Context, req UpdateUserDTO) (*UserResponse, error) {
		return &UserResponse{
			ID:       req.UserID.String(),
			Username: req.Username,
			Email:    req.Email,
			Role:     req.Role,
		}, nil
	})

	// 3. Server-Sent Events (SSE)
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

## Универсальный роутер и архитектура без клея (Zero-Glue)

В `sein` встроен универсальный компилятор хэндлеров: стандартные HTTP-методы (`Get`, `Post`, `Patch`, `Delete`, `Put`) принимают любые сигнатуры чистых функций Go без необходимости писать методы-обёртки.

### 1. Поддерживаемые сигнатуры хэндлеров

| Задача | Сигнатура функции | Источник данных | Возвращаемое значение |
| :--- | :--- | :--- | :--- |
| **Действие** | `func(ctx context.Context) error` | Только контекст | `200 OK` при `nil` |
| **Запрос без параметров** | `func(ctx context.Context) (Res, error)` | Только контекст | JSON-ответ |
| **Запрос по ID** | `func(ctx context.Context, id ID) (Res, error)` | Параметр `:id` из URL (Snowflake, uint64, string, UUID) | JSON-ответ |
| **Действие по ID** | `func(ctx context.Context, id ID) error` | Параметр `:id` из URL | `200 OK` при `nil` |
| **Тело DTO** | `func(ctx context.Context, req DTO) (Res, error)` | DTO (JSON Body / Query / Headers) | JSON-ответ |
| **ID + Тело DTO** | `func(ctx context.Context, id ID, req DTO) (Res, error)` | `:id` из URL + JSON Body | JSON-ответ |
| **Прямой реквест** | `func(req *sein.Request) (Res, error)` | Доступ к низкоуровневому запросу | JSON-ответ |

### 2. Контроллеры без клея (Метод Promotion через Struct Embedding)

Поскольку сигнатуры `sein` совпадают с чистыми методами сервисов, вы можете встраивать сервисы в контроллеры/модули и монтировать методы напрямую:

```go
type BotController struct {
	*bots.Service // Автоматический промоушн методов Create, Get, Delete, Update и т.д.
}

func (c *BotController) Mount(g *sein.Group) {
	// Табличный маппинг доменных ошибок
	g.MapErrors(sein.Errors{
		database.ErrNotFound:       ErrBotNotFound,
		bots.ErrInvalidUserID:      ErrInvalidBotUserID,
		bots.ErrActiveBot:          ErrBotActiveCannotDelete,
		bots.ErrAlreadyLinkedAccount: ErrBotAlreadyLinkedAccount,
	})

	// Прямое подключение методов сервиса БЕЗ единой строчки прокладок
	g.Post("", c.Create)
	g.Get("/:id", c.Get)
	g.Patch("/:id", c.Update)       // Принимает (ctx, id Snowflake, payload UpdatePayload)
	g.Patch("/:id/type", c.SetType)
	g.Delete("/:id", c.Delete)
	g.Post("/:id/disconnect", c.Disconnect) // Принимает (ctx, id Snowflake) error
}
```

### 3. Табличный маппинг доменных ошибок (Table-Driven Error Mapping)

Декларативная трансляция внутренних ошибок базы/драйверов в типизированные HTTP-ошибки через `sein.Errors`:

```go
var (
	ErrUserNotFound = sein.NotFound("USER_NOT_FOUND", "Пользователь не найден")
	ErrBusyEmail    = sein.Conflict("EMAIL_EXISTS", "Email уже занят")
)

users.MapErrors(sein.Errors{
	database.ErrNotFound: ErrUserNotFound,
	users.ErrEmailTaken:  ErrBusyEmail,
})
```

## Структуры DTO и декларативная валидация

Все параметры запроса (путь, query, заголовки, куки, тело JSON) объявляются в единой структуре DTO с автоматической валидацией и санитизацией:

```go
type UpdateProfileDTO struct {
	// Источники данных протоколов
	UserID      uuid.UUID           `path:"user_id" validate:"uuid"`       // Параметр URL: /users/:user_id
	Search      string              `query:"q,default=all" sanitize:"trim,lower"` // Query-параметр: ?q=...
	Page        int                 `query:"page,default=1" validate:"positive"` // Query с числовым парсингом
	Limit       int                 `query:"limit,default=20" validate:"multiple_of=5,le=100"` // Ограничения
	Tags        []string            `query:"tags,sep=|"`                   // Срез с кастомным разделителем
	TraceID     string              `header:"X-Trace-ID" validate:"required"` // HTTP-заголовок
	SessionID   string              `cookie:"session_id" validate:"required"` // Cookie
	AuthToken   string              `auth:"bearer,required"`               // Authorization: Bearer <token>
	ClientIP    net.IP              `net:"ip"`                             // Вычисленный IP клиента
	Avatar      *sein.File          `file:"avatar,required"`               // Загруженный файл
	Gallery     []*sein.File        `files:"gallery"`                      // Набор загруженных файлов
	Password    sein.Secret[string] `json:"password" validate:"min=8"`     // Маскируется в логах и трейсах
	UserSession *Session            `ctx:""`                               // Типизированная сессия из контекста
	Bio         string              `json:"bio" validate:"max=500" sanitize:"squish"` // Схлопывание пробелов
}
```

<details>
<summary><b>📋 Справочник директив тегов DTO</b></summary>

| Категория | Директива | Описание | Пример |
| :--- | :--- | :--- | :--- |
| **Источники** | `path:"key"` | Параметр URL-пути (`/users/:id`) | `path:"id"` |
| | `query:"key"` | URL query-параметр (`?page=1`) | `query:"page,default=1"` |
| | `header:"key"` | HTTP-заголовок запроса | `header:"X-API-Key"` |
| | `cookie:"key"` | Значение HTTP-cookie | `cookie:"session_id"` |
| | `auth:"bearer"` | Извлечение `Authorization: Bearer <token>` | `auth:"bearer,required"` |
| | `form:"key"` | Поле формы (multipart или urlencoded) | `form:"title"` |
| | `file:"key"` | Одиночный файл формы (`*sein.File`) | `file:"avatar,required"` |
| | `files:"key"` | Набор файлов формы (`[]*sein.File`) | `files:"attachments"` |
| | `json:"key"` | Поле JSON-тела запроса | `json:"name"` |
| | `net:"ip"` | Вычисленный IP-клиента | `net:"ip"` |
| | `ctx:""` | Внедрение значения из контекста | `ctx:""` |
| **Санитизаторы (`sanitize:"..."`)** | `trim` | Удаление концевых пробелов | `sanitize:"trim"` |
| | `lower` | Приведение ASCII символов к нижнему регистру | `sanitize:"lower"` |
| | `upper` | Приведение ASCII символов к верхнему регистру | `sanitize:"upper"` |
| | `squish` | Схлопывание повторяющихся пробелов | `sanitize:"squish"` |
| | `digits_only` | Извлечение только цифр | `sanitize:"digits_only"` |
| **Валидаторы (`validate:"..."`)** | `required` | Поле обязательно и не может быть пустым | `validate:"required"` |
| | `min=N` / `max=N` | Границы длины строки или числового значения | `validate:"min=8,max=64"` |
| | `enum=a\|b\|c` | Проверка допустимого набора значений | `validate:"enum=asc\|desc"` |
| | `email` | Валидация стандартного формата email | `validate:"email"` |
| | `uuid` | Валидация формата UUID (RFC 4122 / RFC 9562) | `validate:"uuid"` |
| | `pattern=regex` | Соответствие регулярному выражению | `validate:"pattern=^[A-Z0-9]+$"` |

</details>

## Конфигурационные пресеты

Быстрая инициализация готовых наборов middleware для production:

```go
import "github.com/lemon4ksan/sein/preset"

// Production пресет включает: Panic Recovery, Security Headers, CORS, RequestID,
// Prometheus метрики (/system/metrics), Health Checks (/system/health), и ревизию (/system/version)
app := preset.Production(
	preset.WithPrometheus("/system/metrics"),
	preset.WithRevision("v1.2.0", "/system/version"),
	preset.WithCORS(preset.CORSConfig{
		AllowOrigins: []string{"https://example.com"},
	}),
)
```

## ⚡ Профиль производительности

### 1. Тест сетевой пропускной способности (TechEmpower Round 22, 32 ядра, 10GbE):

| Фреймворк | Язык / Среда | Сетевой движок | Пропускная способность | Относительно Gin |
| :--- | :---: | :---: | :---: | :---: |
| **Nest** | Node.js | HTTP parser | `105,064` reqs/s | 0.15x |
| **Express** | Node.js | HTTP parser | `113,117` reqs/s | 0.16x |
| **Fastify** | Node.js | fast-json | `415,600` reqs/s | 0.61x |
| **Spring** | Java | Netty / NIO | `506,087` reqs/s | 0.75x |
| **Gin** | Go | `net/http` | `676,019` reqs/s | 1.00x *(База)* |
| **Elysia** | Bun (C++/JS) | `uWebSockets` (C++) | `2,454,631` reqs/s | 3.63x |
| **Sein (Native H1 Net)** | **Go** | **Native H1 Engine** | **`~3,200,000+`**\* reqs/s | **4.73x** |
| **Sein (In-Memory Core)** | **Go** | **SIMD Fast H1 Core** | **`21,291,486`**\* reqs/s | **31.50x** |

> \* Локальные результаты. Не тестированы в реальных условиях.

### 2. Сравнение производительности TCP-сокетов ОС (Loopback)

Замеры в стеке TCP ОС с keep-alive соединениями (`net.Listen` + `net.Dial`):

```text
cpu: 12th Gen Intel(R) Core(TM) i5-12400F (12 Threads)
BenchmarkTechEmpower_RealTCPSocket_Sein-12       3,056 ns/op   178 B/op    7 allocs/op   (~330,000 req/s на сокет)
BenchmarkTechEmpower_RealTCPSocket_StdHTTP-12    4,716 ns/op  2,252 B/op   20 allocs/op   (~210,000 req/s на сокет)
```

## Лицензия

`sein` распространяется под лицензией [BSD-3-Clause](LICENSE).
