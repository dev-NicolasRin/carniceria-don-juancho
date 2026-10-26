# Documentación completa — API Carnicería Don Juancho

Esta guía explica **qué hace cada parte del proyecto, por qué está construida así y qué conceptos de Go, SQL y HTTP hay detrás**. Está pensada para alguien que está aprendiendo: no solo se documenta el “qué”, también el “por qué” y las alternativas que existen.

---

## Índice

1. [¿Qué es este proyecto?](#1-qué-es-este-proyecto)
2. [Cómo ejecutarlo](#2-cómo-ejecutarlo)
3. [Arquitectura por capas](#3-arquitectura-por-capas)
4. [Recorrido completo de una petición](#4-recorrido-completo-de-una-petición)
5. [El modelo de datos](#5-el-modelo-de-datos)
6. [Capa por capa, archivo por archivo](#6-capa-por-capa-archivo-por-archivo)
7. [Decisiones de diseño y por qué](#7-decisiones-de-diseño-y-por-qué)
8. [Transacciones explicadas a fondo](#8-transacciones-explicadas-a-fondo)
9. [Referencia de la API](#9-referencia-de-la-api)
10. [Manejo de errores y códigos HTTP](#10-manejo-de-errores-y-códigos-http)
11. [Tests](#11-tests)
12. [Glosario](#12-glosario)
13. [Limitaciones actuales y siguientes pasos](#13-limitaciones-actuales-y-siguientes-pasos)
14. [Comandos útiles](#14-comandos-útiles)

---

## 1. ¿Qué es este proyecto?

Una API REST escrita en Go que resuelve el reto de `Docs/Challenge.txt`:

- CRUD de **cortes de carne** (nombre, tipo de animal, precio por kilo, stock en kilos).
- Registro de **ventas**, descontando stock.
- Regla crítica: una venta y el descuento de stock ocurren en **una sola transacción**.
- Respuestas JSON con códigos HTTP correctos.
- Extras: filtros de cortes, reporte diario de ventas y tests con `httptest`.

### ¿Por qué Go?

- La librería estándar ya trae `net/http` y `database/sql`; no hace falta framework.
- El tipado fuerte y los errores explícitos obligan a pensar en los casos límite.
- Los binarios compilados son un solo archivo, fáciles de desplegar.

### ¿Por qué SQLite?

- No requiere servidor: la base es un archivo (`carniceria.db`).
- Ideal para aprender SQL sin montar infraestructura.
- `database/sql` es genérica: si mañana se cambia a PostgreSQL, la mayor parte del código de `store` se reutiliza cambiando el driver y los placeholders (`?` → `$1`).

---

## 2. Cómo ejecutarlo

### Dependencias

```bash
go mod download
```

El driver es `github.com/mattn/go-sqlite3`, que usa **CGO**. Por eso necesitas un compilador de C (`gcc` o `clang`) instalado. Si no lo tienes, la compilación falla con errores de `gcc`. La alternativa sin CGO es `modernc.org/sqlite`, pero cambia el nombre del driver a `"sqlite"` y puede ser más lenta.

### Arrancar

```bash
go run .
```

Salida esperada:

```
2026/10/26 00:08:48 API escuchando en :8080
```

Al arrancar, `main.go`:

1. Abre (o crea) `carniceria.db`.
2. Ejecuta `store.Migrar`, que crea las tablas si no existen.
3. Arma el grafo de dependencias.
4. Levanta el servidor HTTP en el puerto 8080.

### Variables que puedes tocar

Actualmente el puerto y la ruta de la base están **fijos en el código** (`main.go`). Una mejora sencilla es leerlos de variables de entorno:

```go
puerto := os.Getenv("PORT")       // por defecto "8080"
rutaDB := os.Getenv("DB_PATH")    // por defecto "carniceria.db"
```

Se dejó fijo para no añadir complejidad al reto, pero es el siguiente paso natural en un proyecto real.

---

## 3. Arquitectura por capas

```
                 HTTP
                  │
        ┌─────────▼─────────┐
        │    transport      │  handlers: leen HTTP, escriben JSON
        │  (corte_handler,  │  traducen errores de dominio a 400/404/409/500
        │   venta_handler)  │
        └─────────┬─────────┘
                  │ llama a
        ┌─────────▼─────────┐
        │      service      │  reglas de negocio: cantidad > 0, precio >= 0
        │                   │  NO conoce HTTP ni SQL
        └─────────┬─────────┘
                  │ llama a
        ┌─────────▼─────────┐
        │       store       │  SQL puro: SELECT/INSERT/UPDATE/DELETE
        │                   │  transacciones, errores sentinel
        └─────────┬─────────┘
                  │ database/sql
        ┌─────────▼─────────┐
        │      SQLite       │  carniceria.db
        └───────────────────┘

        model: structs compartidos por todas las capas (no dependen de nadie)
```

### Las tres reglas de la arquitectura

1. **Las dependencias apuntan hacia adentro**: `transport` conoce a `service`, `service` conoce a `store`, `store` conoce a SQLite. Nunca al revés. El `store` no sabe que existe HTTP; el `service` no sabe que existe SQLite.
2. **Cada capa tiene una responsabilidad**: HTTP, negocio y persistencia. Mezclarlas produce handlers gigantes difíciles de testear.
3. **Los modelos son transversales**: `model` no importa a nadie. Cualquier capa puede usarlo.

### ¿Por qué no hacerlo todo en `main.go`?

Porque un solo archivo con SQL, validaciones y HTTP sería:

- **Difícil de testear**: probar una regla exigiría levantar el servidor.
- **Difícil de cambiar**: tocar SQL podría romper HTTP.
- **Difícil de leer**: con 3 endpoints ya son cientos de líneas.

Separar tiene un costo (más archivos y algo de “pasamanos”), pero a partir de un proyecto pequeño-mediano el beneficio es enorme. Este proyecto tiene el tamaño justo para practicar esa separación.

---

## 4. Recorrido completo de una petición

Ejemplo: `POST /ventas` con `{"corte_id":1,"cantidad_kg":2.5}`.

### Paso 1 — `main.go` y el router

```go
mux.HandleFunc("/ventas", ventaHandler.HandleVentas)
```

El `ServeMux` recibe la petición y llama al handler registrado para esa ruta. El handler verifica el método (`POST`) y decodifica el body JSON en `crearVentaInput`.

- Si el JSON está mal formado → **400**.
- Si todo está bien, llama a `service.CrearVenta(1, 2.5)`.

### Paso 2 — `service.CrearVenta`

```go
if cantidadKg <= 0 { return nil, ErrCantidadInvalida }
return servicio.ventas.CreateVenta(&model.Venta{...})
```

Valida la regla de negocio que **no necesita la base**: cantidad positiva.

- Si falla → `ErrCantidadInvalida`, que el handler traduce a **400**.
- Si pasa, delega al store.

### Paso 3 — `store.CreateVenta` (el corazón)

```go
tx, _ := db.Begin()
defer tx.Rollback()

SELECT precio_kg, stock_kg FROM cortes WHERE id = 1
```

- Si el corte no existe → `sql.ErrNoRows` → handler responde **404**.
- Si `stock < cantidad` → `ErrStockInsuficiente` → **409** y `Rollback`.

Si hay stock:

```go
total = precio × cantidad
INSERT INTO ventas (...)
UPDATE cortes SET stock_kg = stock_kg - cantidad WHERE id = 1
tx.Commit()
```

Todo en una transacción: o se guardan ambas cosas o ninguna.

### Paso 4 — Respuesta

El handler recibe la venta con `ID` y `Total` ya calculados, y responde **201** con JSON. Si en cualquier paso hubo un error inesperado (BD caída), responde **500**.

### Resumen visual

```
POST /ventas
   │
   ▼
HandleVentas ── JSON inválido ──► 400
   │
   ▼
service.CrearVenta ── cantidad <= 0 ──► 400
   │
   ▼
store.CreateVenta
   ├── corte no existe ──► 404
   ├── stock insuficiente ──► 409 (rollback)
   ├── error de BD ──► 500 (rollback)
   └── OK ──► INSERT + UPDATE + COMMIT ──► 201
```

---

## 5. El modelo de datos

### Tabla `cortes`

| Columna | Tipo SQLite | En Go | Notas |
|---------|-------------|-------|-------|
| `id` | INTEGER PK AUTOINCREMENT | `int` | Lo genera la BD; el cliente no lo envía |
| `nombre` | TEXT NOT NULL | `string` | Ej. “Lomo fino” |
| `tipo_animal` | TEXT NOT NULL | `string` | res, cerdo, pollo, cordero (no validado aún) |
| `precio_kg` | REAL NOT NULL | `float64` | Precio por kilogramo |
| `stock_kg` | REAL NOT NULL | `float64` | Kilos disponibles |
| `fecha_creacion` | DATETIME DEFAULT CURRENT_TIMESTAMP | `time.Time` | UTC generado por SQLite |

### Tabla `ventas`

| Columna | Tipo SQLite | En Go | Notas |
|---------|-------------|-------|-------|
| `id` | INTEGER PK AUTOINCREMENT | `int` | |
| `corte_id` | INTEGER NOT NULL REFERENCES cortes(id) | `int` | FK; requiere `?_foreign_keys=on` |
| `cantidad_kg` | REAL NOT NULL | `float64` | Debe ser > 0 |
| `total` | REAL NOT NULL | `float64` | Precio histórico × cantidad |
| `created_at` | DATETIME DEFAULT CURRENT_TIMESTAMP | `time.Time` | Se inserta explícitamente en UTC |

### ¿Por qué `REAL` y no `INTEGER` para precios?

Porque un precio puede ser `45000.50`. `REAL` en SQLite es un número de coma flotante de 8 bytes (equivalente a `float64` de Go).

**Advertencia didáctica**: `float64` no puede representar exactamente todos los decimales (0.1 + 0.2 != 0.3). Para dinero real se suele:

- Guardar **centavos como entero** (`4500050`), o
- Usar un tipo decimal (librería `shopspring/decimal`).

Aquí se usa `float64` porque el reto pide decimales y los montos son simples. Lo importante es que sepas por qué en producción bancaria no se usa `float`.

### ¿Por qué `AUTOINCREMENT` y no solo `INTEGER PRIMARY KEY`?

En SQLite, `INTEGER PRIMARY KEY` ya es un alias del `rowid` y autoincrementa. `AUTOINCREMENT` añade una garantía extra: **los ids borrados no se reutilizan**. Esto importa con ventas históricas: si se borra el corte 5 y se crea otro, sin AUTOINCREMENT el nuevo podría recibir el id 5 y las ventas viejas “apuntarían” al corte equivocado.

### ¿Por qué el `total` se guarda y no se calcula al leer?

Porque el precio del corte puede cambiar con el tiempo. Si el reporte calculara `precio_actual × cantidad`, un cambio de precio alteraría retroactivamente el histórico. Guardar el total “congela” el monto del momento de la venta. Es un principio importante: **los datos históricos no deben depender de datos mutables**.

### Foreign keys en SQLite

SQLite **desactiva las FKs por defecto** por compatibilidad histórica. El DSN de `main.go` incluye `?_foreign_keys=on`; sin eso, `REFERENCES cortes(id)` sería solo un comentario y se podrían insertar ventas de cortes inexistentes. Es una trampa clásica.

### ¿Faltan índices?

Para el tamaño del reto no son necesarios, pero en un sistema real sí:

```sql
CREATE INDEX idx_cortes_tipo ON cortes(tipo_animal);
CREATE INDEX idx_ventas_corte ON ventas(corte_id);
CREATE INDEX idx_ventas_fecha ON ventas(created_at);
```

El de `ventas(created_at)` aceleraría el reporte diario; el de `ventas(corte_id)` aceleraría el JOIN. Se omitieron para no desviar el foco, pero conviene saberlo.

---

## 6. Capa por capa, archivo por archivo

### `main.go` — composition root

Es el único lugar donde se construyen las dependencias:

```go
db := sql.Open(...)          // infraestructura
store.New(db)                // repositorio de cortes
store.NewVentas(db)          // repositorio de ventas
service.New(...)             // negocio
transport.New(...)           // HTTP
http.ListenAndServe(...)     // servidor
```

Este patrón se llama **inyección de dependencias manual** y el punto de ensamblaje se llama **composition root**. Ninguna capa crea sus propias dependencias; todas las reciben. Ventajas:

- Los tests pueden inyectar una BD en memoria.
- Cambiar la implementación de un repositorio es cambiar una línea en `main`.
- El grafo de dependencias es explícito y visible en un solo archivo.

Detalles:

- `sql.Open` **no conecta** todavía; solo valida el driver y prepara el pool. `db.Ping()` fuerza la primera conexión para fallar rápido.
- `defer db.Close()` cierra el pool al salir.
- El blank import `_ "github.com/mattn/go-sqlite3"` registra el driver en `database/sql` sin usar sus símbolos.
- `/cortes/` (con barra) es un patrón de subárbol: captura `/cortes/1`, `/cortes/2`, etc. `/cortes` (sin barra) es exacto.

### `internal/model` — datos puros

- `corte.go`, `venta.go`, `reporte.go`.
- Structs con tags `json:"..."` que definen el contrato JSON.
- No tienen lógica ni dependencias de infraestructura.
- `Venta.NombreCorte` es un campo **derivado**: no existe en la tabla; se rellena con un JOIN.

### `internal/store` — persistencia

- `Store` y `VentasStore` son **interfaces**: definen qué operaciones existen, no cómo se hacen.
- `store` y `ventasStore` son las implementaciones concretas (no exportadas).
- `New(db)` y `NewVentas(db)` reciben el pool y devuelven la interfaz.
- Todo el SQL vive aquí; ninguna otra capa escribe consultas.
- `esquema.go` centraliza el DDL para que los tests usen el mismo esquema que producción.

Conceptos usados:

- **Placeholders `?`**: separan estructura SQL de datos y evitan SQL injection.
- **`rows.Close()` con `defer`**: libera el cursor y devuelve la conexión al pool.
- **`rows.Err()`**: detecta errores ocurridos a mitad de la iteración (si no, se devuelve una lista incompleta en silencio).
- **`sql.ErrNoRows`**: error estándar cuando una consulta no devuelve filas.
- **Errores sentinel** (`ErrStockInsuficiente`): valores de error exportados que se comparan con `errors.Is`.

### `internal/service` — reglas de negocio

- Recibe los repositorios por constructor.
- Valida reglas que no dependen de la base: `cantidad > 0`, `precio >= 0`, `stock >= 0`.
- Devuelve errores de dominio (`ErrCantidadInvalida`, `ErrPrecioInvalido`) **sin códigos HTTP**.
- No importa `net/http` ni `database/sql` (buena señal de desacoplamiento).

### `internal/transport` — HTTP

- `HandleCortes`: GET con filtros y POST.
- `HandleCorteByID`: GET, PUT y DELETE.
- `HandleVentas`: GET y POST.
- `HandleVentasDelDia`: GET del reporte.
- `responderJSON`: helper que fija `Content-Type` antes de `WriteHeader`.

Conceptos usados:

- **`http.ResponseWriter` / `*http.Request`**: contrato de un handler.
- **`json.NewDecoder(r.Body).Decode(&v)`**: deserializa el body como stream.
- **`json.NewEncoder(w).Encode(v)`**: serializa directo a la respuesta.
- **`http.Error`**: escribe un mensaje de texto plano y un status. Después de llamarlo, **hay que hacer `return`** o el handler sigue ejecutándose y puede intentar escribir una segunda respuesta.
- **Orden de headers**: `Header().Set` debe ir **antes** de `WriteHeader`; después ya no se envían.
- **DTO de entrada** (`crearVentaInput`): expone solo los campos que el cliente puede controlar.

### `internal/transport/transport_test.go` — tests de integración

- Levanta el router real con una BD SQLite `:memory:`.
- `db.SetMaxOpenConns(1)` es imprescindible: cada conexión nueva a `:memory:` crea una base distinta.
- `t.Helper()` mejora los reportes de error.
- `t.Cleanup` cierra la BD al terminar, incluso si el test falla.
- `httptest.NewRequest` + `httptest.NewRecorder` ejecutan handlers sin abrir un puerto.

---

## 7. Decisiones de diseño y por qué

### ¿Por qué `database/sql` y no un ORM (GORM, ent)?

- `database/sql` obliga a aprender SQL de verdad, que es el objetivo del reto.
- No hay “magia”: lo que se ejecuta es exactamente la consulta escrita.
- Los ORM son útiles en proyectos grandes, pero esconden el costo de las consultas y complican el diagnóstico.

### ¿Por qué interfaces en el store?

- Permiten sustituir la implementación por un fake en tests unitarios del servicio.
- Documentan el contrato mínimo.
- Si mañana se migra a PostgreSQL, `service` y `transport` no cambian.

Se mantienen **pequeñas**: una interfaz de 10 métodos es difícil de implementar y de mockear. Las de este proyecto tienen 3 y 5 métodos.

### ¿Por qué punteros en `*model.Corte`?

- Evita copiar el struct en cada iteración.
- Permite representar “sin resultado” como `nil`.
- `store.Create` necesita modificar `corte.ID`; con un valor no podría reflejarse en el llamador (aunque aquí el valor de retorno lo soluciona).

### ¿Por qué errores sentinel y `errors.Is`?

```go
var ErrStockInsuficiente = errors.New("...")
...
if errors.Is(err, store.ErrStockInsuficiente) { /* 409 */ }
```

- Comparar mensajes (`err.Error() == "..."`) es frágil: cambiar una tilde rompe la lógica.
- `errors.Is` recorre la cadena de errores, así que funciona aunque el error se envuelva con `fmt.Errorf("...: %w", err)`.
- Es el mecanismo idiomático de Go, que no tiene excepciones.

### ¿Por qué el total se calcula en el servidor?

El cliente solo envía `corte_id` y `cantidad_kg`. Si enviara `total`, podría pagar 1 por 100 kg. **Nunca confíes en datos calculables enviados por el cliente.**

### ¿Por qué se relee la fila después de INSERT/UPDATE?

Para devolver los valores reales generados por la BD (`id`, `fecha_creacion`). Devolver el struct de entrada produciría fechas en cero y datos desactualizados. Alternativa moderna: `INSERT ... RETURNING *` (SQLite 3.35+), que ahorra la segunda consulta.

### ¿Por qué UTC?

- SQLite genera `CURRENT_TIMESTAMP` en UTC.
- Si en Go se usara `time.Now()` local, habría dos zonas mezcladas y el reporte “del día” podría contar ventas en el día equivocado.
- Guardar en UTC y convertir en la presentación es la práctica recomendada en APIs.

### ¿Por qué `responderJSON` centralizado?

Porque el orden `Content-Type` → `WriteHeader` → `Encode` es fácil de equivocar. Un helper garantiza consistencia. Antes de este helper había un bug real: en `POST /cortes` se llamaba a `WriteHeader` antes de poner el `Content-Type`, por lo que el header se perdía.

### ¿Por qué filtros con SQL dinámico y argumentos?

- Se evita escribir 4 variantes del SELECT (sin filtro, tipo, stock, ambos).
- Los valores siempre van como `?`, nunca concatenados: **SQL injection**.
- `strings.Join(condiciones, " AND ")` solo concatena fragmentos **controlados por el programador**, no datos del usuario.

### ¿Por qué los tests usan SQLite real y no mocks?

- Prueban el camino completo, incluidas las consultas SQL y las transacciones.
- Un mock del store no detectaría un error de SQL como `stock kg` (bug real que existió en este proyecto).
- Son igual de rápidos gracias a `:memory:`.
- Los mocks se reservan para tests unitarios de reglas muy aisladas.

---

## 8. Transacciones explicadas a fondo

### El problema

Registrar una venta requiere dos escrituras:

1. `INSERT INTO ventas ...`
2. `UPDATE cortes SET stock_kg = stock_kg - cantidad ...`

Si se ejecutan por separado y falla la segunda, queda una venta cobrada sin descontar stock. El inventario queda corrupto y no hay forma automática de saber qué pasó.

### La solución

```go
tx, err := db.Begin()
defer tx.Rollback()

// ... SELECT, validaciones, INSERT, UPDATE ...

if err := tx.Commit(); err != nil { ... }
```

- `Begin` inicia la transacción.
- `Commit` confirma **todo** de forma atómica.
- `Rollback` deshace **todo**.
- `defer tx.Rollback()` se ejecuta siempre al salir de la función. Si ya se hizo `Commit`, devuelve `sql.ErrTxDone` y no pasa nada. Este patrón garantiza que ningún `return` de error olvide revertir.

### ACID

| Propiedad | Qué significa aquí |
|-----------|--------------------|
| **Atomicidad** | Venta y descuento se guardan juntos o no se guarda ninguno |
| **Consistencia** | Nunca queda stock negativo ni venta sin descuento |
| **Aislamiento** | Otras conexiones no ven el estado intermedio (venta insertada pero stock sin descontar) |
| **Durabilidad** | Tras el `Commit`, el dato sobrevive a un reinicio |

### Condiciones de carrera

El precio y el stock se leen **dentro** de la transacción. Si se leyeran antes (`db.QueryRow` en vez de `tx.QueryRow`), otro proceso podría cambiar el precio entre la lectura y el descuento, y el total se calcularía con un precio viejo.

El UPDATE usa `stock_kg = stock_kg - ?` en SQL, no `stock = stockLeido - cantidad` en Go: así el cálculo lo hace el motor y no se pierde una resta si dos ventas ocurren casi a la vez.

### Limitación real de SQLite

SQLite permite **un solo escritor a la vez**. Con muchas ventas concurrentes, algunas transacciones pueden recibir `SQLITE_BUSY`. Para el reto no es un problema; en producción se resolvería con:

- `?_busy_timeout=5000` en el DSN, o
- usar PostgreSQL/MySQL.

### ¿Por qué la validación de stock no está en `service`?

Porque necesita leer la BD y descontar en la **misma** transacción. Si se validara en `service`, entre la comprobación y el descuento habría una ventana para que otra venta agotara el stock (TOCTOU: time-of-check to time-of-use).

---

## 9. Referencia de la API

URL base: `http://localhost:8080`

### `GET /cortes`

Lista cortes. Filtros opcionales combinables:

| Query | Ejemplo | Efecto |
|-------|---------|--------|
| `tipo` | `?tipo=res` | Solo cortes de ese animal |
| `stock_bajo` | `?stock_bajo=true` | Solo cortes con stock < 5 kg |

**200 OK**

```json
[
  {
    "id": 1,
    "nombre": "Lomo fino",
    "tipo_animal": "res",
    "precio_kg": 45000,
    "stock_kg": 10,
    "fecha_creacion": "2026-10-26T05:23:17Z"
  }
]
```

### `POST /cortes`

**Body**

```json
{"nombre":"Lomo fino","tipo_animal":"res","precio_kg":45000,"stock_kg":10}
```

- **201 Created** con el corte completo (incluye `id` y `fecha_creacion`).
- **400 Bad Request** si el JSON es inválido o el precio/stock es negativo.
- **405** si el método no es GET/POST.

### `GET /cortes/{id}`

- **200 OK** con el corte.
- **400** si el id no es numérico.
- **404** si no existe.
- **500** ante un error inesperado de BD.

### `PUT /cortes/{id}`

**Body** (solo se actualizan estos dos campos):

```json
{"precio_kg":50000,"stock_kg":8}
```

- **200 OK** con los datos reales releídos de la BD.
- **400** si el JSON es inválido.
- **500** si el id no existe (mejora pendiente: devolver 404).

### `DELETE /cortes/{id}`

- **204 No Content**.
- **500** si hay ventas asociadas (la FK lo impide) o ante error de BD. Mejora pendiente: **409 Conflict** con un mensaje claro.

### `POST /ventas`

**Body**

```json
{"corte_id":1,"cantidad_kg":2.5}
```

- **201 Created**:

```json
{
  "id": 1,
  "corte_id": 1,
  "cantidad_kg": 2.5,
  "total": 112500,
  "fecha": "2026-10-26T05:23:17.141228228Z"
}
```

- **400** si `cantidad_kg <= 0` o el JSON es inválido.
- **404** si el corte no existe.
- **409 Conflict** si no hay stock suficiente.
- **500** ante error inesperado.

### `GET /ventas`

**200 OK** con el histórico; incluye `corte_nombre` gracias al JOIN:

```json
[
  {
    "id": 1,
    "corte_id": 1,
    "corte_nombre": "Lomo fino",
    "cantidad_kg": 2.5,
    "total": 112500,
    "fecha": "2026-10-26T05:23:17Z"
  }
]
```

### `GET /reportes/ventas-del-dia`

**200 OK**

```json
{"fecha":"2026-10-26","cantidad_ventas":2,"total":135000}
```

- **405** si el método no es GET.

---

## 10. Manejo de errores y códigos HTTP

| Código | Cuándo se usa en este proyecto |
|--------|--------------------------------|
| **200 OK** | GET/PUT exitosos |
| **201 Created** | POST que crea un recurso |
| **204 No Content** | DELETE exitoso |
| **400 Bad Request** | JSON inválido, id no numérico, cantidad <= 0, precio/stock negativo |
| **404 Not Found** | El corte solicitado/comprado no existe |
| **405 Method Not Allowed** | Verbo no soportado en la ruta |
| **409 Conflict** | Stock insuficiente (conflicto con el estado del recurso) |
| **500 Internal Server Error** | Fallo de BD, transacción fallida, error no clasificado |

### ¿Por qué no usar 400 para todo?

Porque el código HTTP es parte del contrato de la API: los clientes (apps, otros servicios) deciden qué hacer según él. Un `400` significa “tu petición está mal”; un `409` significa “tu petición está bien, pero el estado actual no lo permite”; un `500` significa “el error es nuestro”. Usar 400 para stock insuficiente haría que el cliente no pudiera distinguir un error de datos de un conflicto de negocio.

### El patrón `if err != nil { http.Error(...); return }`

El `return` es obligatorio. Sin él:

- Se sigue ejecutando el resto del handler.
- Se intenta escribir una segunda respuesta → `http: superfluous response.WriteHeader call`.
- Puede escribirse JSON sobre un body que ya contenía el texto del error.

Era un bug real en el código original (faltaban varios `return`).

---

## 11. Tests

### Ejecutar

```bash
go test ./...           # todos
go test -v ./internal/transport   # detallado
go test -run TestCrearVenta ./internal/transport  # uno solo
```

### Cobertura actual (8 tests)

| Test | Qué verifica |
|------|--------------|
| `TestCrearVentaExitosaDescuentaStock` | 201, total correcto, stock 10 → 7.5 |
| `TestCrearVentaSinStockResponde409` | 409 y rollback completo (sin venta ni cambio de stock) |
| `TestCrearVentaCorteInexistenteResponde404` | 404 |
| `TestCrearVentaCantidadInvalidaResponde400` | 400 |
| `TestFiltroCortesPorTipo` | `?tipo=cerdo` devuelve solo cerdo |
| `TestFiltroStockBajo` | `?stock_bajo=true` devuelve solo stock < 5 |
| `TestObtenerCorteInexistenteResponde404` | 404 en GET por id |
| `TestReporteVentasDelDia` | 2 ventas, total 135000 |

### ¿Qué es `:memory:`?

Una base SQLite que vive en RAM y desaparece al cerrar la conexión. Ventajas:

- No deja archivos ni contamina la BD de desarrollo.
- Cada test empieza limpio (aislamiento total).
- Es muy rápida.

Trampa: cada **conexión** nueva crea una base distinta. `database/sql` usa un pool, así que sin `db.SetMaxOpenConns(1)` el test podría escribir en una base y leer de otra.

### ¿Qué es `httptest`?

Paquete de la librería estándar para probar HTTP sin red:

- `httptest.NewRequest(...)` construye una petición.
- `httptest.NewRecorder()` captura status, headers y body.
- `mux.ServeHTTP(rec, req)` ejecuta el handler de forma síncrona.

### Arrange–Act–Assert

Cada test sigue el patrón:

1. **Arrange**: `nuevoRouterPrueba` + `sembrarCorte`.
2. **Act**: `peticion(...)`.
3. **Assert**: comprobar `rec.Code` y el estado en la BD.

---

## 12. Glosario

- **API REST**: interfaz HTTP donde las URLs representan recursos y los verbos (GET, POST, PUT, DELETE) acciones.
- **Handler**: función que recibe una petición HTTP y escribe una respuesta.
- **DTO**: objeto para transportar datos entre capas, con solo los campos necesarios.
- **Repository**: capa que abstrae el acceso a datos.
- **Inyección de dependencias**: recibir las dependencias desde fuera en vez de crearlas dentro.
- **Composition root**: lugar donde se construyen y conectan todas las dependencias (`main`).
- **Sentinel error**: error exportado que representa un caso concreto y se compara con `errors.Is`.
- **Transacción**: unidad atómica de trabajo (todo o nada).
- **Rollback**: deshacer los cambios de una transacción.
- **Commit**: confirmar definitivamente los cambios.
- **Pool de conexiones**: conjunto reutilizable de conexiones a la BD; `*sql.DB` lo gestiona.
- **FK**: clave foránea; referencia a la PK de otra tabla.
- **SQL injection**: ataque que inserta SQL malicioso en datos del usuario; se evita con placeholders.
- **JOIN**: combinar filas de dos tablas usando una condición.
- **COALESCE**: devuelve el primer valor no nulo; aquí evita `SUM` = NULL.
- **UTC**: zona horaria de referencia; evita ambigüedades entre zonas.
- **TOCTOU**: carrera entre comprobar y usar un valor.
- **CGO**: mecanismo para llamar código C desde Go; lo requiere `mattn/go-sqlite3`.

---

## 13. Limitaciones actuales y siguientes pasos

### Mejoras de correctitud

1. **`PUT` y `DELETE` de un corte inexistente** devuelven 500/204. Lo correcto sería 404. Se puede detectar con `RowsAffected()` o con `sql.ErrNoRows` después del `GetByID`.
2. **Borrar un corte con ventas** devuelve 500 por la FK. Debería ser **409 Conflict** con un mensaje claro, o usar borrado lógico.
3. **`tipo_animal` no se valida** contra la lista res/cerdo/pollo/cordero. Cualquier string entra.
4. **Cantidades y precios extremos** (NaN, Inf, negativos en el PUT) no se validan en todos los caminos.
5. **El reporte usa UTC**: si el negocio está en GMT-5, las ventas después de las 19:00 locales caen en el día siguiente UTC. Se podría recibir una zona horaria o calcular el día en la zona del negocio.

### Mejoras de robustez

6. **Graceful shutdown**: capturar SIGINT/SIGTERM y llamar a `srv.Shutdown(ctx)` para terminar las peticiones en curso.
7. **Timeouts del servidor**: `ReadTimeout`, `WriteTimeout`, `IdleTimeout`.
8. **Logging estructurado**: `log/slog` con request id, método, ruta y duración.
9. **Recuperación de panics** con un middleware que responda 500.
10. **CORS** si la API se consume desde un navegador en otro origen.
11. **Paginación** en `/ventas` y `/cortes` para listas grandes.
12. **Validación en el PUT**: hoy permite dejar `stock` y `precio` en cero implícitamente si el body los omite.

### Mejoras de arquitectura

13. **Migraciones versionadas** (`golang-migrate`, `goose`) en vez de `CREATE TABLE IF NOT EXISTS`.
14. **Tests unitarios del service** con mocks de `Store`/`VentasStore`.
15. **Tests de la capa store** con casos límite de SQL.
16. **Separar `main` en un paquete** `server` que exponga `NewServer(db) http.Handler` para testear el wiring completo.
17. **`DELETE /ventas/{id}`** con reversión de stock (si el negocio lo necesita).
18. **Autenticación** (JWT o API key) y autorización por roles.
19. **Dockerfile multi-stage** con CGO y una imagen final mínima.

### Ejercicios sugeridos para aprender

- Añadir el filtro `?precio_max=`.
- Implementar `GET /ventas?desde=...&hasta=...`.
- Añadir un endpoint `PATCH /cortes/{id}` que actualice solo un campo.
- Escribir un middleware que mida cuánto tarda cada petición.
- Cambiar el driver a `modernc.org/sqlite` y comparar rendimiento y facilidad de compilación.
- Escribir el mismo proyecto con PostgreSQL y notar qué cambia en `store`.

---

## 14. Comandos útiles

```bash
# Formatear todo el código (Go lo exige para un estilo consistente)
gofmt -w .

# Compilar sin ejecutar
go build ./...

# Análisis estático
go vet ./...

# Tests
go test ./...
go test -v ./internal/transport
go test -run TestCrearVentaSinStock -v ./internal/transport

# Ejecutar
go run .

# Ver dependencias
go mod tidy
go list -m all

# Probar endpoints
curl localhost:8080/cortes
curl -X POST localhost:8080/cortes -H 'Content-Type: application/json' \
  -d '{"nombre":"Lomo fino","tipo_animal":"res","precio_kg":45000,"stock_kg":10}'
curl -X POST localhost:8080/ventas -H 'Content-Type: application/json' \
  -d '{"corte_id":1,"cantidad_kg":2.5}'
curl 'localhost:8080/cortes?tipo=res&stock_bajo=true'
curl localhost:8080/reportes/ventas-del-dia
```

---

## Para terminar

La idea central del proyecto es que **cada capa tenga una sola razón para cambiar**:

- Si cambia el formato de la respuesta → `transport`.
- Si cambia una regla del negocio → `service`.
- Si cambia el esquema o el motor de BD → `store`.
- Si cambia la forma de los datos → `model`.

Esa separación es más valiosa que cualquier framework. Si entiendes por qué existe cada archivo y qué pasaría si se eliminara, ya entendiste la arquitectura.
