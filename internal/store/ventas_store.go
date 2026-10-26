package store

import (
	"carniceria-don-juanchito/internal/model"
	"database/sql"
	"errors"
	"time"
)

var ErrStockInsuficiente = errors.New("no hay suficiente stock para la venta")

type VentasStore interface {
	GetAllVentas() ([]*model.Venta, error)
	CreateVenta(venta *model.Venta) (*model.Venta, error)
}

type ventasStore struct {
	db *sql.DB
}

func NewVentas(db *sql.DB) VentasStore {
	return &ventasStore{db: db}
}

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

func (ventas *ventasStore) CreateVenta(venta *model.Venta) (*model.Venta, error) {
	tx, err := ventas.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() // si algo falla antes del Commit, la venta no queda registrada

	var precioKilo, stockKilos float64
	err = tx.QueryRow("SELECT precio_kg, stock_kg FROM cortes WHERE id = ?", venta.IDCorte).
		Scan(&precioKilo, &stockKilos)
	if err != nil {
		return nil, err // sql.ErrNoRows si el corte no existe
	}

	if stockKilos < venta.CantidadKg {
		return nil, ErrStockInsuficiente
	}

	venta.Total = precioKilo * venta.CantidadKg
	venta.Fecha = time.Now()

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

	if _, err := tx.Exec(
		"UPDATE cortes SET stock_kg = stock_kg - ? WHERE id = ?",
		venta.CantidadKg, venta.IDCorte,
	); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	venta.ID = int(id)
	return venta, nil
}
