package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"awaken/src/plugins"
	"awaken/src/skills"
)

const composerCatalogSSEEventName = "composer-catalog"

type composerCatalogEvent struct {
	Revision uint64 `json:"revision"`
	Kind     string `json:"kind"`
}

// composerCatalogHandlers turns the Core-owned Skill Registry and Plugin Store
// mutations into one monotonic invalidation stream. The stream carries no
// catalog payload; consumers re-read the authoritative typed list route for
// their current workspace and marker.
type composerCatalogHandlers struct {
	mu          sync.Mutex
	revision    uint64
	subscribers map[chan composerCatalogEvent]struct{}
}

func newComposerCatalogHandlers(registry *skills.SkillRegistry, store *plugins.Store) *composerCatalogHandlers {
	h := &composerCatalogHandlers{
		revision:    1,
		subscribers: make(map[chan composerCatalogEvent]struct{}),
	}
	if registry != nil {
		registry.SubscribeCatalogChanges(func() { h.publish("skills") })
	}
	if store != nil {
		store.SubscribeChanges(func() { h.publish("plugins") })
	}
	return h
}

func (h *composerCatalogHandlers) publish(kind string) {
	h.mu.Lock()
	h.revision++
	event := composerCatalogEvent{Revision: h.revision, Kind: kind}
	for subscriber := range h.subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
	h.mu.Unlock()
}

func (h *composerCatalogHandlers) subscribe() (chan composerCatalogEvent, composerCatalogEvent) {
	ch := make(chan composerCatalogEvent, 16)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	snapshot := composerCatalogEvent{Revision: h.revision, Kind: "snapshot"}
	h.mu.Unlock()
	return ch, snapshot
}

func (h *composerCatalogHandlers) unsubscribe(ch chan composerCatalogEvent) {
	h.mu.Lock()
	delete(h.subscribers, ch)
	h.mu.Unlock()
}

func (h *composerCatalogHandlers) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	headers := w.Header()
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")
	headers.Set("X-Accel-Buffering", "no")
	flusher.Flush()

	events, snapshot := h.subscribe()
	defer h.unsubscribe(events)
	write := func(event composerCatalogEvent) error {
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", composerCatalogSSEEventName, payload); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	if err := write(snapshot); err != nil {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-events:
			if err := write(event); err != nil {
				return
			}
		}
	}
}
