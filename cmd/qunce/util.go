package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func newID(prefix string) string {
	raw := make([]byte, 6)
	_, _ = io.ReadFull(rand.Reader, raw)
	encoded := hex.EncodeToString(raw)
	if prefix == "" {
		return encoded
	}
	return prefix + "_" + encoded
}

func trimOrZero(raw string) string {
	return strings.TrimSpace(raw)
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func asMap(raw interface{}) (map[string]interface{}, bool) {
	payload, ok := raw.(map[string]interface{})
	return payload, ok
}

func toString(raw interface{}, fallback string) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case []byte:
		return strings.TrimSpace(string(value))
	case nil:
		return fallback
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	}
}

func toInt(raw interface{}) int {
	switch value := raw.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func toBool(raw interface{}, fallback bool) bool {
	switch value := raw.(type) {
	case bool:
		return value
	default:
		return fallback
	}
}

func parseBody(r *http.Request) map[string]interface{} {
	defer func() {
		_ = r.Body.Close()
	}()

	if r.Body == nil {
		return map[string]interface{}{}
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload == nil {
		return map[string]interface{}{}
	}
	return payload
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	raw, err := json.Marshal(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to serialize response")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(raw); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]interface{}{"message": message})
}
