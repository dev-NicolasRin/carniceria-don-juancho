package transport

import (
	"carniceria-don-juanchito/internal/service"
	"carniceria-don-juanchito/internal/store"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
)

// VentaHandler agrupa los handlers de /ventas y del reporte diario.
type VentaHandler struct {
	service *service.Service
}

// NewVentaHandler construye el handler con el servicio inyectado.
func NewVentaHandler(servicio *service.Service) *VentaHandler {
	return &VentaHandler{
		service: servicio,
	}
}

// crearVentaInput es un DTO (Data Transfer Object) específico del transporte.
//
// ¿Por qué no usar model.Venta directamente para decodificar el body?
// Porque model.Venta incluye ID, Total y Fecha, que el cliente NO debe
// controlar. Al definir un struct solo con los campos permitidos, cualquier
// campo extra del JSON se ignora y es imposible inyectar un total falso.
// Es una aplicación del principio de mínima exposición.
type crearVentaInput struct {
	CorteID    int     `json:"corte_id"`
	CantidadKg float64 `json:"cantidad_kg"`
}

// HandleVentas atiende /ventas:
//
//	GET  -> 200 con el histórico (incluye nombre del corte)
//	POST -> 201 con la venta registrada
func (handler *VentaHandler) HandleVentas(writer http.ResponseWriter, reader *http.Request) {
	switch reader.Method {
	case http.MethodGet:
		ventas, err := handler.service.ObtenTodasLasVentas()
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		responderJSON(writer, http.StatusOK, ventas)
	case http.MethodPost:
		var input crearVentaInput
		if err := json.NewDecoder(reader.Body).Decode(&input); err != nil {
			http.Error(writer, "input invalido", http.StatusBadRequest)
			return
		}

		venta, err := handler.service.CrearVenta(input.CorteID, input.CantidadKg)
		if err != nil {
			// Traducción de errores de dominio a códigos HTTP. ¿Por qué aquí
			// y no en el service? Porque el service no debe saber de HTTP:
			// así el mismo error podría mapearse a otro protocolo o a una
			// respuesta de CLI sin cambios.
			switch {
			case errors.Is(err, service.ErrCantidadInvalida):
				// El cliente envió cantidad <= 0.
				http.Error(writer, err.Error(), http.StatusBadRequest)
			case errors.Is(err, sql.ErrNoRows):
				// El corte referenciado no existe.
				http.Error(writer, "el corte no existe", http.StatusNotFound)
			case errors.Is(err, store.ErrStockInsuficiente):
				// Conflicto con el estado actual del recurso: hay menos stock
				// que el solicitado. 409 Conflict es el código correcto según
				// el enunciado; 400 sería incorrecto porque la petición está
				// bien formada y 422 no aplica a este caso.
				http.Error(writer, err.Error(), http.StatusConflict)
			default:
				// Cualquier otro error (BD caída, transacción fallida) = 500.
				http.Error(writer, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		responderJSON(writer, http.StatusCreated, venta)
	default:
		http.Error(writer, "metodo no disponible", http.StatusMethodNotAllowed)
	}
}

// HandleVentasDelDia atiende GET /reportes/ventas-del-dia.
// Solo acepta GET; cualquier otro método responde 405.
func (handler *VentaHandler) HandleVentasDelDia(writer http.ResponseWriter, reader *http.Request) {
	if reader.Method != http.MethodGet {
		http.Error(writer, "metodo no disponible", http.StatusMethodNotAllowed)
		return
	}

	reporte, err := handler.service.ObtenVentasDelDia()
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	responderJSON(writer, http.StatusOK, reporte)
}

// responderJSON centraliza la escritura de respuestas JSON.
//
// ¿Por qué un helper? Para garantizar dos cosas que son fáciles de olvidar:
//  1. El header Content-Type debe establecerse ANTES de WriteHeader; una vez
//     enviado el status, los headers ya no se pueden modificar. Si se pone
//     después, el cliente recibe el JSON sin el tipo correcto.
//  2. WriteHeader fija el código de estado. Si no se llama, Go responde 200
//     por defecto.
//
// json.NewEncoder(writer).Encode serializa directamente al ResponseWriter,
// sin crear un buffer intermedio. Si la codificación fallara a mitad, ya no
// se puede cambiar el status; en ese caso lo correcto sería serializar a un
// buffer primero y solo escribir si no hubo error. Para este proyecto la
// simplicidad pesa más y los datos son siempre serializables.
func responderJSON(writer http.ResponseWriter, status int, cuerpo any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	json.NewEncoder(writer).Encode(cuerpo)
}
