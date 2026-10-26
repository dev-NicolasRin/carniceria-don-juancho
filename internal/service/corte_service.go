package service

import (
	"carniceria-don-juanchito/internal/model"
	"carniceria-don-juanchito/internal/store"
	"errors"
)

type Service struct {
	store  store.Store
	ventas store.VentasStore
}

func New(servicio store.Store, ventas store.VentasStore) *Service {
	return &Service{
		store:  servicio,
		ventas: ventas,
	}
}

func (servicio *Service) ObtenTodosLosCortes() ([]*model.Corte, error) {
	return servicio.store.GetAll()
}

func (servicio *Service) ObtenCortePorID(id int) (*model.Corte, error) {
	return servicio.store.GetByID(id)
}

var ErrPrecioInvalido = errors.New("el precio por kilo no puede ser negativo") //al declararlo como una variable el handler puede distinguir el error

func (servicio *Service) CrearCorte(corte model.Corte) (*model.Corte, error) {
	if corte.PrecioKilo < 0 || corte.StockKilos < 0 {
		return nil, ErrPrecioInvalido
	}
	return servicio.store.Create(&corte)
}

func (servicio *Service) UpdateAllCorte(id int, corte model.Corte) (*model.Corte, error) {
	return servicio.store.Update(id, &corte)
}

func (servicio *Service) BorrarCorte(id int) error {
	return servicio.store.Delete(id)
}
