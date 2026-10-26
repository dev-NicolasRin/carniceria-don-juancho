package transport

import (
	"carniceria-don-juanchito/internal/service"
	"carniceria-don-juanchito/internal/store"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
)

type VentaHandler struct {
	service *service.Service
}

func NewVentaHandler(servicio *service.Service) *VentaHandler {
	return &VentaHandler{
		service: servicio,
	}
}

type crearVentaInput struct {
	CorteID    int     `json:"corte_id"`
	CantidadKg float64 `json:"cantidad_kg"`
}

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
			switch {
			case errors.Is(err, service.ErrCantidadInvalida):
				http.Error(writer, err.Error(), http.StatusBadRequest)
			case errors.Is(err, sql.ErrNoRows):
				http.Error(writer, "el corte no existe", http.StatusNotFound)
			case errors.Is(err, store.ErrStockInsuficiente):
				http.Error(writer, err.Error(), http.StatusConflict)
			default:
				http.Error(writer, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		responderJSON(writer, http.StatusCreated, venta)
	default:
		http.Error(writer, "metodo no disponible", http.StatusMethodNotAllowed)
	}
}

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

func responderJSON(writer http.ResponseWriter, status int, cuerpo any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	json.NewEncoder(writer).Encode(cuerpo)
}
