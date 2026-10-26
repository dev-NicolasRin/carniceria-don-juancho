package store

import "database/sql"

// Esquema contiene el DDL (Data Definition Language) que crea las tablas de
// la base de datos.
//
// ¿Por qué vive aquí y no en main.go?
// Porque lo usan DOS consumidores: la aplicación (main) y los tests
// (store.Migrar sobre una base en memoria). Si estuviera en main, el paquete
// de tests tendría que duplicarlo o importar main (que no es una librería).
// Mantenerlo en store evita divergencias: el esquema de los tests es
// exactamente el de producción.
//
// ¿Por qué CREATE TABLE IF NOT EXISTS y no un sistema de migraciones?
// Porque el reto no requiere evolucionar el esquema. IF NOT EXISTS hace la
// operación idempotente: se puede ejecutar en cada arranque sin borrar datos
// existentes. En un proyecto real se usaría una herramienta de migraciones
// (golang-migrate, goose, atlas) que versiona cada cambio.
const Esquema = `
-- cortes: inventario de carne disponible.
-- id INTEGER PRIMARY KEY AUTOINCREMENT: en SQLite, INTEGER PRIMARY KEY ya
--   es un alias del rowid; AUTOINCREMENT además garantiza que los ids no se
--   reutilicen aunque se borren filas (evita que una venta antigua apunte
--   por accidente a un corte nuevo con el mismo id).
-- REAL: tipo de SQLite para números con decimales (equivale a float64 en Go).
-- NOT NULL: obliga a que el dato exista; si un INSERT lo omite, SQLite
--   rechaza la operación en vez de guardar NULL.
-- DEFAULT CURRENT_TIMESTAMP: SQLite genera la fecha en UTC automáticamente.
CREATE TABLE IF NOT EXISTS cortes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	nombre TEXT NOT NULL,
	tipo_animal TEXT NOT NULL,
	precio_kg REAL NOT NULL,
	stock_kg REAL NOT NULL,
	fecha_creacion DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- ventas: registro histórico de ventas.
-- corte_id INTEGER NOT NULL REFERENCES cortes(id): clave foránea. Ojo: las
--   FKs en SQLite están desactivadas por defecto; main.go las enciende con
--   el parámetro ?_foreign_keys=on del DSN. Sin eso, la FK sería solo
--   documentación: SQLite no la haría cumplir.
-- total REAL: guarda el monto calculado al momento de la venta; así el
--   histórico no cambia si luego se actualiza el precio del corte.
-- created_at: se llama distinto a fecha_creacion por fidelidad al enunciado,
--   aunque cumple la misma función. En Go se mapea al campo Venta.Fecha.
CREATE TABLE IF NOT EXISTS ventas (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	corte_id INTEGER NOT NULL REFERENCES cortes(id),
	cantidad_kg REAL NOT NULL,
	total REAL NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

// Migrar aplica el esquema a la base de datos recibida.
//
// Se usa tanto en main.go (base de archivo carniceria.db) como en los tests
// (base :memory:). Devolver el error permite que el llamador decida qué
// hacer: main aborta con log.Fatal y los tests fallan con t.Fatalf.
//
// ¿Por qué no usar db.MustExec? Porque database/sql no tiene MustExec; ese
// método es de sqlx. La librería estándar devuelve errores explícitos y aquí
// se propagan sin ocultarlos.
func Migrar(db *sql.DB) error {
	_, err := db.Exec(Esquema)
	return err
}
