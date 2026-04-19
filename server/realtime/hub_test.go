package realtime

import (
	"ToDoList/server/service"
	"testing"
)

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
