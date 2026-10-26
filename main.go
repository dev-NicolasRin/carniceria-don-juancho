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

func main() {
	db, err := sql.Open("sqlite3", "carniceria.db?_foreign_keys=on")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}
	if err := store.Migrar(db); err != nil {
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
	mux.HandleFunc("/reportes/ventas-del-dia", ventaHandler.HandleVentasDelDia)

	log.Println("API escuchando en :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
