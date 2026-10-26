package model

import "time"

// Venta representa una transacción de venta de un corte de carne.
//
// Regla de negocio importante: el campo Total se calcula en el servidor
// (precio_kg del corte × cantidad_kg) en el momento exacto de la venta.
// No se recibe del cliente porque sería manipulable: un cliente podría
// enviar total = 1 y comprar 100 kg. Al guardar el total "congelado" en la
// fila de la venta, el histórico no cambia aunque después suba el precio
// del corte (los reportes siguen siendo correctos).
type Venta struct {
	// ID es la clave primaria autoincremental de la venta.
	ID int `json:"id"`

	// IDCorte es la clave foránea al corte vendido. En JSON se llama
	// "corte_id" para que el cliente sepa a qué corte se refiere.
	// En la base de datos la FK está declarada como
	// REFERENCES cortes(id), lo que impide ventas de cortes inexistentes
	// (cuando las foreign keys están activadas con ?_foreign_keys=on).
	IDCorte int `json:"corte_id"`

	// NombreCorte no es una columna de la tabla ventas: es un campo
	// "derivado" que se rellena con un JOIN al listar ventas para que la
	// respuesta sea más útil (el cliente ve "Lomo fino" en vez de solo el
	// id 1). El tag `omitempty` hace que, si el nombre no se cargó, la clave
	// se omita del JSON en lugar de aparecer como "".
	NombreCorte string `json:"corte_nombre,omitempty"`

	// CantidadKg es cuántos kilogramos se vendieron. Debe ser > 0; esa
	// validación vive en la capa service (service.CrearVenta), no aquí,
	// porque el modelo es solo una estructura de datos.
	CantidadKg float64 `json:"cantidad_kg"`

	// Total es el monto cobrado (precio_kg histórico × CantidadKg).
	Total float64 `json:"total"`

	// Fecha es cuándo ocurrió la venta. Se guarda en UTC para que coincida
	// con el resto de fechas que genera SQLite y para que los reportes por
	// día no dependan de la zona horaria del servidor.
	Fecha time.Time `json:"fecha"`
}
