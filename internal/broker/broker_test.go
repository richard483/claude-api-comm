package broker

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/richard483/claude-api-comm/internal/model"
)

func TestPublishReachesSubscriber(t *testing.T) {
	b := New()
	id := uuid.New()
	ch, cancel := b.Subscribe(id)
	defer cancel()

	b.Publish(id, model.Event{Type: "text", Text: "hi"})

	select {
	case ev := <-ch:
		if ev.Text != "hi" {
			t.Errorf("got %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestCloseClosesChannel(t *testing.T) {
	b := New()
	id := uuid.New()
	ch, _ := b.Subscribe(id)
	b.Close(id)
	select {
	case _, open := <-ch:
		if open {
			t.Error("expected channel closed")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for close")
	}
}
