// Tests de integración de la capa HTTP.
//
// ¿Por qué "de integración" y no unitarios?
// Porque no se mockean las capas: se levanta un router real con handlers,
// service y repositorios reales sobre una base SQLite en memoria. Así se
// prueba el camino completo (HTTP -> service -> SQL -> respuesta) y se
// detectan errores de wiring que un mock ocultaría. Son rápidos porque la
// base vive en RAM y se destruye al terminar cada test.
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

// nuevoRouterPrueba construye un router idéntico al de main.go pero sobre una
// base en memoria. Devuelve también la *sql.DB para que cada test pueda
// sembrar datos y verificar el estado final.
//
// Patrón t.Helper(): marca la función como helper para que, si falla, el
// reporte de Go señale la línea del test que la llamó y no la línea interna
// del helper. Mejora mucho la legibilidad de los errores.
func nuevoRouterPrueba(t *testing.T) (*http.ServeMux, *sql.DB) {
	t.Helper()

	// ":memory:" crea una base SQLite en RAM. Es ideal para tests: no deja
	// archivos, es rapidísima y cada test empieza desde cero (aislamiento).
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("abriendo db de prueba: %v", err)
	}
	// Con :memory: cada CONEXIÓN crea una base distinta. database/sql usa un
	// pool, así que si abre dos conexiones, el test vería dos bases
	// separadas (una con las tablas y otra vacía). Limitar el pool a 1
	// conexión garantiza que todos los queries compartan la misma base.
	db.SetMaxOpenConns(1)
	// t.Cleanup registra el cierre para cuando el test termine, incluso si
	// falla. Evita fugas de recursos y no requiere defer manual en cada test.
	t.Cleanup(func() { db.Close() })

	// Se usa el MISMO esquema que producción (store.Esquema). Si se
	// duplicara aquí, un cambio en las tablas podría hacer que los tests
	// pasen con un esquema viejo.
	if err := store.Migrar(db); err != nil {
		t.Fatalf("creando esquema: %v", err)
	}

	servicio := service.New(store.New(db), store.NewVentas(db))
	corteHandler := New(servicio)
	ventaHandler := NewVentaHandler(servicio)

	// Mismo registro de rutas que main.go. Si se agrega un endpoint en main
	// hay que agregarlo aquí para poder testearlo.
	mux := http.NewServeMux()
	mux.HandleFunc("/cortes", corteHandler.HandleCortes)
	mux.HandleFunc("/cortes/", corteHandler.HandleCorteByID)
	mux.HandleFunc("/ventas", ventaHandler.HandleVentas)
	mux.HandleFunc("/reportes/ventas-del-dia", ventaHandler.HandleVentasDelDia)

	return mux, db
}

// sembrarCorte inserta un corte directamente con SQL, sin pasar por la API.
//
// ¿Por qué no crearlo con POST /cortes? Porque así el test de ventas no
// depende de que el endpoint de cortes funcione: se aísla la causa de un
// fallo. Es el concepto de "arrange" en el patrón Arrange-Act-Assert.
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

// peticion ejecuta una petición HTTP contra el mux sin abrir un puerto real.
//
// httptest.NewRequest crea un *http.Request; httptest.NewRecorder actúa como
// un http.ResponseWriter que guarda status, headers y body en memoria. Llamar
// a mux.ServeHTTP ejecuta el handler de forma síncrona. Es el patrón estándar
// para testear handlers en Go sin red ni servidor.
func peticion(t *testing.T, mux *http.ServeMux, metodo, ruta, cuerpo string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// stockDeCorte consulta el stock actual directamente en la BD.
// Verificar el efecto en la base (no solo el JSON de respuesta) es clave en
// una operación con efectos secundarios como descontar inventario.
func stockDeCorte(t *testing.T, db *sql.DB, id int) float64 {
	t.Helper()
	var stock float64
	if err := db.QueryRow("SELECT stock_kg FROM cortes WHERE id = ?", id).Scan(&stock); err != nil {
		t.Fatalf("consultando stock: %v", err)
	}
	return stock
}

// TestCrearVentaExitosaDescuentaStock cubre el camino feliz: la venta se crea
// (201), el total se calcula en el servidor (45000 × 2.5 = 112500) y el stock
// baja de 10 a 7.5 kg.
func TestCrearVentaExitosaDescuentaStock(t *testing.T) {
	mux, db := nuevoRouterPrueba(t)
	id := sembrarCorte(t, db, "Lomo fino", "res", 45000, 10)

	rec := peticion(t, mux, http.MethodPost, "/ventas", `{"corte_id":1,"cantidad_kg":2.5}`)

	// t.Fatalf corta el test en el acto; se usa cuando seguir no tiene
	// sentido (por ejemplo, si el status no es el esperado).
	if rec.Code != http.StatusCreated {
		t.Fatalf("status esperado 201, obtenido %d: %s", rec.Code, rec.Body.String())
	}

	var venta model.Venta
	if err := json.NewDecoder(rec.Body).Decode(&venta); err != nil {
		t.Fatalf("decodificando venta: %v", err)
	}
	// t.Errorf registra el fallo pero deja continuar, para reportar varios
	// problemas de una sola ejecución.
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

// TestCrearVentaSinStockResponde409 verifica la regla de negocio más
// importante: si no hay stock, responde 409 Conflict Y la transacción no deja
// rastros (ni venta insertada ni stock modificado). Es la prueba del rollback.
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

// TestCrearVentaCorteInexistenteResponde404 comprueba que vender un corte que
// no existe devuelve 404 y no 500. El service propaga sql.ErrNoRows y el
// handler lo traduce.
func TestCrearVentaCorteInexistenteResponde404(t *testing.T) {
	mux, _ := nuevoRouterPrueba(t)

	rec := peticion(t, mux, http.MethodPost, "/ventas", `{"corte_id":999,"cantidad_kg":1}`)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status esperado 404, obtenido %d: %s", rec.Code, rec.Body.String())
	}
}

// TestCrearVentaCantidadInvalidaResponde400 valida la regla cantidad > 0 en la
// capa service (antes de tocar la base de datos).
func TestCrearVentaCantidadInvalidaResponde400(t *testing.T) {
	mux, db := nuevoRouterPrueba(t)
	sembrarCorte(t, db, "Lomo fino", "res", 45000, 10)

	rec := peticion(t, mux, http.MethodPost, "/ventas", `{"corte_id":1,"cantidad_kg":-1}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status esperado 400, obtenido %d: %s", rec.Code, rec.Body.String())
	}
}

// TestFiltroCortesPorTipo verifica el WHERE dinámico del repositorio:
// ?tipo=cerdo debe devolver solo los cortes de cerdo.
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

// TestFiltroStockBajo verifica ?stock_bajo=true: solo aparece el corte con
// stock menor al umbral de 5 kg (3 kg en este caso).
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

// TestObtenerCorteInexistenteResponde404 cubre la traducción de sql.ErrNoRows
// a 404 en el GET por id.
func TestObtenerCorteInexistenteResponde404(t *testing.T) {
	mux, _ := nuevoRouterPrueba(t)

	rec := peticion(t, mux, http.MethodGet, "/cortes/99", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status esperado 404, obtenido %d: %s", rec.Code, rec.Body.String())
	}
}

// TestReporteVentasDelDia registra dos ventas (2 kg y 1 kg a 45000/kg) y
// comprueba que el reporte agrega bien: 2 ventas y 135000 de total.
// El cálculo lo hace SQL (COUNT + SUM), no Go.
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
