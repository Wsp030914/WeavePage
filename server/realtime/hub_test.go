package realtime

// 文件说明：这个文件为对应模块提供测试，重点保护关键边界、并发语义和容易回归的行为。
// 实现方式：通过 stub、最小集成场景或显式断言覆盖最脆弱的逻辑分支。
// 这样做的好处是后续重构、补注释或调整实现时，可以快速发现行为回归。

import (
	"ToDoList/server/service"
	"testing"
)

// TestProjectHubPresenceSnapshot_DeduplicatesUsersAndCountsConnections 验证 presence 快照会按用户去重并统计连接数。
func TestProjectHubPresenceSnapshot_DeduplicatesUsersAndCountsConnections(t *testing.T) {
	t.Parallel()

	hub := NewProjectHub(nil, nil, "node-a", nil)
	clientA := &projectClient{
		hub: hub,
		session: service.ProjectRealtimeSession{
			UserID:    7,
			Username:  "alice",
			ProjectID: 9,
		},
		send: make(chan ProjectServerMessage, 1),
	}
	clientB := &projectClient{
		hub: hub,
		session: service.ProjectRealtimeSession{
			UserID:    7,
			Username:  "alice",
			ProjectID: 9,
		},
		send: make(chan ProjectServerMessage, 1),
	}
	clientC := &projectClient{
		hub: hub,
		session: service.ProjectRealtimeSession{
			UserID:    8,
			Username:  "bob",
			ProjectID: 9,
		},
		send: make(chan ProjectServerMessage, 1),
	}

	hub.register(clientA)
	hub.register(clientB)
	hub.register(clientC)

	presence := hub.presenceSnapshot(9)
	if len(presence) != 2 {
		t.Fatalf("expected 2 unique users, got %d", len(presence))
	}
	if presence[0].UserID != 7 || presence[0].Connections != 2 {
		t.Fatalf("unexpected first presence item: %+v", presence[0])
	}
	if presence[1].UserID != 8 || presence[1].Connections != 1 {
		t.Fatalf("unexpected second presence item: %+v", presence[1])
	}

	hub.unregister(clientA)
	presence = hub.presenceSnapshot(9)
	if len(presence) != 2 {
		t.Fatalf("expected 2 unique users after one disconnect, got %d", len(presence))
	}
	if presence[0].Connections != 1 {
		t.Fatalf("expected alice to have 1 remaining connection, got %d", presence[0].Connections)
	}
}

// TestProjectHubMetricsSnapshot_CountsRoomsConnectionsAndCounters verifies local project hub gauges and counters.
func TestProjectHubMetricsSnapshot_CountsRoomsConnectionsAndCounters(t *testing.T) {
	t.Parallel()

	hub := NewProjectHub(nil, nil, "node-a", nil)
	hub.register(&projectClient{
		hub: hub,
		session: service.ProjectRealtimeSession{
			UserID:    7,
			Username:  "alice",
			ProjectID: 9,
		},
		send: make(chan ProjectServerMessage, 1),
	})
	hub.register(&projectClient{
		hub: hub,
		session: service.ProjectRealtimeSession{
			UserID:    8,
			Username:  "bob",
			ProjectID: 9,
		},
		send: make(chan ProjectServerMessage, 1),
	})
	hub.broadcast(9, ProjectServerMessage{Type: ProjectMessageTypePong})

	metrics := hub.MetricsSnapshot()
	if metrics.NodeID != "node-a" || metrics.Hub != "project" {
		t.Fatalf("unexpected hub identity: %+v", metrics)
	}
	if metrics.ActiveRooms != 1 || metrics.ActiveConnections != 2 || metrics.ActiveUsers != 2 {
		t.Fatalf("unexpected active gauges: %+v", metrics)
	}
	if len(metrics.Rooms) != 1 || metrics.Rooms[0].ID != 9 || metrics.Rooms[0].Connections != 2 || metrics.Rooms[0].Users != 2 {
		t.Fatalf("unexpected room metrics: %+v", metrics.Rooms)
	}
	if metrics.Counters.ConnectionsAccepted != 2 || metrics.Counters.BroadcastMessages != 1 {
		t.Fatalf("unexpected counters: %+v", metrics.Counters)
	}
}

// TestContentHubMetricsSnapshot_CountsRoomsConnectionsAndCounters verifies local content hub gauges and counters.
func TestContentHubMetricsSnapshot_CountsRoomsConnectionsAndCounters(t *testing.T) {
	t.Parallel()

	hub := NewContentHub(nil, nil, "node-b")
	hub.register(&contentClient{
		hub: hub,
		session: service.TaskContentSession{
			UserID:    7,
			ProjectID: 1,
			TaskID:    11,
		},
		send: make(chan ContentServerMessage, 1),
	})
	hub.register(&contentClient{
		hub: hub,
		session: service.TaskContentSession{
			UserID:    8,
			ProjectID: 1,
			TaskID:    12,
		},
		send: make(chan ContentServerMessage, 1),
	})
	hub.broadcast(11, ContentServerMessage{Type: ContentMessageTypePong}, nil)

	metrics := hub.MetricsSnapshot()
	if metrics.NodeID != "node-b" || metrics.Hub != "content" {
		t.Fatalf("unexpected hub identity: %+v", metrics)
	}
	if metrics.ActiveRooms != 2 || metrics.ActiveConnections != 2 || metrics.ActiveUsers != 2 {
		t.Fatalf("unexpected active gauges: %+v", metrics)
	}
	if len(metrics.Rooms) != 2 || metrics.Rooms[0].ID != 11 || metrics.Rooms[1].ID != 12 {
		t.Fatalf("unexpected room ordering: %+v", metrics.Rooms)
	}
	if metrics.Counters.ConnectionsAccepted != 2 || metrics.Counters.BroadcastMessages != 1 {
		t.Fatalf("unexpected counters: %+v", metrics.Counters)
	}
}
