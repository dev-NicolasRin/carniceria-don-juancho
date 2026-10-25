package store

import (
	"carniceria-don-juanchito/internal/model"
	"database/sql"
)

type Store interface {
	GetAll() ([]*model.Corte, error)
	GetByID(id int) (*model.Corte, error)
	Create(book *model.Corte) (*model.Corte, error)
	Update(id int, book *model.Corte) (*model.Corte, error)
	Delete(id int) error
}

type store struct {
	db *sql.DB
}

func New(db *sql.DB) Store {
	return &store{db: db}
}

func (carniceria *store) GetAll() ([]*model.Corte, error) {
	q := "SELECT id, nombre, tipo_animal, precio_kg, stock kg, fecha_creacion"
	rows, err := carniceria.db.Query(q)

	if err != nil {
		return nil, err
	}
	defer rows.Close() //cierra la conexion a la db

	var Cortes []*model.Corte
	for rows.Next() {
		corte_actual := model.Corte{}
		if err := rows.Scan(&corte_actual.ID, &corte_actual.Nombre, &corte_actual.TipoAnimal,
			&corte_actual.PrecioKilo, &corte_actual.StockKilos, &corte_actual.FechaCreacion); err != nil {
			return nil, err
		}
		Cortes = append(Cortes, &corte_actual)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return Cortes, nil
}
