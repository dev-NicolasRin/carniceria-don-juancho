package main

import (
	"carniceria-don-juanchito/internal/service"
	"carniceria-don-juanchito/internal/store"
	"carniceria-don-juanchito/internal/transport"
	"database/sql"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"
)

const esquema = `
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

func main() {
	db, err := sql.Open("sqlite3", "carniceria.db?_foreign_keys=on")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(esquema); err != nil {
		log.Fatal(err)
	}

	cortesStore := store.New(db)
	ventasStore := store.NewVentas(db)
	servicio := service.New(cortesStore, ventasStore)

	corteHandler := transport.New(servicio)
	ventaHandler := transport.NewVentaHandler(servicio)

	mux := http.NewServeMux()
	mux.HandleFunc("/cortes", corteHandler.HandleCortes)
	mux.HandleFunc("/cortes/", corteHandler.HandleCorteByID)
	mux.HandleFunc("/ventas", ventaHandler.HandleVentas)

	log.Println("API escuchando en :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
