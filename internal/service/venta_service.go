package service

import (
	"carniceria-don-juanchito/internal/model"
	"errors"
)

var ErrCantidadInvalida = errors.New("la cantidad en kilos debe ser mayor que cero")

func (servicio *Service) ObtenTodasLasVentas() ([]*model.Venta, error) {
	return servicio.ventas.GetAllVentas()
}

func (servicio *Service) ObtenVentasDelDia() (*model.ReporteVentasDia, error) {
	return servicio.ventas.VentasDelDia()
}

func (servicio *Service) CrearVenta(idCorte int, cantidadKg float64) (*model.Venta, error) {
	if cantidadKg <= 0 {
		return nil, ErrCantidadInvalida
	}
	return servicio.ventas.CreateVenta(&model.Venta{
		IDCorte:    idCorte,
		CantidadKg: cantidadKg,
	})
}
