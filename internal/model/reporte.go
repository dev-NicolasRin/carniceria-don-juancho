package model

type ReporteVentasDia struct {
	Fecha          string  `json:"fecha"`
	CantidadVentas int     `json:"cantidad_ventas"`
	Total          float64 `json:"total"`
}
