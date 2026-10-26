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
	q := "SELECT id, nombre, tipo_animal, precio_kg, stock_kg, fecha_creacion FROM cortes"
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

func (carniceria *store) GetByID(id int) (*model.Corte, error) {
	q := "SELECT id, nombre, tipo_animal, precio_kg, stock_kg, fecha_creacion FROM cortes WHERE id = ?"
	corte_encontrado := model.Corte{}
	err := carniceria.db.QueryRow(q, id).Scan(&corte_encontrado.ID, &corte_encontrado.Nombre, &corte_encontrado.TipoAnimal,
		&corte_encontrado.PrecioKilo, &corte_encontrado.StockKilos, &corte_encontrado.FechaCreacion)
	if err != nil {
		return nil, err
	}
	return &corte_encontrado, nil

}

func (carniceria *store) Create(corte *model.Corte) (*model.Corte, error) {
	q := "INSERT INTO cortes (nombre, tipo_animal, precio_kg, stock_kg) VALUES (?, ?, ?, ?)"
	resp, err := carniceria.db.Exec(q, corte.Nombre, corte.TipoAnimal, corte.PrecioKilo, corte.StockKilos)
	if err != nil {
		return nil, err
	}
	id, err := resp.LastInsertId()
	if err != nil {
		return nil, err
	}
	corte.ID = int(id)
	return carniceria.GetByID(corte.ID) // relee la fila para traer la fecha de creacion generada por SQLite
}

func (carniceria *store) Update(id int, corte *model.Corte) (*model.Corte, error) {
	q := "UPDATE cortes SET precio_kg = ?, stock_kg = ? WHERE id = ?"
	_, err := carniceria.db.Exec(q, corte.PrecioKilo, corte.StockKilos, id)
	if err != nil {
		return nil, err
	}
	return carniceria.GetByID(id) // relee la fila para devolver los datos reales guardados
}

func (carniceria *store) Delete(id int) error {
	q := `DELETE from cortes WHERE id = ?`
	_, err := carniceria.db.Exec(q, id)
	if err != nil {
		return err
	}
	return nil
}
