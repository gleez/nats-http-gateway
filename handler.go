package natshttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/nats-io/nats.go"
)

// Handler provides HTTP to NATS bridging.
type Handler struct {
	nc *nats.Conn
}

func New(nc *nats.Conn) *Handler {
	return &Handler{nc}
}

// NatsHandler routes HTTP requests to appropriate handlers based on path prefixes.
func (h *Handler) NatsHandler(w http.ResponseWriter, r *http.Request) {
	// KV API routes.
	if strings.HasPrefix(r.URL.Path, "/kv/") {
		h.handleKvOperation(w, r)
		return
	}
	// Core NATS subjects routes (e.g., "/nats/subjects/", "/v1/nats/subjects/" or the full API prefix).
	if strings.HasPrefix(r.URL.Path, "/nats/subjects/") || strings.HasPrefix(r.URL.Path, "/v1/nats/subjects/") || strings.HasPrefix(r.URL.Path, "/api/v1/nats/subjects/") {
		h.handleCoreMessage(w, r)
		return
	}
	// Fallback for unknown paths.
	http.Error(w, "Invalid path", http.StatusNotFound)
}

// Error struct for JSON error responses.
type Error struct {
	Message string `json:"message"`
}

// WriteJSONError writes an error response as JSON.
func WriteJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	payload, err := json.Marshal(Error{Message: message})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"code": 500, "message": "Could not write response"}`))
		return
	}
	w.WriteHeader(statusCode)
	w.Write(payload)
}

// WriteJSONResponse writes a successful NATS response as JSON.
func WriteJSONResponse(w http.ResponseWriter, r *http.Request, result interface{}) {
	body, err := json.Marshal(result)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(prettyJSON(body))
}

// prettyJSON formats JSON with indentation.
func prettyJSON(b []byte) []byte {
	var out bytes.Buffer
	json.Indent(&out, b, "", "  ")
	return out.Bytes()
}
