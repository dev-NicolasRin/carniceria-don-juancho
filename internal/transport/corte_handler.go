// Package transport implementa la capa HTTP: recibe peticiones, valida el
// formato, llama al service y escribe la respuesta con el código adecuado.
//
// Es la única capa que conoce net/http y los códigos de estado. El service
// devuelve errores de dominio y aquí se traducen a HTTP (400, 404, 409...).
// Esa traducción se concentra en los handlers para no contaminar la lógica de
// negocio con detalles del protocolo.
package transport

import (
	"carniceria-don-juanchito/internal/model"
	"carniceria-don-juanchito/internal/service"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

// CorteHandler agrupa los handlers de /cortes.
// Guarda un puntero al service (no lo construye) porque main se lo inyecta;
// así el handler se puede probar con un service real sobre una BD :memory:.
type CorteHandler struct {
	service *service.Service
}

// New construye el handler inyectando el servicio.
func New(servicio *service.Service) *CorteHandler {
	return &CorteHandler{
		service: servicio,
	}
}

// HandleCortes atiende /cortes (colección):
//
//	GET  /cortes?tipo=res&stock_bajo=true -> 200 lista filtrada
//	POST /cortes                           -> 201 corte creado
//
// ¿Por qué un solo handler con switch en vez de uno por método?
// Porque comparten la misma ruta. Podría usarse el enrutamiento por método de
// Go 1.22 ("GET /cortes"), pero aquí se mantiene el switch para que se vea
// explícitamente cómo se discrimina el verbo HTTP. Con switch, un método no
// soportado cae en default y responde 405 Method Not Allowed.
func (handler *CorteHandler) HandleCortes(writer http.ResponseWriter, reader *http.Request) {
	switch reader.Method {
	case http.MethodGet:
		// URL.Query() parsea el query string (?tipo=res&stock_bajo=true).
		// Get devuelve "" si el parámetro no está, que para nosotros
		// significa "sin filtro".
		tipo := reader.URL.Query().Get("tipo")
		// ParseBool acepta "true", "1", "t", etc. Se ignora el error con _
		// a propósito: un valor inválido (ej. ?stock_bajo=hola) se trata
		// como false en vez de responder 400. Es una decisión pragmática;
		// una API estricta podría validarlo.
		stockBajo, _ := strconv.ParseBool(reader.URL.Query().Get("stock_bajo"))
		cortes, err := handler.service.ObtenCortes(tipo, stockBajo)
		if err != nil {
			// 500 porque un fallo de BD no es culpa del cliente.
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		responderJSON(writer, http.StatusOK, cortes)
	case http.MethodPost:
		// json.NewDecoder lee el body como stream y lo deserializa en el
		// struct. Decode devuelve error si el JSON está mal formado o los
		// tipos no coinciden -> 400 Bad Request (el cliente envió algo
		// inválido).
		var corte model.Corte
		if err := json.NewDecoder(reader.Body).Decode(&corte); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		creado, err := handler.service.CrearCorte(corte)
		if err != nil {
			// Aquí el error es de dominio (ErrPrecioInvalido): 400.
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		// 201 Created: el recurso se creó y se devuelve su representación.
		responderJSON(writer, http.StatusCreated, creado)
	default:
		http.Error(writer, "Metodo no disponible", http.StatusMethodNotAllowed)
	}
}

// HandleCorteByID atiende /cortes/{id}:
//
//	GET    -> 200 con el corte
//	PUT    -> 200 con el corte actualizado
//	DELETE -> 204 sin contenido
//
// El id llega en la URL y hay que convertirlo de string a int con Atoi.
// Si no es un número, se responde 400 (petición mal formada).
func (handler *CorteHandler) HandleCorteByID(writer http.ResponseWriter, reader *http.Request) {
	// TrimPrefix quita el prefijo literal "/cortes/" de la ruta.
	// Ej.: "/cortes/7" -> "7". Si la ruta fuera anidada ("/cortes/7/algo")
	// esto fallaría en Atoi -> 400. Para una API más robusta se usaría el
	// patrón "GET /cortes/{id}" de Go 1.22 y r.PathValue("id").
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
			// errors.Is compara la cadena de errores. sql.ErrNoRows significa
			// "la consulta no devolvió filas" -> el recurso no existe -> 404.
			// Cualquier otro error (BD caída, tabla corrupta) es 500.
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(writer, "el corte no existe", http.StatusNotFound)
				return
			}
			http.Error(writer, err.Error(), http.StatusInternalServerError)
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
		// 204 No Content: se borró correctamente y no hay cuerpo que devolver.
		writer.WriteHeader(http.StatusNoContent)
	default:
		http.Error(writer, "metodo no disponible", http.StatusMethodNotAllowed)
	}
}
