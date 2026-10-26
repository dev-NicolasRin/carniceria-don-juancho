// Package model contiene las estructuras de datos (los "modelos") que
// representan las entidades del negocio: cortes de carne, ventas y reportes.
//
// ¿Por qué existe este paquete y no se usan map[string]any?
// Porque un struct tipado:
//   - Hace que el compilador detecte errores (ej. escribir corte.PresioKilo).
//   - Documenta por sí solo qué campos tiene una entidad.
//   - Permite que encoding/json convierta a/desde JSON de forma declarativa
//     usando los tags `json:"..."`, sin código manual.
//
// Este paquete NO sabe nada de HTTP ni de SQL: solo describe datos. Esa
// separación es clave para poder cambiar la base de datos o el transporte
// sin tocar los modelos.
package model

import "time"

// Corte representa un tipo de carne que la carnicería tiene disponible.
// Es la unidad central del inventario: cada corte tiene un precio por kilo
// y un stock expresado en kilos.
type Corte struct {
	// ID es la clave primaria. La genera SQLite (AUTOINCREMENT) al insertar,
	// por eso en un POST el cliente no debe enviarlo: se ignora y se rellena
	// después con el valor real devuelto por la base de datos.
	ID int `json:"id"`

	// Nombre es el nombre comercial del corte (ej. "Lomo fino").
	// El tag `json:"nombre"` indica que en JSON la clave se llama "nombre"
	// en minúscula, tal como es habitual en las APIs REST.
	Nombre string `json:"nombre"`

	// TipoAnimal indica de qué animal proviene el corte: res, cerdo, pollo
	// o cordero. En JSON viaja como "tipo_animal" (snake_case, el estilo
	// dominante en los nombres de columnas SQL).
	TipoAnimal string `json:"tipo_animal"`

	// PrecioKilo es el precio por kilogramo en la moneda del negocio.
	// Se usa float64 (no int) porque los precios pueden tener decimales.
	// Nota didáctica: para dinero real convendría un tipo decimal o guardar
	// centavos como entero para evitar errores de redondeo binario; para el
	// alcance de este reto float64 es suficiente.
	PrecioKilo float64 `json:"precio_kg"`

	// StockKilos es la cantidad disponible en kilogramos. Al registrar una
	// venta se descuenta dentro de una transacción (ver store.CreateVenta).
	StockKilos float64 `json:"stock_kg"`

	// FechaCreacion la genera SQLite con DEFAULT CURRENT_TIMESTAMP.
	// time.Time le dice a encoding/json que serialice el valor como una
	// fecha ISO 8601 (RFC 3339), por ejemplo "2026-10-26T05:23:17Z".
	// Se rellena al releer la fila después del INSERT (ver store.Create).
	FechaCreacion time.Time `json:"fecha_creacion"`
}
