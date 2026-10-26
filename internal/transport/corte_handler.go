package transport

import (
	"carniceria-don-juanchito/internal/model"
	"carniceria-don-juanchito/internal/service"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type CorteHandler struct {
	service *service.Service
}

func New(servicio *service.Service) *CorteHandler {
	return &CorteHandler{
		service: servicio,
	}
}

func (handler *CorteHandler) HandleCortes(writer http.ResponseWriter, reader *http.Request) {
	switch reader.Method {
	case http.MethodGet:
		cortes, err := handler.service.ObtenTodosLosCortes()
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		responderJSON(writer, http.StatusOK, cortes)
	case http.MethodPost:
		var corte model.Corte
		if err := json.NewDecoder(reader.Body).Decode(&corte); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		creado, err := handler.service.CrearCorte(corte)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		responderJSON(writer, http.StatusCreated, creado)
	default:
		http.Error(writer, "Metodo no disponible", http.StatusMethodNotAllowed)
	}
}

func (handler *CorteHandler) HandleCorteByID(writer http.ResponseWriter, reader *http.Request) {
	idStr := strings.TrimPrefix(reader.URL.Path, "/cortes/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(writer, "no lo encontre", http.StatusBadRequest)
		return
	}

	switch reader.Method {
	case http.MethodGet:
		corte, err := handler.service.ObtenCortePorID(id)
		if err != nil {
			http.Error(writer, "no se encontro jeje", http.StatusBadRequest)
			return
		}
		responderJSON(writer, http.StatusOK, corte)
	case http.MethodPut:
		var corte model.Corte
		if err := json.NewDecoder(reader.Body).Decode((&corte)); err != nil {
			http.Error(writer, "input invalido", http.StatusBadRequest)
			return
		}
		updated, err := handler.service.UpdateAllCorte(id, corte)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		responderJSON(writer, http.StatusOK, updated)
	case http.MethodDelete:
		err := handler.service.BorrarCorte(id)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	default:
		http.Error(writer, "metodo no disponible", http.StatusMethodNotAllowed)
	}
}
