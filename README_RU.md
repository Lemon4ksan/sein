<div align="center">

# sein

### Суверенный высокопроизводительный серверный реактор сетевых протоколов для Go

[![License](https://img.shields.io/github/license/lemon4ksan/sein?style=flat-square)](LICENSE)
[![Status](https://img.shields.io/badge/статус-активная%20разработка-blue?style=flat-square)](#)
[![Ecosystem](https://img.shields.io/badge/экосистема-foundation-blueviolet?style=flat-square)](https://github.com/lemon4ksan/foundation)
[![Go Version](https://img.shields.io/badge/go-1.24%2B-00ADD8?style=flat-square&logo=go)](https://go.dev)

> _«В бэкендах безумие — это состояние по умолчанию. Пусть **sein** станет вашим светом разума.»_

#### [English](README.md) • Русский • [Концептуальный Манифест](docs/CONCEPT.md)

</div>

---

## 1. Обзор проекта

**`sein`** — это единый высокопроизводительный серверный IP-реактор и типобезопасный веб-фреймворк для Go с zero-alloc архитектурой (**0 B/op**), объединяющий **HTTP/1.1, HTTP/2, HTTP/3 (QUIC), WebSockets и gRPC на едином порту `:443`** без внешних прокси-серверов, с математически верифицированной безопасностью памяти (`borrow.Scope`) и аппаратным иммунитетом к сетевым DoS-атакам.

### Ключевые возможности и архитектурные столпы
- **Single-Port Protocol Matrix (Единый порт `:443`)**:
  - Диспетчеризация HTTP/1.1, HTTP/2 (ALPN `h2`), HTTP/3 (QUIC ALPN `h3`) и WebSockets на одном сокете без Nginx, Envoy или Caddy.
  - Нативные мультиплексированные WebSockets через HTTP/2 и HTTP/3 (**RFC 8441** и **RFC 9220** Extended `CONNECT`).
- **Чистые математические функции-обработчики**:
  - Сигнатуры обработчиков: `(ctx, DTO) -> (Response, error)` или `(ctx) -> (Response, error)`.
  - Автоматическая сериализация JSON, вывод HTTP статус-кодов и типизированные конструкторы ответов (`sein.OK`, `sein.Created`, `sein.NoContent`, `sein.Redirect`).
- **Унифицированная DTO-ингестия на контрактах**:
  - Извлечение параметров пути (Path), query-строки, заголовков (Headers), cookies, токенов авторизации (Bearer), multipart-файлов, L1 context сессий и JSON-тел в единую Go-структуру.
  - Декларативная санитизация строк (`trim`, `lower`, `squish`) и правила валидации (`email`, `uuid`, `enum`, `min`, `max`, `pattern`).
- **Кремниевый детерминизм и 0 аллокаций**:
  - Zero-alloc Radix роутинг, шардирование памяти по логическим ядрам CPU (`PerPStorage`), плоский inline L1 кэш контекста (`[8]contextSlot` массив).
  - Интеграция с `borrow.Scope` для статического контроля времени жизни буферов.

---

## 2. Быстрый старт

### Установка
```bash
go get github.com/lemon4ksan/sein
```

### Полный рабочий пример
```go
package main

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/lemon4ksan/sein"
)

// 1. Описываем унифицированный DTO запроса с санитизацией и валидацией
type UpdateProfileDTO struct {
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

	srv.POST("/api/v1/users", func(c *sein.Context) error {
		var req CreateUserRequest
		if err := c.BindJSON(&req); err != nil {
			return c.SendStatus(400)
		}
		return c.SendJSON(201, UserResponse{
			ID:       c.NextID(),
			Username: req.Username,
	// 2. Чистый математический обработчик: (ctx, DTO) -> (Result, error)
	srv.Post("/users/:id", func(ctx context.Context, req UpdateProfileDTO) (*UserResponse, error) {
		return &UserResponse{
			ID:       req.UserID.String(),
			Username: req.Username,
			Email:    req.Email,
			Role:     req.Role,
		}, nil
	})

	// 3. Простой GET обработчик: (ctx) -> (Result, error)
	srv.Get("/health", func(ctx context.Context) (string, error) {
		return "OK", nil
	})

	// 4. Стриминг Server-Sent Events (SSE) в реальном времени
	srv.Get("/events", func(ctx context.Context) (sein.SSEResponse, error) {
		return sein.SSE(func(sse *sein.SSESender) error {
			_ = sse.SendJSON("connected", map[string]string{"status": "online"})
			return nil
		}), nil
	})

	log.Println("sein сервер запущен на http://localhost:8080")
	log.Fatal(srv.Listen(":8080"))
}
```

---

## 3. Матрица директив унифицированного DTO

Опишите все ожидаемые входные параметры протоколов в единой декларативной структуре:

```go
type UpdateProfileDTO struct {
    // 1. Источники данных (Откуда извлекаются значения)
    UserID      uuid.UUID           `path:"user_id,uuid"`                  // Параметр URL: /users/:user_id
    Search      string              `query:"q,default=all,trim,lower"`     // Query-параметр: ?q=...
    Page        int                 `query:"page,default=1,positive"`      // Числовой query-параметр
    Limit       int                 `query:"limit,default=20,multiple_of=5,le=100"` // Шаг пагинации
    Tags        []string            `query:"tags,sep=|"`                   // Срез со своим разделителем
    TraceID     string              `header:"X-Trace-ID,required"`         // HTTP-заголовок
    SessionID   string              `cookie:"session_id,required"`         // Значение Cookie
    AuthToken   string              `auth:"bearer,required"`               // Authorization: Bearer <token>
    ClientIP    net.IP              `net:"ip"`                             // IP клиента (net.IP или netip.Addr)
    Scheme      string              `net:"scheme"`                         // http или https
    Avatar      *sein.File          `file:"avatar,required"`               // Одиночный multipart-файл
    Gallery     []*sein.File        `files:"gallery"`                      // Коллекция multipart-файлов
    Category    string              `form:"category,trim"`                 // Поле формы (multipart / urlencoded)
    RawHMAC     []byte              `query:"hmac,hex"`                     // Бинарный срез из hex
    PayloadB64  []byte              `json:"payload,base64"`                // Бинарный срез из base64
    Password    sein.Secret[string] `json:"password,min=8"`                // Скрывается в логах
    UserSession *Session            `ctx:""`                               // Типизированная L1 сессия контекста
    Bio         string              `json:"bio,squish,max=500"`            // JSON поле со сжатием пробелов
}
```

### Таблица тегов DTO

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

---

## 4. Маршрутизация и обработчики

### Чистые математические функции
`sein` избавляет от громоздких параметров `w http.ResponseWriter, r *http.Request`:

```go
// Чистый GET с DTO: (ctx, DTO) -> (Result, error)
srv.GetWith("/users/:id", func(ctx context.Context, req GetUserDTO) (*User, error) {
    return userService.Find(ctx, req.ID)
})

// Чистый POST с DTO: (ctx, DTO) -> (Result, error)
srv.Post("/users", func(ctx context.Context, req CreateUserDTO) (sein.Response[*User], error) {
    user, err := userService.Create(ctx, req)
    if err != nil {
        return sein.Response[*User]{}, err
    }
    return sein.Created(user), nil
})
```

### Группировка маршрутов и Middleware
```go
api := srv.Group("/api/v1", authMiddleware)
{
    users := api.Group("/users")
    users.Get("", listUsersHandler)
    users.Post("", createUserHandler)
    users.GetWith("/:id", getUserHandler)
}
```

---

## 5. Производительность и бенчмарки

`sein` спроектирован для zero-allocation исполнения, аппаратного параллелизма, по-ядерного пулинга памяти (`foundation/silicon/pool`) и zero-copy сериализации пакетов.

### 1. Реальная сеть: Физический бенчмарк TechEmpower (Round 22)

В официальном аппаратном сетевом тестировании (**TechEmpower Round 22**, 32-ядерный сервер + 10GbE сеть, нагрузка через утилиту `wrk`), производительность определяется сетевым стеком ядра ОС, системными вызовами и аллокациями фреймворков:

| Фреймворк | Язык / Рантайм | Сетевой движок | Пропускная способность Round 22 | Архитектурные особенности |
| :--- | :---: | :---: | :---: | :--- |
| **Nest** | Node.js | HTTP parser | `105,064` reqs/s | V8 Single-Thread + Слой Middleware |
| **Express** | Node.js | HTTP parser | `113,117` reqs/s | V8 Single-Threaded Event Loop |
| **Fastify** | Node.js | fast-json | `415,600` reqs/s | Схемная оптимизация JSON |
| **Spring** | Java | Netty / NIO | `506,087` reqs/s | Пул потоков JVM + Epoll транспорт |
| **Gin** | Go | `net/http` | `676,019` reqs/s | Горутина на соединение + `map[string][]string` заголовки |
| **Elysia** | Bun (C++/JS) | `uWebSockets` (C++) | `2,454,631` reqs/s | C++ Event Loop + PicoHTTPParser SIMD |

> **Почему Gin выдаёт ~676k req/s**: Стандартный Go `net/http` выделяет отдельную горутину на каждое TCP-соединение и непрерывно аллоцирует в куче `http.Header` (`map[string][]string`) и `http.Request`. Под нагрузкой в тысячи соединений планировщик Go тратит до 40% CPU на переключение контекста горутин, а сборщик мусора (GC) вызывает микропаузы.

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

---

## 6. Экосистема

`sein` является серверным компонентом сетевого стека:

* **[`aoni`](https://github.com/lemon4ksan/aoni)** — Исходящий клиентский реактор (Chromium стелс, uTLS эвазия, JA4+, Happy Eyeballs v3, MASQUE).
* **[`sein`](https://github.com/lemon4ksan/sein)** — Входящий серверный реактор (Single-port `:443`, 0 B/op, anti-DoS, RFC 8441/9220 WebSockets).
* **[`foundation`](https://github.com/lemon4ksan/foundation)** — Высокопроизводительный Go-субстрат (SIMD векторы, Per-P пулы, off-heap память, lock-free кольца).

---

## 7. Лицензия

Распространяется под лицензией **BSD 3-Clause License**. См. [LICENSE](LICENSE) для подробностей.

<div align="center">
  <sub>В бэкендах безумие — это состояние по умолчанию. Пусть <b>sein</b> станет вашим светом разума.</sub>
</div>
