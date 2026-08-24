package server

import (
	"testing"
	"time"
)

func TestComposerCatalogHubPublishesMonotonicRevisions(t *testing.T) {
	hub := newComposerCatalogHandlers(nil, nil)
	events, snapshot := hub.subscribe()
	defer hub.unsubscribe(events)
	if snapshot.Revision != 1 || snapshot.Kind != "snapshot" {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	hub.publish("skills")
	select {
	case event := <-events:
		if event.Revision != 2 || event.Kind != "skills" {
			t.Fatalf("event=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("catalog event was not published")
	}
}
