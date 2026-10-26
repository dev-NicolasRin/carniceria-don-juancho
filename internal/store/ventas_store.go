package store

import (
	"carniceria-don-juanchito/internal/model"
	"database/sql"
	"errors"
	"time"
)

// ErrStockInsuficiente es un "error sentinel": una variable de error con un
// mensaje fijo que las capas superiores pueden identificar con errors.Is.
//
// ¿Por qué no devolver un simple fmt.Errorf("no hay stock")?
// Porque comparar mensajes de texto es frágil (cambiar una tilde rompe la
// lógica) y acopla las capas. Con una variable exportada, el handler pregunta
// "¿este error ES exactamente este caso?" y responde 409 Conflict. Es el
// mecanismo idiomático de Go para clasificar errores sin excepciones.
var ErrStockInsuficiente = errors.New("no hay suficiente stock para la venta")

// VentasStore es la interfaz del repositorio de ventas.
// Cada método corresponde a una operación de negocio concreta.
type VentasStore interface {
	GetAllVentas() ([]*model.Venta, error)
	CreateVenta(venta *model.Venta) (*model.Venta, error)
	VentasDelDia() (*model.ReporteVentasDia, error)
}

// ventasStore es la implementación sobre SQLite. Se separa del store de
// cortes aunque compartan base de datos: cada repositorio agrupa las
// operaciones de su entidad (principio de responsabilidad única).
type ventasStore struct {
	db *sql.DB
}

func NewVentas(db *sql.DB) VentasStore {
	return &ventasStore{db: db}
}

// GetAllVentas lista el histórico de ventas junto con el nombre del corte.
//
// ¿Por qué un JOIN y no dos consultas (una de ventas y otra de cortes)?
// Porque el JOIN resuelve todo en una sola ida a la base de datos: es más
// eficiente (evita el problema N+1: una consulta por cada venta) y la
// respuesta ya sale lista para serializar. c.nombre se guarda en el campo
// derivado Venta.NombreCorte.
//
// ORDER BY v.id da un orden estable y predecible, útil para tests y para el
// cliente. Sin ORDER BY, SQL no garantiza ningún orden.
func (ventas *ventasStore) GetAllVentas() ([]*model.Venta, error) {
	q := `SELECT v.id, v.corte_id, c.nombre, v.cantidad_kg, v.total, v.created_at
	      FROM ventas v
	      JOIN cortes c ON c.id = v.corte_id
	      ORDER BY v.id`

	rows, err := ventas.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lista []*model.Venta
	for rows.Next() {
		venta_actual := model.Venta{}
		if err := rows.Scan(&venta_actual.ID, &venta_actual.IDCorte, &venta_actual.NombreCorte,
			&venta_actual.CantidadKg, &venta_actual.Total, &venta_actual.Fecha); err != nil {
			return nil, err
		}
		lista = append(lista, &venta_actual)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return lista, nil
}

// CreateVenta registra una venta y descuenta el stock en UNA transacción.
//
// ¿Qué es una transacción y por qué es imprescindible aquí?
// Una transacción agrupa varias operaciones en una unidad atómica: o se
// aplican TODAS o NINGUNA. Registrar una venta implica dos escrituras:
//  1. INSERT en ventas.
//  2. UPDATE del stock en cortes.
//
// Si se hicieran por separado y fallara la segunda (corte borrado, disco
// lleno, corte de luz), quedaría una venta cobrada sin descontar stock: el
// inventario mentiría. Con Begin/Commit, si algo falla se ejecuta Rollback y
// la base queda como estaba. Esto cumple las propiedades ACID:
// Atomicidad (todo o nada), Consistencia (stock nunca negativo), Aislamiento
// (otras conexiones no ven estados intermedios) y Durabilidad (el Commit se
// persiste).
//
// ¿Por qué se lee el precio DENTRO de la transacción?
// Para evitar una condición de carrera: entre leer el precio y descontar el
// stock, otro proceso podría cambiar el precio. Al hacerlo dentro de la misma
// transacción, el total se calcula con el precio vigente en ese instante.
//
// El flujo es:
//
//	Begin -> SELECT precio/stock -> validar stock -> INSERT venta
//	      -> UPDATE stock -> Commit
//
// y cualquier retorno antes del Commit deja que el defer ejecute Rollback.
func (ventas *ventasStore) CreateVenta(venta *model.Venta) (*model.Venta, error) {
	// Begin inicia la transacción y devuelve un *sql.Tx, que se comporta
	// como la conexión pero con estado transaccional.
	tx, err := ventas.db.Begin()
	if err != nil {
		return nil, err
	}
	// defer tx.Rollback() se ejecuta SIEMPRE al salir de la función. Si ya se
	// hizo Commit, Rollback devuelve sql.ErrTxDone y no hace nada: es
	// inofensivo. Este patrón "defer rollback + commit explícito" es el
	// idiomático en Go porque garantiza que nunca se olvide revertir.
	defer tx.Rollback()

	// Se leen precio y stock actuales del corte dentro de la transacción.
	// QueryRow sobre tx (no sobre db) es clave: si se usara db, la lectura
	// podría ocurrir fuera de la transacción y no vería los bloqueos.
	var precioKilo, stockKilos float64
	err = tx.QueryRow("SELECT precio_kg, stock_kg FROM cortes WHERE id = ?", venta.IDCorte).
		Scan(&precioKilo, &stockKilos)
	if err != nil {
		return nil, err // sql.ErrNoRows si el corte no existe -> el handler responde 404
	}

	// Validación de negocio: no se puede vender más de lo que hay.
	// Se devuelve el sentinel y el defer hace Rollback automáticamente.
	if stockKilos < venta.CantidadKg {
		return nil, ErrStockInsuficiente
	}

	// El total SIEMPRE se calcula en el servidor con el precio de la BD,
	// nunca con un total enviado por el cliente. Así se evita que alguien
	// manipule el precio de la venta.
	venta.Total = precioKilo * venta.CantidadKg
	// UTC para que coincida con las fechas que genera SQLite (su
	// CURRENT_TIMESTAMP es UTC). Mezclar zonas horarias haría que el reporte
	// "del día" contara ventas en el día equivocado.
	venta.Fecha = time.Now().UTC()

	resp, err := tx.Exec(
		"INSERT INTO ventas (corte_id, cantidad_kg, total, created_at) VALUES (?, ?, ?, ?)",
		venta.IDCorte, venta.CantidadKg, venta.Total, venta.Fecha,
	)
	if err != nil {
		return nil, err
	}
	id, err := resp.LastInsertId()
	if err != nil {
		return nil, err
	}

	// UPDATE atómico en la base: stock_kg = stock_kg - cantidad. Se hace en
	// SQL (no leyendo y escribiendo desde Go) para que el cálculo ocurra en
	// el motor; así dos ventas concurrentes no pisan el valor de la otra.
	// Nótese que el WHERE usa el id y no compara stock: la comprobación ya
	// se hizo arriba dentro de la transacción.
	if _, err := tx.Exec(
		"UPDATE cortes SET stock_kg = stock_kg - ? WHERE id = ?",
		venta.CantidadKg, venta.IDCorte,
	); err != nil {
		return nil, err
	}

	// Commit confirma de forma definitiva los cambios de la transacción.
	// A partir de aquí la venta y el nuevo stock son visibles para todos.
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	venta.ID = int(id)
	return venta, nil
}

// VentasDelDia calcula cuántas ventas se hicieron hoy y cuánto dinero sumaron.
//
// Detalles de la consulta:
//   - COUNT(*) cuenta filas (ventas), no la suma de cantidades.
//   - COALESCE(SUM(total), 0) evita que SUM devuelva NULL cuando no hay
//     ventas; sin él, escanear NULL en un float64 daría error.
//   - DATE(created_at) extrae solo la parte de fecha. Comparar contra el
//     string YYYY-MM-DD evita problemas de hora/minuto/segundo.
//   - La fecha actual se calcula en Go en UTC y se pasa como parámetro, en
//     lugar de usar DATE('now') dentro del SQL: así el resultado es
//     determinista y testeable (se podría inyectar otra fecha si se quisiera).
func (ventas *ventasStore) VentasDelDia() (*model.ReporteVentasDia, error) {
	reporte := model.ReporteVentasDia{Fecha: time.Now().UTC().Format("2006-01-02")}
	err := ventas.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(total), 0) FROM ventas WHERE DATE(created_at) = ?`,
		reporte.Fecha,
	).Scan(&reporte.CantidadVentas, &reporte.Total)
	if err != nil {
		return nil, err
	}
	return &reporte, nil
}
