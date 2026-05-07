package natshttp

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

// handleObjOperation processes ObjectStore API requests (GET, PUT, DELETE) per the OpenAPI spec.
func (h *Handler) handleObjOperation(w http.ResponseWriter, r *http.Request) {
	// Expected path: /obj/{bucket}/{key}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/obj/"), "/")
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
	store, err := js.ObjectStore(bucket)
	if err != nil {
		WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// metaOnly query param for GET.
	metaOnly := r.URL.Query().Get("metaOnly") == "true"

	switch r.Method {
	case http.MethodGet:
		// Retrieve the object.
		obj, err := store.Get(key)
		if err != nil {
			if err == nats.ErrKeyNotFound {
				WriteJSONError(w, http.StatusNotFound, "Object not found")
			} else {
				WriteJSONError(w, http.StatusBadRequest, err.Error())
			}
			return
		}
		// Obtain metadata.
		info, err := obj.Info()
		if err != nil {
			WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Populate headers with metadata.
		w.Header().Set("X-Nats-Obj-Name", info.Name)
		w.Header().Set("X-Nats-Obj-Bucket", info.Bucket)
		w.Header().Set("X-Nats-Obj-Nuid", info.NUID)
		w.Header().Set("X-Nats-Obj-Size", strconv.FormatUint(info.Size, 10))
		w.Header().Set("X-Nats-Obj-Mod-Time", info.ModTime.Format(time.RFC3339))
		w.Header().Set("X-Nats-Obj-Chunks", strconv.FormatUint(uint64(info.Chunks), 10))
		w.Header().Set("X-Nats-Obj-Digest", info.Digest)
		if info.Description != "" {
			w.Header().Set("X-Nats-Obj-Description", info.Description)
		}
		if info.Opts != nil && info.Opts.ChunkSize > 0 {
			w.Header().Set("X-Nats-Obj-Max-Chunk-Size", strconv.FormatUint(uint64(info.Opts.ChunkSize), 10))
		}
		if info.Opts != nil && info.Opts.Link != nil {
			w.Header().Set("X-Nats-Obj-Link-Bucket", info.Opts.Link.Bucket)
			w.Header().Set("X-Nats-Obj-Link-Name", info.Opts.Link.Name)
		}
		if info.Deleted {
			w.Header().Set("X-Nats-Obj-Deleted", "true")
		}
		if metaOnly {
			w.WriteHeader(http.StatusOK)
			return
		}
		// Stream the payload.
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = io.Copy(w, obj)
	case http.MethodPut:
		// Store or update the object.
		_, err := store.Put(&nats.ObjectMeta{Name: key}, r.Body)
		if err != nil {
			WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}

		rev := uint64(0000) // @TODO get revision number

		// Choose response format based on Accept header.
		accept := r.Header.Get("Accept")
		if strings.Contains(accept, "application/text") {
			w.Header().Set("Content-Type", "application/text")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(strconv.FormatUint(rev, 10)))
		} else {
			WriteJSONResponse(w, r, map[string]uint64{"revision": rev})
		}
	case http.MethodDelete:
		// Delete the object.
		if err := store.Delete(key); err != nil {
			WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
	}
}
