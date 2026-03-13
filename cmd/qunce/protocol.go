package main

import (
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"
)

type endpointRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type envelope struct {
	V         int         `json:"v"`
	Type      string      `json:"type"`
	EventID   string      `json:"event_id"`
	RequestID string      `json:"request_id,omitempty"`
	TS        string      `json:"ts"`
	Source    endpointRef `json:"source"`
	Target    endpointRef `json:"target"`
	Data      any         `json:"data"`
}

func buildEnvelope(
	messageType string,
	sourceKind string,
	sourceID string,
	targetKind string,
	targetID string,
	data any,
	requestID string,
) envelope {
	return envelope{
		V:         1,
		Type:      messageType,
		EventID:   newID("evt"),
		RequestID: requestID,
		TS:        time.Now().UTC().Format(time.RFC3339),
		Source:    endpointRef{Kind: sourceKind, ID: sourceID},
		Target:    endpointRef{Kind: targetKind, ID: targetID},
		Data:      data,
	}
}

func sendEnvelope(conn *websocket.Conn, payload envelope) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, raw)
}

type rawEnvelope struct {
	V         int             `json:"v"`
	Type      string          `json:"type"`
	EventID   string          `json:"event_id"`
	RequestID string          `json:"request_id"`
	TS        string          `json:"ts"`
	Source    endpointRef     `json:"source"`
	Target    endpointRef     `json:"target"`
	Data      json.RawMessage `json:"data"`
}

func readEnvelope(conn *websocket.Conn) (rawEnvelope, error) {
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return rawEnvelope{}, err
	}

	var payload rawEnvelope
	if err := json.Unmarshal(raw, &payload); err != nil {
		return rawEnvelope{}, err
	}
	return payload, nil
}

func dataMap(raw json.RawMessage) (map[string]interface{}, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]interface{}{}, nil
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func asStringSlice(raw interface{}) ([]string, error) {
	values, ok := raw.([]interface{})
	if !ok {
		return nil, nil
	}

	results := make([]string, 0, len(values))
	for _, value := range values {
		item, ok := value.(string)
		if !ok {
			continue
		}
		if normalized := item; normalized != "" {
			results = append(results, normalized)
		}
	}
	return results, nil
}
