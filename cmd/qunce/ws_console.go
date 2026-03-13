package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

var wsConsoleUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool {
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func registerConsoleSocket(mux *http.ServeMux, consoles *consoleRegistry) {
	mux.HandleFunc("/ws/console", handleConsoleSocket(consoles))
}

func handleConsoleSocket(consoles *consoleRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsConsoleUpgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("upgrade console socket: %v", err)
			return
		}

		var subscriptionID int64
		defer func() {
			if subscriptionID > 0 {
				consoles.remove(subscriptionID)
			}
			_ = conn.Close()
		}()

		for {
			raw, err := readEnvelope(conn)
			if err != nil {
				return
			}
			payload, err := dataMap(raw.Data)
			if err != nil {
				_ = conn.WriteMessage(websocket.TextMessage, mustJSON(buildEnvelope(
					"server.notice",
					"server",
					"main",
					"console",
					"browser",
					map[string]interface{}{"level": "warning", "message": "invalid subscribe payload"},
					raw.RequestID,
				)))
				continue
			}

			if strings.TrimSpace(raw.Type) != "console.subscribe" {
				_ = conn.WriteMessage(websocket.TextMessage, mustJSON(buildEnvelope(
					"server.notice",
					"server",
					"main",
					"console",
					"browser",
					map[string]interface{}{"level": "warning", "message": "expected console.subscribe"},
					raw.RequestID,
				)))
				continue
			}

			var chatIDs []string
			if rawChatIDs, ok := payload["chat_ids"].([]interface{}); ok {
				for _, rawChatID := range rawChatIDs {
					chatID, ok := rawChatID.(string)
					if !ok {
						continue
					}
					chatID = strings.TrimSpace(chatID)
					if chatID == "" {
						continue
					}
					chatIDs = append(chatIDs, chatID)
				}
			}
			watchNode := toBool(payload["watch_nodes"], false)

			if subscriptionID == 0 {
				subscriptionID = consoles.add(conn, chatIDs, watchNode)
			} else {
				consoles.update(subscriptionID, chatIDs, watchNode)
			}

			consoles.sendChatSnapshot(subscriptionID, raw.RequestID, nil)
			if watchNode {
				consoles.sendNodeUpdate(subscriptionID, raw.RequestID, nil)
			}
		}
	}
}
