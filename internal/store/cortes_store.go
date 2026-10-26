// Package store implementa el acceso a datos (patrón repository).
//
// La idea del patrón repository es simple: la aplicación no debe saber si los
// datos viven en SQLite, PostgreSQL o un archivo. El resto del programa habla
// con una INTERFAZ (Store, VentasStore) y este paquete es el único que escribe
// SQL. Si mañana se cambia de base de datos, se reescribe este paquete y nada
// más. Además, al ser una interfaz, se puede sustituir por un doble de prueba
// (mock/fake) en tests de las capas superiores.
package store

import (
	"carniceria-don-juanchito/internal/model"
	"database/sql"
	"strings"
)

// stockBajoKilos define el umbral de "stock bajo" usado por el filtro
// ?stock_bajo=true. Es una constante y no un número mágico esparcido por el
// código: si mañana el negocio decide que el umbral es 10 kg, se cambia en un
// solo lugar.
const stockBajoKilos = 5

// FiltroCortes agrupa los filtros opcionales de listado.
//
// ¿Por qué un struct en vez de dos parámetros sueltos (tipo string, stockBajo bool)?
// Porque (1) es autodocumentado en la llamada, (2) permite agregar filtros
// nuevos sin cambiar la firma de todos los métodos y (3) evita el clásico
// error de pasar los argumentos invertidos, ya que ambos serían string/bool
// posicionales. En Go no hay argumentos con nombre, así que un struct es la
// forma idiomática de agrupar opciones.
type FiltroCortes struct {
	// Tipo es el tipo de animal a filtrar. Cadena vacía significa
	// "sin filtro" (se devuelven todos los cortes).
	Tipo string

	// StockBajo, si es true, devuelve solo los cortes con stock menor
	// al umbral stockBajoKilos.
	StockBajo bool
}

// Store es la interfaz del repositorio de cortes.
//
// En Go, la convención es que las interfaces se definan en el paquete que las
// CONSUME (aquí service podría definirla), pero en proyectos pequeños es
// común tenerla junto a la implementación para que todo quede cerca. Al ser
// una interfaz pequeña y concreta, cumple el principio de segregación de
// interfaces (ISP): solo expone lo que la capa de negocio necesita.
type Store interface {
	GetAll(filtro FiltroCortes) ([]*model.Corte, error)
	GetByID(id int) (*model.Corte, error)
	Create(book *model.Corte) (*model.Corte, error)
	Update(id int, book *model.Corte) (*model.Corte, error)
	Delete(id int) error
}

// store es la implementación concreta sobre SQLite.
//
// Es un struct NO exportado (minúscula) a propósito: nadie fuera del paquete
// puede crear un store sin pasar por New. Esa es la Encapsulación: se controla
// la construcción y se puede cambiar la implementación interna sin romper a
// los consumidores.
type store struct {
	db *sql.DB
}

// New recibe una *sql.DB ya abierta y devuelve la interfaz Store.
//
// ¿Por qué recibir la conexión en vez de abrirla aquí?
// Porque así se inyecta la dependencia (dependency injection): main decide
// qué base usar (archivo, memoria, otra ruta) y los tests pueden pasar una
// base :memory: sin tocar el código de producción. También permite compartir
// la misma conexión entre los repositorios de cortes y ventas, algo necesario
// para las transacciones.
//
// *sql.DB no es una conexión individual, sino un pool de conexiones seguro
// para usar desde varias goroutines. Por eso se recibe como puntero.
func New(db *sql.DB) Store {
	return &store{db: db}
}

// GetAll devuelve todos los cortes, opcionalmente filtrados.
//
// Construcción dinámica del WHERE:
// Se acumulan condiciones y argumentos en paralelo. El SQL final se arma con
// strings.Join, pero los VALORES nunca se concatenan: se pasan como
// parámetros (?) al driver. Esa es la defensa contra SQL injection: el driver
// escapa y envía los datos aparte de la estructura de la consulta. Si se
// hiciera "WHERE tipo_animal = '" + filtro.Tipo + "'" un atacante podría
// enviar tipo=res'; DROP TABLE cortes;-- y ejecutar SQL arbitrario.
func (carniceria *store) GetAll(filtro FiltroCortes) ([]*model.Corte, error) {
	q := "SELECT id, nombre, tipo_animal, precio_kg, stock_kg, fecha_creacion FROM cortes"

	var condiciones []string
	var args []any // []any (antes []interface{}) acepta valores de cualquier tipo
	if filtro.Tipo != "" {
		condiciones = append(condiciones, "tipo_animal = ?")
		args = append(args, filtro.Tipo)
	}
	if filtro.StockBajo {
		// El umbral también va como parámetro: el SQL no cambia, solo el valor.
		condiciones = append(condiciones, "stock_kg < ?")
		args = append(args, stockBajoKilos)
	}
	if len(condiciones) > 0 {
		q += " WHERE " + strings.Join(condiciones, " AND ")
	}

	// Query se usa para consultas que devuelven varias filas (SELECT).
	// Con args... se expande el slice como argumentos variádicos.
	rows, err := carniceria.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	// defer rows.Close() garantiza liberar el cursor y devolver la conexión
	// al pool incluso si se retorna antes por un error. Olvidarlo produce
	// fugas de conexiones (connection leak): tras varias peticiones el pool
	// se agota y la API deja de responder.
	defer rows.Close()

	// Se devuelven punteros (*model.Corte) para no copiar la estructura en
	// cada iteración y para poder devolver nil de forma explícita si no hay
	// datos. Un slice nil se serializa como `null` en JSON; si se prefiere
	// `[]`, habría que inicializarlo con make([]*model.Corte, 0).
	var Cortes []*model.Corte
	for rows.Next() {
		corte_actual := model.Corte{}
		// Scan copia las columnas (en el orden del SELECT) dentro de los
		// campos. El orden importa: si se cambia el SELECT hay que cambiar
		// este Scan. Pasar los tipos correctos (&int, &string, &float64,
		// &time.Time) es responsabilidad del programador; database/sql
		// convierte cuando puede, pero no adivina.
		if err := rows.Scan(&corte_actual.ID, &corte_actual.Nombre, &corte_actual.TipoAnimal,
			&corte_actual.PrecioKilo, &corte_actual.StockKilos, &corte_actual.FechaCreacion); err != nil {
			return nil, err
		}
		Cortes = append(Cortes, &corte_actual)
	}
	// rows.Err() es MUY importante: Next() devuelve false tanto al terminar
	// normalmente como si hubo un error a mitad de la iteración. Sin este
	// chequeo se devolvería una lista incompleta sin avisar. Es un error
	// clásico y silencioso.
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return Cortes, nil
}

// GetByID busca un corte por su clave primaria.
//
// QueryRow se usa cuando se espera como máximo UNA fila. Si no hay filas,
// Scan devuelve sql.ErrNoRows; ese error se propaga tal cual para que el
// handler lo traduzca a un 404. ¿Por qué no devolver nil, nil? Porque
// "no encontrado" es un caso excepcional que el llamador debe distinguir de
// "encontrado con éxito"; un error sentinel (sql.ErrNoRows) es la forma
// idiomática de hacerlo y se compara con errors.Is.
func (carniceria *store) GetByID(id int) (*model.Corte, error) {
	q := "SELECT id, nombre, tipo_animal, precio_kg, stock_kg, fecha_creacion FROM cortes WHERE id = ?"
	corte_encontrado := model.Corte{}
	err := carniceria.db.QueryRow(q, id).Scan(&corte_encontrado.ID, &corte_encontrado.Nombre,
		&corte_encontrado.TipoAnimal, &corte_encontrado.PrecioKilo,
		&corte_encontrado.StockKilos, &corte_encontrado.FechaCreacion)
	if err != nil {
		return nil, err
	}
	return &corte_encontrado, nil
}

// Create inserta un corte y devuelve la fila completa ya guardada.
//
// ¿Por qué se relee con GetByID en vez de devolver el struct recibido?
// Porque la base de datos genera datos que el cliente no envió:
//   - id (AUTOINCREMENT)
//   - fecha_creacion (DEFAULT CURRENT_TIMESTAMP)
//
// Si se devolviera el struct original, la respuesta del POST tendría id 0 y
// fecha cero ("0001-01-01T00:00:00Z"), datos falsos. Releer cuesta una
// consulta extra, pero garantiza que la respuesta refleje exactamente lo que
// quedó persistido. En un sistema de alto tráfico se podría usar
// RETURNING (SQLite 3.35+) para hacerlo en una sola sentencia; aquí
// priorizamos claridad.
func (carniceria *store) Create(corte *model.Corte) (*model.Corte, error) {
	q := "INSERT INTO cortes (nombre, tipo_animal, precio_kg, stock_kg) VALUES (?, ?, ?, ?)"
	// Exec se usa para INSERT/UPDATE/DELETE: no devuelve filas, sino un
	// sql.Result con metadatos (filas afectadas, último id insertado).
	resp, err := carniceria.db.Exec(q, corte.Nombre, corte.TipoAnimal, corte.PrecioKilo, corte.StockKilos)
	if err != nil {
		return nil, err
	}
	id, err := resp.LastInsertId()
	if err != nil {
		return nil, err
	}
	corte.ID = int(id)
	return carniceria.GetByID(corte.ID) // relee la fila para traer la fecha generada por SQLite
}

// Update modifica precio y stock de un corte existente.
//
// Solo se actualizan precio_kg y stock_kg porque son los datos mutables del
// negocio según el enunciado; nombre y tipo_animal se consideran
// identificatorios y no se tocan en un PUT. El WHERE id = ? es obligatorio:
// sin él, el UPDATE afectaría TODAS las filas (error catastrófico clásico).
//
// Igual que en Create, se relee la fila para devolver el estado real
// (incluida la fecha de creación, que el body del PUT no trae).
func (carniceria *store) Update(id int, corte *model.Corte) (*model.Corte, error) {
	q := "UPDATE cortes SET precio_kg = ?, stock_kg = ? WHERE id = ?"
	_, err := carniceria.db.Exec(q, corte.PrecioKilo, corte.StockKilos, id)
	if err != nil {
		return nil, err
	}
	return carniceria.GetByID(id) // relee la fila para devolver los datos reales guardados
}

// Delete elimina un corte por id.
//
// Nota didáctica: si el corte tiene ventas asociadas, la FK de ventas impide
// el borrado (si las foreign keys están activas), y el error sube al handler.
// En un caso real se preferiría un borrado lógico (columna activo = false)
// para no romper el histórico de ventas. Aquí se implementa el borrado físico
// porque es lo que pide el reto.
func (carniceria *store) Delete(id int) error {
	q := `DELETE from cortes WHERE id = ?`
	_, err := carniceria.db.Exec(q, id)
	if err != nil {
		return err
	}
	return nil
}
