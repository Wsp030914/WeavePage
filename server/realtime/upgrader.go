package realtime

// 文件说明：这个文件负责某部分实时协同基础设施。
// 实现方式：把连接管理、协议或多节点广播职责拆分实现。
// 这样做的好处是实时链路更容易演进和排障。
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
