// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package service

import "sync"

type EventType string

const (
	EventForwardCreated EventType = "forward:created"
	EventForwardUpdated EventType = "forward:updated"
	EventForwardDeleted EventType = "forward:deleted"
	EventProxyCreated   EventType = "proxy:created"
	EventProxyUpdated   EventType = "proxy:updated"
	EventProxyDeleted   EventType = "proxy:deleted"
	EventDDNSCreated    EventType = "ddns:created"
	EventDDNSUpdated    EventType = "ddns:updated"
	EventDDNSDeleted    EventType = "ddns:deleted"
	EventSTUNCreated    EventType = "stun:created"
	EventSTUNUpdated    EventType = "stun:updated"
	EventSTUNDeleted    EventType = "stun:deleted"
	EventWOLCreated     EventType = "wol:created"
	EventWOLUpdated     EventType = "wol:updated"
	EventWOLDeleted     EventType = "wol:deleted"
	EventCronCreated    EventType = "cron:created"
	EventCronUpdated    EventType = "cron:updated"
	EventCronDeleted    EventType = "cron:deleted"
	EventACMECreated    EventType = "acme:created"
	EventACMEUpdated    EventType = "acme:updated"
	EventACMEDeleted    EventType = "acme:deleted"
	EventStorageCreated EventType = "storage:created"
	EventStorageUpdated EventType = "storage:updated"
	EventStorageDeleted EventType = "storage:deleted"
)

type Event struct {
	Type EventType
	ID   string
}

type Handler func(event Event)

type Bus struct {
	mu       sync.RWMutex
	handlers map[EventType][]Handler
}

func NewBus() *Bus {
	return &Bus{handlers: make(map[EventType][]Handler)}
}

func (b *Bus) Subscribe(eventType EventType, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

func (b *Bus) Publish(event Event) {
	b.mu.RLock()
	handlers := make([]Handler, len(b.handlers[event.Type]))
	copy(handlers, b.handlers[event.Type])
	b.mu.RUnlock()
	for _, h := range handlers {
		h(event)
	}
}
