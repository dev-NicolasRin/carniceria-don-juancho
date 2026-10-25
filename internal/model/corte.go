package model

import "time"

type Corte struct {
	ID            int       `json:"id"`
	Nombre        string    `json:"nombre"`
	TipoAnimal    string    `json:"tipo_animal"`
	PrecioKilo    float64   `json:"precio_kg"`
	StockKilos    float64   `json:"stock_kg"`
	FechaCreacion time.Time `json:"creado_el"`
}
