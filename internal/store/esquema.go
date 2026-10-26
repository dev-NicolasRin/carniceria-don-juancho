package store

import "database/sql"

// Esquema contiene las tablas de la API. Se ejecuta al arrancar la aplicación
// y en las pruebas, por eso vive en el paquete store y no en main.
const Esquema = `
CREATE TABLE IF NOT EXISTS cortes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	nombre TEXT NOT NULL,
	tipo_animal TEXT NOT NULL,
	precio_kg REAL NOT NULL,
	stock_kg REAL NOT NULL,
	fecha_creacion DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS ventas (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	corte_id INTEGER NOT NULL REFERENCES cortes(id),
	cantidad_kg REAL NOT NULL,
	total REAL NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

func Migrar(db *sql.DB) error {
	_, err := db.Exec(Esquema)
	return err
}
