package model

import "time"

type Venta struct {
	ID          int       `json:"id"`
	IDCorte     int       `json:"corte_id"`
	NombreCorte string    `json:"corte_nombre,omitempty"`
	CantidadKg  float64   `json:"cantidad_kg"`
	Total       float64   `json:"total"`
	Fecha       time.Time `json:"fecha"`
}
