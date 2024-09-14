package natshttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/nats-io/nats.go"
)

// Handler is a struct that represents the NATS connection.
type Handler struct {
	nc *nats.Conn
}

func New(nc *nats.Conn) *Handler {
	return &Handler{nc}
}

// NatsHandler routes HTTP requests to appropriate NATS operations.
func (h *Handler) NatsHandler(w http.ResponseWriter, r *http.Request) {
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

// handleNatsReq makes a request to NATS.
func (h *Handler) handleNatsReq(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	query := r.URL.Query()
	reply := query.Get("reply")
	timeout := getTimeout(query)
	subj := getNatsSubject(w, r)
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

// handleNatsPublish publishes a message with an optional reply.
func (h *Handler) handleNatsPublish(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	query := r.URL.Query()
	reply := query.Get("reply")
	hdrs := getNatsHeaders(r.Header)
	subj := getNatsSubject(w, r)

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

// handleNatsSubscribe handles subscribing to a NATS subject and streams events over HTTP.
func (h *Handler) handleNatsSubscribe(w http.ResponseWriter, r *http.Request) {
	subject := getNatsSubject(w, r)
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
			enc := json.NewEncoder(&buf)
			if err := enc.Encode(ev); err != nil {
				fmt.Fprintf(w, "data: error encoding message\n\n")
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				continue
			}
			fmt.Fprintf(w, "data: %v\n\n", buf.String())

			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}

		case <-clientGone:
			fmt.Println("Client disconnected")
			return

		case <-timeout:
			fmt.Fprintf(w, ": nothing to send, connection closing\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			return
		}
	}
}

// getNatsSubject extracts the subject from the URL.
func getNatsSubject(w http.ResponseWriter, r *http.Request) string {
	subject := path.Base(r.URL.Path)
	if len(subject) == 0 {
		WriteJSONError(w, http.StatusBadRequest, "Subject not found")
		return ""
	}
	return subject
}

// getTimeout extracts the timeout query parameter and converts it to time.Duration.
func getTimeout(query url.Values) time.Duration {
	timeoutStr := query.Get("timeout")
	if timeoutStr == "" {
		return 5000 * time.Millisecond
	}

	timeout, err := strconv.ParseUint(timeoutStr, 10, 64)
	if err != nil {
		return 5000 * time.Millisecond
	}

	return time.Duration(timeout) * time.Millisecond
}

// getNatsHeaders converts HTTP headers to NATS headers if they start with "NatsH".
func getNatsHeaders(httpHeaders http.Header) nats.Header {
	natsHeaders := nats.Header{}
	for key, values := range httpHeaders {
		if strings.HasPrefix(key, "Natsh-") {
			natsKey := firstLetterToLower(strings.TrimPrefix(key, "Natsh-"))
			natsHeaders.Add(natsKey, values[0])
		}
	}
	return natsHeaders
}

// Error a struct to return on error
type Error struct {
	Message string `json:"message"`
}

// WriteJSONError writes the given error as JSON to the given writer
func WriteJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	response, err := json.Marshal(Error{Message: message})

	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(`{"code": 500, "message": "Could not write response"}`))
		return
	}

	w.WriteHeader(statusCode)
	w.Write(response)
}

func WriteJSONResponse(w http.ResponseWriter, r *http.Request, result interface{}) {
	body, err := json.Marshal(result)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		// log.Error().Err(err).Msg("JSON marshal failed")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(prettyJSON(body))
}

func prettyJSON(b []byte) []byte {
	var out bytes.Buffer
	json.Indent(&out, b, "", "  ")
	return out.Bytes()
}

func firstLetterToLower(s string) string {

	if len(s) == 0 {
		return s
	}

	r := []rune(s)
	r[0] = unicode.ToLower(r[0])

	return string(r)
}

// NewNatsMsg creates a new NATS message with subject, reply, headers, and body.
func NewNatsMsg(subject, reply string, headers nats.Header, body []byte) *nats.Msg {
	return &nats.Msg{
		Subject: subject,
		Reply:   reply,
		Header:  headers,
		Data:    body,
	}
}
