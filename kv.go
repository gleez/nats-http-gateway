package natshttp

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

// handleKvOperation processes KV API requests (GET, POST, PUT, DELETE) per the OpenAPI spec.
func (h *Handler) handleKvOperation(w http.ResponseWriter, r *http.Request) {
	// Expected path: /kv/{bucket}/{key}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/kv/"), "/")
	if len(parts) < 2 {
		WriteJSONError(w, http.StatusBadRequest, "Bucket or key missing")
		return
	}
	bucket, key := parts[0], parts[1]

	js, err := h.nc.JetStream()
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	kv, err := js.KeyValue(bucket)
	if err != nil {
		WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	query := r.URL.Query()
	switch r.Method {
	case http.MethodGet:
		// Optional revision parameter.
		var entry nats.KeyValueEntry
		if revStr := query.Get("revision"); revStr != "" {
			rev, _ := strconv.ParseUint(revStr, 10, 64)
			entry, err = kv.GetRevision(key, rev)
		} else {
			entry, err = kv.Get(key)
		}
		if err != nil {
			if err == nats.ErrKeyNotFound {
				WriteJSONError(w, http.StatusNotFound, "Key not found")
			} else {
				WriteJSONError(w, http.StatusBadRequest, err.Error())
			}
			return
		}
		// Determine response format based on Accept header.
		accept := r.Header.Get("Accept")
		if strings.Contains(accept, "application/text") {
			// Text response – metadata in headers, raw value in body.
			w.Header().Set("Content-Type", "application/text")
			w.Header().Set("X-Nats-Kv-Bucket", bucket)
			w.Header().Set("X-Nats-Kv-Created", entry.Created().Format(time.RFC3339))
			w.Header().Set("X-Nats-Kv-Revision", strconv.FormatUint(entry.Revision(), 10))
			w.Header().Set("X-Nats-Kv-Delta", strconv.FormatUint(entry.Delta(), 10))
			w.Header().Set("X-Nats-Kv-Operation", entry.Operation().String())
			w.Write(entry.Value())
		} else {
			// JSON response – include metadata and base64‑encoded value.
			resp := map[string]interface{}{
				"bucket":    bucket,
				"key":       key,
				"revision":  entry.Revision(),
				"delta":     entry.Delta(),
				"operation": entry.Operation().String(),
				"value":     base64.StdEncoding.EncodeToString(entry.Value()),
			}
			WriteJSONResponse(w, r, resp)
		}
	case http.MethodPost:
		// Create entry – fail if it already exists.
		body, err := readRequestBody(r)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusBadRequest)
			return
		}
		rev, err := kv.Create(key, body)
		if err != nil {
			WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		// Determine response format based on Accept header.
		accept := r.Header.Get("Accept")
		if strings.Contains(accept, "application/text") {
			w.Header().Set("Content-Type", "application/text")
			w.Header().Set("X-Nats-Kv-Sequence", strconv.FormatUint(rev, 10))
			w.WriteHeader(http.StatusCreated)
			// No body for text response per spec.
		} else {
			w.WriteHeader(http.StatusCreated)
			WriteJSONResponse(w, r, map[string]uint64{"revision": rev})
		}
	case http.MethodPut:
		// Add or update – optional previousRevision for CAS.
		body, err := readRequestBody(r)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusBadRequest)
			return
		}
		var rev uint64
		if prev := query.Get("previousRevision"); prev != "" {
			prevRev, _ := strconv.ParseUint(prev, 10, 64)
			rev, err = kv.Update(key, body, prevRev)
		} else {
			rev, err = kv.Put(key, body)
		}
		if err != nil {
			WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		// Determine response format based on Accept header.
		accept := r.Header.Get("Accept")
		if strings.Contains(accept, "application/text") {
			w.Header().Set("Content-Type", "application/text")
			w.Header().Set("X-Nats-Kv-Sequence", strconv.FormatUint(rev, 10))
			w.WriteHeader(http.StatusOK)
			// No body for text response per spec.
		} else {
			WriteJSONResponse(w, r, map[string]uint64{"revision": rev})
		}
	case http.MethodDelete:
		// Optional purge query parameter.
		purge := query.Get("purge")
		if purge == "true" || purge == "1" {
			err = kv.Purge(key)
		} else {
			err = kv.Delete(key)
		}
		if err != nil {
			WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
	}
}
