# 🥩 Carnicería Don Juancho — API de inventario

API REST en **Go + SQLite** para gestionar los cortes de carne de una carnicería y registrar sus ventas descontando stock de forma transaccional.

> 📚 **¿Estás aprendiendo Go?** La guía completa con el *por qué* de cada decisión está en [`Docs/DOCUMENTACION.md`](Docs/DOCUMENTACION.md).

## Requisitos

- Go 1.22 o superior (el proyecto usa `go 1.26` en `go.mod`)
- `gcc` disponible, porque el driver `mattn/go-sqlite3` usa CGO

## Ejecutar

```bash
go run .
# API escuchando en :8080
```

La base `carniceria.db` se crea automáticamente en el directorio actual (está en `.gitignore`).

## Tests

```bash
go test ./...
```

## Endpoints

| Método | Ruta | Descripción | Éxito |
|--------|------|-------------|-------|
| `GET` | `/cortes` | Lista cortes. Filtros opcionales `?tipo=res` y `?stock_bajo=true` | `200` |
| `POST` | `/cortes` | Crea un corte | `201` |
| `GET` | `/cortes/{id}` | Obtiene un corte | `200` |
| `PUT` | `/cortes/{id}` | Actualiza precio y stock | `200` |
| `DELETE` | `/cortes/{id}` | Elimina un corte | `204` |
| `POST` | `/ventas` | Registra una venta y descuenta stock | `201` |
| `GET` | `/ventas` | Lista el histórico de ventas | `200` |
| `GET` | `/reportes/ventas-del-dia` | Total y cantidad vendidos hoy | `200` |

## Ejemplos rápidos

```bash
# Crear un corte
curl -X POST localhost:8080/cortes \
  -H 'Content-Type: application/json' \
  -d '{"nombre":"Lomo fino","tipo_animal":"res","precio_kg":45000,"stock_kg":10}'

# Listar cortes de res con stock bajo
curl 'localhost:8080/cortes?tipo=res&stock_bajo=true'

# Registrar una venta de 2.5 kg
curl -X POST localhost:8080/ventas \
  -H 'Content-Type: application/json' \
  -d '{"corte_id":1,"cantidad_kg":2.5}'
# -> 201 y el stock del corte baja de 10 a 7.5
# -> 409 si no hay stock suficiente
# -> 404 si el corte no existe
# -> 400 si la cantidad es <= 0

# Reporte del día
curl localhost:8080/reportes/ventas-del-dia
# {"fecha":"2026-10-26","cantidad_ventas":1,"total":112500}
```

## Estructura

```
main.go                    punto de entrada y wiring (composition root)
internal/
  model/                   estructuras de datos (Corte, Venta, Reporte)
  store/                   acceso a SQLite (repository pattern)
  service/                 reglas de negocio
  transport/               handlers HTTP
Docs/                      enunciado del reto y documentación
```

Cada capa solo conoce a la siguiente: `transport → service → store → SQLite`. Los modelos son transversales y no conocen a nadie.

## Licencia

Proyecto de práctica, sin licencia definida.
