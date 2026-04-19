package realtime

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
)

var defaultUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// UpgradeWebSocket upgrades an HTTP request to a WebSocket connection.
func UpgradeWebSocket(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	conn, err := defaultUpgrader.Upgrade(w, r, nil)
	if err != nil {
		if errors.Is(err, http.ErrAbortHandler) {
			return nil, err
		}
		return nil, fmt.Errorf("upgrade websocket: %w", err)
	}
	return conn, nil
}

// UpgradeContent upgrades a task content collaboration request.
func UpgradeContent(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	return UpgradeWebSocket(w, r)
}
