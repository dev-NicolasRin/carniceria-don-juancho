package service

import (
	"carniceria-don-juanchito/internal/model"
	"errors"
)

// ErrCantidadInvalida es un error de dominio para ventas con cantidad <= 0.
// Igual que ErrPrecioInvalido, el handler lo traduce a HTTP 400 Bad Request.
// Se usa errors.New (no fmt.Errorf) porque no necesita formato dinámico y así
// es comparable con errors.Is.
var ErrCantidadInvalida = errors.New("la cantidad en kilos debe ser mayor que cero")

// ObtenTodasLasVentas devuelve el histórico completo de ventas.
// La capa service no agrega lógica aquí: es un "passthrough". Se mantiene el
// método para no acoplar el transport al store y para poder añadir reglas
// (paginación, permisos) en el futuro sin cambiar a los consumidores.
func (servicio *Service) ObtenTodasLasVentas() ([]*model.Venta, error) {
	return servicio.ventas.GetAllVentas()
}

// ObtenVentasDelDia devuelve el resumen de ventas del día actual.
// El cálculo (SUM, COUNT) se hace en la base de datos, que está optimizada
// para agregar; traer todas las filas a Go para sumarlas sería lento y
// consumiría memoria innecesariamente.
func (servicio *Service) ObtenVentasDelDia() (*model.ReporteVentasDia, error) {
	return servicio.ventas.VentasDelDia()
}

// CrearVenta valida la entrada y delega la transacción al repositorio.
//
// ¿Por qué se reciben idCorte y cantidadKg como parámetros sueltos en vez de
// un model.Venta completo?
// Porque el cliente no debe poder definir campos calculados (Total, Fecha, ID).
// Al aceptar solo lo que el usuario puede decidir, la API es más segura: no
// hay forma de inyectar un total arbitrario.
//
// La validación de stock NO se hace aquí sino en store.CreateVenta, porque
// necesita leer y actualizar la base de datos dentro de la misma transacción.
// Hacerlo en service abriría una ventana de carrera entre la comprobación y
// el descuento.
func (servicio *Service) CrearVenta(idCorte int, cantidadKg float64) (*model.Venta, error) {
	if cantidadKg <= 0 {
		return nil, ErrCantidadInvalida
	}
	return servicio.ventas.CreateVenta(&model.Venta{
		IDCorte:    idCorte,
		CantidadKg: cantidadKg,
	})
}
