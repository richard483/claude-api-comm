package broker

import (
	"sync"

	"github.com/google/uuid"
	"github.com/richard483/claude-api-comm/internal/model"
)

type Broker struct {
	mu   sync.Mutex
	subs map[uuid.UUID][]chan model.Event
}

func New() *Broker {
	return &Broker{subs: make(map[uuid.UUID][]chan model.Event)}
}

func (b *Broker) Subscribe(turnID uuid.UUID) (<-chan model.Event, func()) {
	ch := make(chan model.Event, 64)
	b.mu.Lock()
	b.subs[turnID] = append(b.subs[turnID], ch)
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		list := b.subs[turnID]
		for i, c := range list {
			if c == ch {
				b.subs[turnID] = append(list[:i], list[i+1:]...)
				close(ch)
				break
			}
		}
	}
	return ch, cancel
}

func (b *Broker) Publish(turnID uuid.UUID, ev model.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs[turnID] {
		select {
		case ch <- ev:
		default: // drop for slow subscriber
		}
	}
}

func (b *Broker) Close(turnID uuid.UUID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs[turnID] {
		close(ch)
	}
	delete(b.subs, turnID)
}
