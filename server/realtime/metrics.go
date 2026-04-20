package realtime

import "sync/atomic"

// HubMetrics is a local-node snapshot for one realtime hub.
type HubMetrics struct {
	NodeID            string           `json:"node_id"`
	Hub               string           `json:"hub"`
	ActiveRooms       int              `json:"active_rooms"`
	ActiveConnections int              `json:"active_connections"`
	ActiveUsers       int              `json:"active_users,omitempty"`
	Rooms             []RoomMetrics    `json:"rooms"`
	Counters          RealtimeCounters `json:"counters"`
}

// RoomMetrics summarizes a single local room.
type RoomMetrics struct {
	ID          int `json:"id"`
	Connections int `json:"connections"`
	Users       int `json:"users,omitempty"`
}

// RealtimeCounters contains cumulative local-node counters since process start.
type RealtimeCounters struct {
	ConnectionsAccepted uint64 `json:"connections_accepted"`
	ConnectionsClosed   uint64 `json:"connections_closed"`
	BroadcastMessages   uint64 `json:"broadcast_messages"`
	PubSubPublished     uint64 `json:"pubsub_published"`
	PubSubReceived      uint64 `json:"pubsub_received"`
	PubSubPublishErrors uint64 `json:"pubsub_publish_errors"`
	DroppedClients      uint64 `json:"dropped_clients"`
	ContentUpdates      uint64 `json:"content_updates,omitempty"`
	DuplicateUpdates    uint64 `json:"duplicate_updates,omitempty"`
	LockRequests        uint64 `json:"lock_requests,omitempty"`
	LockErrors          uint64 `json:"lock_errors,omitempty"`
}

type realtimeCounters struct {
	connectionsAccepted atomic.Uint64
	connectionsClosed   atomic.Uint64
	broadcastMessages   atomic.Uint64
	pubSubPublished     atomic.Uint64
	pubSubReceived      atomic.Uint64
	pubSubPublishErrors atomic.Uint64
	droppedClients      atomic.Uint64
	contentUpdates      atomic.Uint64
	duplicateUpdates    atomic.Uint64
	lockRequests        atomic.Uint64
	lockErrors          atomic.Uint64
}

func (c *realtimeCounters) snapshot() RealtimeCounters {
	if c == nil {
		return RealtimeCounters{}
	}
	return RealtimeCounters{
		ConnectionsAccepted: c.connectionsAccepted.Load(),
		ConnectionsClosed:   c.connectionsClosed.Load(),
		BroadcastMessages:   c.broadcastMessages.Load(),
		PubSubPublished:     c.pubSubPublished.Load(),
		PubSubReceived:      c.pubSubReceived.Load(),
		PubSubPublishErrors: c.pubSubPublishErrors.Load(),
		DroppedClients:      c.droppedClients.Load(),
		ContentUpdates:      c.contentUpdates.Load(),
		DuplicateUpdates:    c.duplicateUpdates.Load(),
		LockRequests:        c.lockRequests.Load(),
		LockErrors:          c.lockErrors.Load(),
	}
}
