package main

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type agentConnection struct {
	nodeID  string
	conn    *websocket.Conn
	sendMux *sync.Mutex
}

type pendingValidation struct {
	done chan map[string]interface{}
}

type agentRegistry struct {
	mu      sync.Mutex
	nodes   map[string]*agentConnection
	pending map[string]*pendingValidation
}

func newAgentRegistry() *agentRegistry {
	return &agentRegistry{
		nodes:   map[string]*agentConnection{},
		pending: map[string]*pendingValidation{},
	}
}

func (r *agentRegistry) upsert(nodeID string, conn *websocket.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nodes[nodeID] = &agentConnection{
		nodeID:  nodeID,
		conn:    conn,
		sendMux: &sync.Mutex{},
	}
}

func (r *agentRegistry) remove(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.nodes, nodeID)
}

func (r *agentRegistry) get(nodeID string) *agentConnection {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.nodes[nodeID]
}

func (r *agentRegistry) disconnect(nodeID string) {
	conn := r.get(nodeID)
	if conn == nil {
		return
	}
	_ = conn.conn.WriteMessage(
		websocket.TextMessage,
		mustJSON(buildEnvelope(
			"server.notice",
			"server",
			"main",
			"agent",
			nodeID,
			map[string]interface{}{"level": "warning", "message": "node removed by server"},
			"",
		)),
	)
	_ = conn.conn.Close()
	r.remove(nodeID)
}

func (r *agentRegistry) requestWorkspaceValidation(nodeID, workspaceDir string) (map[string]interface{}, error) {
	conn := r.get(nodeID)
	if conn == nil {
		return nil, errors.New("agent not connected")
	}
	requestID := newID("req")
	waiter := &pendingValidation{done: make(chan map[string]interface{}, 1)}
	r.mu.Lock()
	r.pending[requestID] = waiter
	r.mu.Unlock()

	payload := buildEnvelope(
		"server.workspace.validate",
		"server",
		"main",
		"agent",
		nodeID,
		map[string]interface{}{"workspace_dir": workspaceDir},
		requestID,
	)
	if err := sendEnvelopeSafe(conn, payload); err != nil {
		r.completePending(requestID, nil)
		r.disconnect(nodeID)
		return nil, err
	}

	select {
	case result := <-waiter.done:
		if result == nil {
			return nil, errors.New("agent workspace validation timed out")
		}
		return result, nil
	case <-time.After(8 * time.Second):
		r.completePending(requestID, nil)
		return nil, errors.New("agent workspace validation timed out")
	}
}

func (r *agentRegistry) resolveWorkspaceValidation(requestID string, result map[string]interface{}) bool {
	r.mu.Lock()
	waiter, ok := r.pending[requestID]
	if ok {
		delete(r.pending, requestID)
	}
	r.mu.Unlock()
	if !ok || waiter == nil {
		return false
	}
	waiter.done <- result
	return true
}

func (r *agentRegistry) completePending(requestID string, result map[string]interface{}) {
	r.mu.Lock()
	waiter, ok := r.pending[requestID]
	if ok {
		delete(r.pending, requestID)
	}
	r.mu.Unlock()
	if ok && waiter != nil {
		waiter.done <- result
	}
}

func sendEnvelopeSafe(conn *agentConnection, payload envelope) error {
	conn.sendMux.Lock()
	defer conn.sendMux.Unlock()
	raw := mustJSON(payload)
	return conn.conn.WriteMessage(websocket.TextMessage, raw)
}

type consoleSubscription struct {
	id        int64
	conn      *websocket.Conn
	chatIDs   map[string]struct{}
	watchNode bool
	sendMux   *sync.Mutex
}

type consoleRegistry struct {
	mu            sync.Mutex
	nextID        int64
	subscriptions map[int64]*consoleSubscription
}

func newConsoleRegistry() *consoleRegistry {
	return &consoleRegistry{
		subscriptions: map[int64]*consoleSubscription{},
	}
}

func (r *consoleRegistry) add(conn *websocket.Conn, chatIDs []string, watchNode bool) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	id := r.nextID
	sub := &consoleSubscription{
		id:        id,
		conn:      conn,
		chatIDs:   asSet(chatIDs),
		watchNode: watchNode,
		sendMux:   &sync.Mutex{},
	}
	r.subscriptions[id] = sub
	return id
}

func (r *consoleRegistry) update(id int64, chatIDs []string, watchNode bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sub := r.subscriptions[id]
	if sub == nil {
		return
	}
	sub.chatIDs = asSet(chatIDs)
	sub.watchNode = watchNode
}

func (r *consoleRegistry) remove(id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.subscriptions, id)
}

func (r *consoleRegistry) get(id int64) *consoleSubscription {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.subscriptions[id]
}

func (r *consoleRegistry) snapshotAll() []*consoleSubscription {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]*consoleSubscription, 0, len(r.subscriptions))
	for _, sub := range r.subscriptions {
		items = append(items, sub)
	}
	return items
}

func asSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		set[value] = struct{}{}
	}
	return set
}

func (r *consoleRegistry) sendChatSnapshot(id int64, requestID string, appState *state) {
	if appState == nil {
		return
	}
	sub := r.get(id)
	if sub == nil {
		return
	}
	for chatID := range sub.chatIDs {
		snapshot := appState.chatSnapshot(chatID)
		if snapshot == nil {
			continue
		}
		sendPayload(sub.sendMux, sub.conn, buildEnvelope(
			"server.chat.snapshot",
			"server",
			"main",
			"console",
			"browser",
			chatSnapshotPayload(snapshot),
			requestID,
		))
	}
}

func (r *consoleRegistry) sendNodeUpdate(id int64, requestID string, appState *state) {
	if appState == nil {
		return
	}
	sub := r.get(id)
	if sub == nil {
		return
	}
	sendPayload(sub.sendMux, sub.conn, buildEnvelope(
		"server.node.updated",
		"server",
		"main",
		"console",
		"browser",
		nodeListPayload(appState.listNodes(), appState.cfg, appState.listPersonas()),
		requestID,
	))
}

func sendPayload(mu *sync.Mutex, conn *websocket.Conn, payload envelope) {
	mu.Lock()
	defer mu.Unlock()
	_ = conn.WriteMessage(websocket.TextMessage, mustJSON(payload))
}

func sendPayloadWithError(mu *sync.Mutex, conn *websocket.Conn, payload envelope) error {
	mu.Lock()
	defer mu.Unlock()
	return conn.WriteMessage(websocket.TextMessage, mustJSON(payload))
}

func mustJSON(v interface{}) []byte {
	raw, _ := json.Marshal(v)
	return raw
}

func broadcastChatSnapshot(reg *consoleRegistry, appState *state, chatID string) {
	if reg == nil || appState == nil {
		return
	}
	for _, sub := range reg.snapshotAll() {
		if _, ok := sub.chatIDs[chatID]; !ok {
			continue
		}
		reg.sendChatSnapshot(sub.id, "", appState)
	}
}

func broadcastNodeUpdate(reg *consoleRegistry, appState *state) {
	if reg == nil || appState == nil {
		return
	}
	for _, sub := range reg.snapshotAll() {
		if !sub.watchNode {
			continue
		}
		reg.sendNodeUpdate(sub.id, "", appState)
	}
}
