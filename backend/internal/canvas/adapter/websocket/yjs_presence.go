package websocket

import (
	"math"
	"time"

	"github.com/google/uuid"
)

// Presence is a bounded ephemeral session value, never a business revision.
type Presence struct {
	ActorID   uuid.UUID `json:"actorId"`
	ClientID  uuid.UUID `json:"clientId"`
	Cursor    *Cursor   `json:"cursor"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Cursor uses canvas world coordinates, independent of each viewer's camera.
type Cursor struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type presenceEntry struct {
	Presence
	owner uuid.UUID
}
type presenceRoom struct {
	peers    map[uuid.UUID]presenceEntry
	watchers map[chan struct{}]struct{}
}

func (h *Handler) joinPresence(id, actor, client uuid.UUID) (chan struct{}, func(), bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.presence == nil {
		h.presence = map[uuid.UUID]*presenceRoom{}
	}
	room := h.presence[id]
	if room == nil {
		if len(h.presence) >= 1000 {
			return nil, func() {}, false
		}
		room = &presenceRoom{peers: map[uuid.UUID]presenceEntry{}, watchers: map[chan struct{}]struct{}{}}
		h.presence[id] = room
	}
	if len(room.watchers) >= 128 {
		return nil, func() {}, false
	}
	if _, exists := room.peers[client]; exists {
		return nil, func() {}, false
	}
	owner := uuid.New()
	wake := make(chan struct{}, 1)
	room.watchers[wake] = struct{}{}
	room.peers[client] = presenceEntry{Presence: Presence{ActorID: actor, ClientID: client, UpdatedAt: time.Now()}, owner: owner}
	notifyPresence(room)
	return wake, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(room.watchers, wake)
		if room.peers[client].owner == owner {
			delete(room.peers, client)
		}
		notifyPresence(room)
		if len(room.watchers) == 0 {
			delete(h.presence, id)
		}
	}, true
}
func notifyPresence(room *presenceRoom) {
	for wake := range room.watchers {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}
func (h *Handler) updatePresence(id, actor, client uuid.UUID, cursor *Cursor, touch bool) bool {
	if cursor != nil && (math.IsNaN(cursor.X) || math.IsNaN(cursor.Y) || math.IsInf(cursor.X, 0) || math.IsInf(cursor.Y, 0) || math.Abs(cursor.X) > 1e7 || math.Abs(cursor.Y) > 1e7) {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.presence[id]
	if room == nil {
		return false
	}
	entry, ok := room.peers[client]
	if !ok || entry.ActorID != actor {
		return false
	}
	if !touch {
		entry.Cursor = cursor
	}
	entry.UpdatedAt = time.Now()
	room.peers[client] = entry
	notifyPresence(room)
	return true
}
func (h *Handler) presenceSnapshot(id uuid.UUID) []Presence {
	h.mu.Lock()
	defer h.mu.Unlock()
	peers := []Presence{}
	if room := h.presence[id]; room != nil {
		for _, entry := range room.peers {
			if time.Since(entry.UpdatedAt) <= 15*time.Second {
				peers = append(peers, entry.Presence)
			}
		}
	}
	return peers
}
