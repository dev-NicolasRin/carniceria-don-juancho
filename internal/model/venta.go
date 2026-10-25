package model

import "time"

type Venta struct {
	ID         int       `json:"id"`
	IDCorte    int       `json:"corte_id"`
	CantidadKg float64   `json:"cantidad_kg"`
	Total      float64   `json:"total"`
	Fecha      time.Time `json:"fecha"`
}
