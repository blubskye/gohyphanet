// GoHyphanet - NPF Message Dispatcher
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

package npf

import (
	"fmt"
	"log"
	"net"
	"sync"
)

// MessageHandler is a function that handles a received message
type MessageHandler func(msg *NPFMessage, from *net.UDPAddr) error

// Dispatcher routes incoming messages to handlers
type Dispatcher struct {
	handlers  map[MessageType][]MessageHandler
	mu        sync.RWMutex
	debugMode bool
}

// NewDispatcher creates a new message dispatcher
func NewDispatcher(debugMode bool) *Dispatcher {
	return &Dispatcher{
		handlers:  make(map[MessageType][]MessageHandler),
		debugMode: debugMode,
	}
}

// Register registers a handler for a message type
func (d *Dispatcher) Register(msgType MessageType, handler MessageHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.handlers[msgType] == nil {
		d.handlers[msgType] = make([]MessageHandler, 0)
	}

	d.handlers[msgType] = append(d.handlers[msgType], handler)

	if d.debugMode {
		log.Printf("[DISPATCHER] Registered handler for message type %d", msgType)
	}
}

// Dispatch dispatches a message to all registered handlers
func (d *Dispatcher) Dispatch(msg *NPFMessage, from *net.UDPAddr) error {
	d.mu.RLock()
	handlers := d.handlers[msg.Type]
	d.mu.RUnlock()

	if len(handlers) == 0 {
		if d.debugMode {
			log.Printf("[DISPATCHER] No handlers for message type %d from %s", msg.Type, from)
		}
		return fmt.Errorf("no handlers for message type %d", msg.Type)
	}

	if d.debugMode {
		log.Printf("[DISPATCHER] Dispatching message type %d from %s to %d handlers",
			msg.Type, from, len(handlers))
	}

	// Call all handlers
	var lastErr error
	for _, handler := range handlers {
		if err := handler(msg, from); err != nil {
			lastErr = err
			if d.debugMode {
				log.Printf("[DISPATCHER] Handler error for type %d: %v", msg.Type, err)
			}
		}
	}

	return lastErr
}

// HasHandler returns true if there is a handler for the given message type
func (d *Dispatcher) HasHandler(msgType MessageType) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	handlers, ok := d.handlers[msgType]
	return ok && len(handlers) > 0
}

// GetStats returns dispatcher statistics
func (d *Dispatcher) GetStats() map[string]interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()

	totalHandlers := 0
	for _, handlers := range d.handlers {
		totalHandlers += len(handlers)
	}

	return map[string]interface{}{
		"message_types":   len(d.handlers),
		"total_handlers":  totalHandlers,
	}
}
