package model

// ReporteVentasDia es un modelo de solo lectura (un DTO, Data Transfer
// Object): no representa una tabla, sino el resultado de una consulta
// agregada. Existe para darle una forma tipada y estable a la respuesta de
// GET /reportes/ventas-del-dia.
//
// ¿Por qué un struct y no devolver directamente un map o una tupla?
// Porque el handler y los tests necesitan campos con nombre y tipo; un
// struct deja claro el contrato de la respuesta y permite decodificarlo
// con json.NewDecoder sin adivinar claves.
type ReporteVentasDia struct {
	// Fecha es el día consultado en formato "YYYY-MM-DD" (UTC).
	Fecha string `json:"fecha"`

	// CantidadVentas es el número de ventas registradas ese día.
	CantidadVentas int `json:"cantidad_ventas"`

	// Total es la suma de los montos vendidos ese día. COALESCE en la
	// consulta SQL garantiza que sea 0 y no NULL cuando no hay ventas.
	Total float64 `json:"total"`
}
