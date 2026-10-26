package transport

import (
	"carniceria-don-juanchito/internal/model"
	"carniceria-don-juanchito/internal/service"
	"carniceria-don-juanchito/internal/store"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func nuevoRouterPrueba(t *testing.T) (*http.ServeMux, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("abriendo db de prueba: %v", err)
	}
	// Con :memory: cada conexion crea una base distinta, por eso limitamos a una sola.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if err := store.Migrar(db); err != nil {
		t.Fatalf("creando esquema: %v", err)
	}

	servicio := service.New(store.New(db), store.NewVentas(db))
	corteHandler := New(servicio)
	ventaHandler := NewVentaHandler(servicio)

	mux := http.NewServeMux()
	mux.HandleFunc("/cortes", corteHandler.HandleCortes)
	mux.HandleFunc("/cortes/", corteHandler.HandleCorteByID)
	mux.HandleFunc("/ventas", ventaHandler.HandleVentas)
	mux.HandleFunc("/reportes/ventas-del-dia", ventaHandler.HandleVentasDelDia)

	return mux, db
}

func sembrarCorte(t *testing.T, db *sql.DB, nombre, tipo string, precio, stock float64) int {
	t.Helper()
	resp, err := db.Exec(
		"INSERT INTO cortes (nombre, tipo_animal, precio_kg, stock_kg) VALUES (?, ?, ?, ?)",
		nombre, tipo, precio, stock,
	)
	if err != nil {
		t.Fatalf("sembrando corte: %v", err)
	}
	id, err := resp.LastInsertId()
	if err != nil {
		t.Fatalf("obteniendo id del corte: %v", err)
	}
	return int(id)
}

func peticion(t *testing.T, mux *http.ServeMux, metodo, ruta, cuerpo string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func stockDeCorte(t *testing.T, db *sql.DB, id int) float64 {
	t.Helper()
	var stock float64
	if err := db.QueryRow("SELECT stock_kg FROM cortes WHERE id = ?", id).Scan(&stock); err != nil {
		t.Fatalf("consultando stock: %v", err)
	}
	return stock
}

func TestCrearVentaExitosaDescuentaStock(t *testing.T) {
	mux, db := nuevoRouterPrueba(t)
	id := sembrarCorte(t, db, "Lomo fino", "res", 45000, 10)

	rec := peticion(t, mux, http.MethodPost, "/ventas", `{"corte_id":1,"cantidad_kg":2.5}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status esperado 201, obtenido %d: %s", rec.Code, rec.Body.String())
	}

	var venta model.Venta
	if err := json.NewDecoder(rec.Body).Decode(&venta); err != nil {
		t.Fatalf("decodificando venta: %v", err)
	}
	if venta.Total != 112500 {
		t.Errorf("total esperado 112500, obtenido %v", venta.Total)
	}
	if venta.ID != 1 {
		t.Errorf("id esperado 1, obtenido %d", venta.ID)
	}

	if stock := stockDeCorte(t, db, id); stock != 7.5 {
		t.Errorf("stock esperado 7.5, obtenido %v", stock)
	}
}

func TestCrearVentaSinStockResponde409(t *testing.T) {
	mux, db := nuevoRouterPrueba(t)
	id := sembrarCorte(t, db, "Costilla", "cerdo", 20000, 1)

	rec := peticion(t, mux, http.MethodPost, "/ventas", `{"corte_id":1,"cantidad_kg":5}`)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status esperado 409, obtenido %d: %s", rec.Code, rec.Body.String())
	}

	if stock := stockDeCorte(t, db, id); stock != 1 {
		t.Errorf("el stock no debe cambiar; esperado 1, obtenido %v", stock)
	}

	var ventas int
	if err := db.QueryRow("SELECT COUNT(*) FROM ventas").Scan(&ventas); err != nil {
		t.Fatalf("contando ventas: %v", err)
	}
	if ventas != 0 {
		t.Errorf("no debe quedar venta registrada, hay %d", ventas)
	}
}

func TestCrearVentaCorteInexistenteResponde404(t *testing.T) {
	mux, _ := nuevoRouterPrueba(t)

	rec := peticion(t, mux, http.MethodPost, "/ventas", `{"corte_id":999,"cantidad_kg":1}`)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status esperado 404, obtenido %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCrearVentaCantidadInvalidaResponde400(t *testing.T) {
	mux, db := nuevoRouterPrueba(t)
	sembrarCorte(t, db, "Lomo fino", "res", 45000, 10)

	rec := peticion(t, mux, http.MethodPost, "/ventas", `{"corte_id":1,"cantidad_kg":-1}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status esperado 400, obtenido %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFiltroCortesPorTipo(t *testing.T) {
	mux, db := nuevoRouterPrueba(t)
	sembrarCorte(t, db, "Lomo fino", "res", 45000, 10)
	sembrarCorte(t, db, "Costilla", "cerdo", 20000, 10)

	rec := peticion(t, mux, http.MethodGet, "/cortes?tipo=cerdo", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status esperado 200, obtenido %d", rec.Code)
	}
	var cortes []model.Corte
	if err := json.NewDecoder(rec.Body).Decode(&cortes); err != nil {
		t.Fatalf("decodificando cortes: %v", err)
	}
	if len(cortes) != 1 || cortes[0].TipoAnimal != "cerdo" {
		t.Fatalf("se esperaba 1 corte de cerdo, obtenido %+v", cortes)
	}
}

func TestFiltroStockBajo(t *testing.T) {
	mux, db := nuevoRouterPrueba(t)
	sembrarCorte(t, db, "Lomo fino", "res", 45000, 3)
	sembrarCorte(t, db, "Costilla", "cerdo", 20000, 10)

	rec := peticion(t, mux, http.MethodGet, "/cortes?stock_bajo=true", "")

	var cortes []model.Corte
	if err := json.NewDecoder(rec.Body).Decode(&cortes); err != nil {
		t.Fatalf("decodificando cortes: %v", err)
	}
	if len(cortes) != 1 || cortes[0].Nombre != "Lomo fino" {
		t.Fatalf("se esperaba solo el corte con stock bajo, obtenido %+v", cortes)
	}
}

func TestObtenerCorteInexistenteResponde404(t *testing.T) {
	mux, _ := nuevoRouterPrueba(t)

	rec := peticion(t, mux, http.MethodGet, "/cortes/99", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status esperado 404, obtenido %d: %s", rec.Code, rec.Body.String())
	}
}

func TestReporteVentasDelDia(t *testing.T) {
	mux, db := nuevoRouterPrueba(t)
	sembrarCorte(t, db, "Lomo fino", "res", 45000, 10)
	peticion(t, mux, http.MethodPost, "/ventas", `{"corte_id":1,"cantidad_kg":2}`)
	peticion(t, mux, http.MethodPost, "/ventas", `{"corte_id":1,"cantidad_kg":1}`)

	rec := peticion(t, mux, http.MethodGet, "/reportes/ventas-del-dia", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	var reporte model.ReporteVentasDia
	if err := json.NewDecoder(rec.Body).Decode(&reporte); err != nil {
		t.Fatalf("decodificando reporte: %v", err)
	}
	if reporte.CantidadVentas != 2 {
		t.Errorf("cantidad de ventas esperada 2, obtenida %d", reporte.CantidadVentas)
	}
	if reporte.Total != 135000 {
		t.Errorf("total esperado 135000, obtenido %v", reporte.Total)
	}
}
