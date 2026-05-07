package natshttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/nats-io/nats.go"
)

// readRequestBody reads the request body safely.
func readRequestBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

// handleCoreMessage delegates core NATS subject operations to the existing handlers.
func (h *Handler) handleCoreMessage(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleNatsSubscribe(w, r)
	case http.MethodPost:
		h.handleNatsReq(w, r)
	case http.MethodPut:
		h.handleNatsPublish(w, r)
	default:
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
	}
}

// handleNatsReq makes a request to NATS and returns the response as JSON.
func (h *Handler) handleNatsReq(w http.ResponseWriter, r *http.Request) {
	body, err := readRequestBody(r)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}
	query := r.URL.Query()
	reply := query.Get("reply")
	timeout := getTimeout(query)
	subj := getNatsSubject(w, r)
	if subj == "" {
		return // getNatsSubject already wrote error response
	}
	hdrs := getNatsHeaders(r.Header)
	res, err := h.nc.RequestMsg(NewNatsMsg(subj, reply, hdrs, body), timeout)
	if err != nil {
		if err == nats.ErrTimeout {
			WriteJSONError(w, http.StatusGatewayTimeout, "Request timed out")
			return
		}
		WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	WriteJSONResponse(w, r, res)
}

// handleNatsPublish publishes a message with an optional reply subject.
func (h *Handler) handleNatsPublish(w http.ResponseWriter, r *http.Request) {
	body, err := readRequestBody(r)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}
	query := r.URL.Query()
	reply := query.Get("reply")
	hdrs := getNatsHeaders(r.Header)
	subj := getNatsSubject(w, r)
	if subj == "" {
		return
	}
	if err := h.nc.PublishMsg(NewNatsMsg(subj, reply, hdrs, body)); err != nil {
		if err == nats.ErrTimeout {
			WriteJSONError(w, http.StatusGatewayTimeout, "Request timed out")
			return
		}
		WriteJSONError(w, http.StatusBadRequest, "Unable to publish")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
}

// handleNatsSubscribe streams NATS messages to the HTTP client as Server‑Sent Events.
func (h *Handler) handleNatsSubscribe(w http.ResponseWriter, r *http.Request) {
	subject := getNatsSubject(w, r)
	if subject == "" {
		return
	}
	query := r.URL.Query()
	event := make(chan *nats.Msg, 10)
	sub, err := h.nc.ChanSubscribe(subject, event)
	if err != nil {
		WriteJSONError(w, http.StatusBadRequest, "Unable to subscribe")
		return
	}
	defer sub.Unsubscribe()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	clientGone := r.Context().Done()
	timeout := time.After(getTimeout(query))
	for {
		select {
		case ev := <-event:
			var buf bytes.Buffer
			if err := json.NewEncoder(&buf).Encode(ev); err != nil {
				fmt.Fprint(w, "data: error encoding message\n\n")
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", buf.String())
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-clientGone:
			fmt.Println("Client disconnected")
			return
		case <-timeout:
			fmt.Fprint(w, ": nothing to send, connection closing\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			return
		}
	}
}

// getNatsSubject extracts the subject from the URL path.
func getNatsSubject(w http.ResponseWriter, r *http.Request) string {
	subject := path.Base(r.URL.Path)
	if subject == "" {
		WriteJSONError(w, http.StatusBadRequest, "Subject not found")
		return ""
	}
	return subject
}

// getTimeout extracts the timeout query parameter (in milliseconds).
func getTimeout(query url.Values) time.Duration {
	const defaultTimeout = 2000 * time.Millisecond
	if t := query.Get("timeout"); t != "" {
		if ms, err := strconv.ParseInt(t, 10, 64); err == nil {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return defaultTimeout
}

// getNatsHeaders converts HTTP headers prefixed with "NatsH-" to NATS headers.
func getNatsHeaders(httpHeaders http.Header) nats.Header {
	natsHeaders := nats.Header{}
	for key, values := range httpHeaders {
		if strings.HasPrefix(key, "NatsH-") {
			natsKey := firstLetterToLower(strings.TrimPrefix(key, "NatsH-"))
			natsHeaders.Add(natsKey, values[0])
		}
	}
	return natsHeaders
}

// firstLetterToLower lower‑cases the first rune of a string.
func firstLetterToLower(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

// NewNatsMsg creates a NATS message.
func NewNatsMsg(subject, reply string, headers nats.Header, body []byte) *nats.Msg {
	return &nats.Msg{Subject: subject, Reply: reply, Header: headers, Data: body}
}
