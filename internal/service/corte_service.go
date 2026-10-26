// Package service contiene la lógica de negocio: las reglas que definen qué
// es válido y qué no, independientemente de HTTP y de SQL.
//
// ¿Por qué una capa intermedia si el handler podría llamar directo al store?
// Por tres razones:
//  1. Reutilización: la regla "precio no negativo" vale para cualquier
//     entrada, venga de HTTP, CLI o un job.
//  2. Testabilidad: se prueba la lógica sin levantar un servidor ni una BD.
//  3. Separación de responsabilidades: el handler se ocupa de HTTP y el
//     store de SQL; aquí viven las reglas del negocio.
//
// Esta capa tampoco conoce códigos HTTP (400, 409...). Solo devuelve errores
// de dominio; el transport decide cómo traducirlos a la respuesta.
package service

import (
	"carniceria-don-juanchito/internal/model"
	"carniceria-don-juanchito/internal/store"
	"errors"
)

// Service agrupa los casos de uso de la aplicación.
//
// Depende de INTERFACES (store.Store, store.VentasStore), no de las
// implementaciones concretas. Eso permite inyectar mocks en tests y cambiar
// la base de datos sin tocar esta capa (principio de inversión de
// dependencias).
type Service struct {
	store  store.Store
	ventas store.VentasStore
}

// New construye el servicio inyectando ambos repositorios.
// Se reciben como parámetros (no se crean aquí) para que main sea el único
// que decide qué implementación concreta se usa. Esto se llama "composition
// root": el punto de entrada arma el grafo de dependencias.
func New(servicio store.Store, ventas store.VentasStore) *Service {
	return &Service{
		store:  servicio,
		ventas: ventas,
	}
}

// ObtenCortes lista cortes aplicando los filtros opcionales.
// El service traduce el filtro "de negocio" (dos valores simples) al struct
// que entiende el repositorio. Así la capa HTTP no depende del paquete store.
func (servicio *Service) ObtenCortes(tipo string, stockBajo bool) ([]*model.Corte, error) {
	return servicio.store.GetAll(store.FiltroCortes{Tipo: tipo, StockBajo: stockBajo})
}

// ObtenCortePorID devuelve un corte o propaga sql.ErrNoRows.
// No se convierte aquí a un error de dominio porque "no encontrado" es un
// concepto universal que el handler sabe mapear a 404.
func (servicio *Service) ObtenCortePorID(id int) (*model.Corte, error) {
	return servicio.store.GetByID(id)
}

// ErrPrecioInvalido es un error de dominio: representa una regla de negocio
// incumplida. Se declara a nivel de paquete y se exporta para que el handler
// pueda identificarlo con errors.Is y responder 400 en lugar de 500.
//
// Un error de dominio NO debe llevar el código HTTP en el mensaje: "el precio
// no puede ser negativo" sirve para HTTP y para cualquier otro consumidor.
var ErrPrecioInvalido = errors.New("el precio por kilo no puede ser negativo")

// CrearCorte valida y persiste un corte nuevo.
//
// Regla del reto: precio y stock no pueden ser negativos. El stock se valida
// aquí porque es una regla del negocio (aunque la columna sea NOT NULL, SQL
// no prohíbe negativos; NOT NULL solo prohíbe NULL).
//
// ¿Por qué se recibe model.Corte por valor y no por puntero?
// Porque es un dato de entrada pequeño que solo se lee; el store recibe un
// puntero y le asigna el ID generado. Pasar por valor evita que la validación
// modifique el original sin querer.
func (servicio *Service) CrearCorte(corte model.Corte) (*model.Corte, error) {
	if corte.PrecioKilo < 0 || corte.StockKilos < 0 {
		return nil, ErrPrecioInvalido
	}
	return servicio.store.Create(&corte)
}

// UpdateAllCorte actualiza precio y stock de un corte existente.
//
// El nombre "All" viene del planteamiento original (reemplazo completo), pero
// el repositorio solo actualiza los campos mutables. Si el id no existe,
// GetByID devolverá sql.ErrNoRows después del UPDATE; el handler podría
// mapearlo a 404 (hoy devuelve 500, una mejora pendiente).
func (servicio *Service) UpdateAllCorte(id int, corte model.Corte) (*model.Corte, error) {
	return servicio.store.Update(id, &corte)
}

// BorrarCorte elimina un corte.
// Como en Update, si el id no existe el DELETE no falla: simplemente afecta
// 0 filas. Para distinguir "no existía" habría que consultar RowsAffected.
func (servicio *Service) BorrarCorte(id int) error {
	return servicio.store.Delete(id)
}
