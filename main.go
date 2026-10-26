// Package main es el punto de entrada de la API.
//
// Aquí se hace el "wiring" o composition root: se abren las dependencias
// externas (base de datos, mux HTTP) y se conectan entre sí. Ninguna otra
// capa construye sus propias dependencias; todas las reciben inyectadas.
// Eso hace que el resto del código sea fácil de testear y de reemplazar.
package main

import (
	"carniceria-don-juanchito/internal/service"
	"carniceria-don-juanchito/internal/store"
	"carniceria-don-juanchito/internal/transport"
	"database/sql"
	"log"
	"net/http"

	// Import con guion bajo (blank import): registra el driver de SQLite en
	// database/sql sin usar sus identificadores directamente. database/sql
	// es genérica; el driver se "auto-registra" con el nombre "sqlite3" al
	// importarse. Sin esta línea, sql.Open("sqlite3", ...) fallaría con
	// "unknown driver".
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// sql.Open NO abre una conexión todavía: solo valida el nombre del
	// driver y configura el pool. La primera conexión real ocurre en el
	// primer uso. Por eso se llama a Ping más abajo: así el arranque falla
	// pronto si la base no es accesible, en vez de fallar en la primera
	// petición de un usuario.
	//
	// El DSN "carniceria.db?_foreign_keys=on" tiene dos partes:
	//   - carniceria.db: archivo en el directorio de trabajo. SQLite crea el
	//     archivo automáticamente si no existe.
	//   - ?_foreign_keys=on: activa la comprobación de claves foráneas.
	//     SQLite las tiene DESACTIVADAS por defecto por compatibilidad
	//     histórica; sin este parámetro, la FK de ventas.corte_id sería
	//     decorativa.
	db, err := sql.Open("sqlite3", "carniceria.db?_foreign_keys=on")
	if err != nil {
		// log.Fatal imprime el error y termina el proceso con código != 0.
		// Es apropiado en main: si no hay base de datos, la API no puede
		// funcionar y no tiene sentido seguir.
		log.Fatal(err)
	}
	// defer db.Close() cierra el pool al salir de main. En un servidor que
	// corre indefinidamente solo se ejecuta si el proceso termina de forma
	// ordenada, pero es buena práctica no filtrar recursos.
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}
	// Migrar crea las tablas si no existen. Es idempotente: se puede llamar
	// en cada arranque sin borrar datos.
	if err := store.Migrar(db); err != nil {
		log.Fatal(err)
	}

	// Inyección de dependencias en cascada:
	// db -> repositorios -> servicio -> handlers -> mux.
	// Se comparte el MISMO *sql.DB entre ambos repositorios: es un pool
	// seguro para concurrencia y necesario para que las transacciones de
	// ventas vean los datos de cortes.
	cortesStore := store.New(db)
	ventasStore := store.NewVentas(db)
	servicio := service.New(cortesStore, ventasStore)

	corteHandler := transport.New(servicio)
	ventaHandler := transport.NewVentaHandler(servicio)

	// ServeMux es el router de la librería estándar.
	// Go 1.22+ permite patrones como "GET /cortes/{id}", pero aquí se usan
	// rutas simples con prefijo porque los handlers ya discriminan el método
	// con switch, lo que mantiene el ejemplo más explícito.
	//
	// Detalle importante: "/cortes/" termina en barra y actúa como subárbol;
	// por eso captura /cortes/1, /cortes/2, etc. La ruta "/cortes" (sin
	// barra) es exacta. Si solo se registrara "/cortes/", /cortes sin barra
	// daría 301 redirect.
	mux := http.NewServeMux()
	mux.HandleFunc("/cortes", corteHandler.HandleCortes)
	mux.HandleFunc("/cortes/", corteHandler.HandleCorteByID)
	mux.HandleFunc("/ventas", ventaHandler.HandleVentas)
	mux.HandleFunc("/reportes/ventas-del-dia", ventaHandler.HandleVentasDelDia)

	log.Println("API escuchando en :8080")
	// ListenAndServe bloquea hasta que el servidor se detiene. Si devuelve
	// un error (puerto ocupado, por ejemplo), se registra y se sale.
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
