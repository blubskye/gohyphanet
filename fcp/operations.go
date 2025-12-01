// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package fcp

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// OperationResult represents the result of an FCP operation
type OperationResult struct {
	Success    bool
	Identifier string
	URI        string
	Data       []byte
	Error      string
	Metadata   map[string]string
}

// Operations provides high-level operations on top of the FCP client
type Operations struct {
	client      *Client
	pending     map[string]chan *OperationResult
	pendingLock sync.RWMutex
}

// NewOperations creates a new Operations instance
func NewOperations(client *Client) *Operations {
	ops := &Operations{
		client:  client,
		pending: make(map[string]chan *OperationResult),
	}

	// Register handlers for operation results
	client.RegisterHandler("AllData", ops.handleAllData)
	client.RegisterHandler("GetFailed", ops.handleGetFailed)
	client.RegisterHandler("PutSuccessful", ops.handlePutSuccessful)
	client.RegisterHandler("PutFailed", ops.handlePutFailed)
	client.RegisterHandler("DataFound", ops.handleDataFound)
	client.RegisterHandler("ProtocolError", ops.handleProtocolError)
	client.RegisterHandler("SimpleProgress", ops.handleSimpleProgress)

	return ops
}

// Get retrieves data from Freenet with a timeout
func (o *Operations) Get(ctx context.Context, uri string) (*OperationResult, error) {
	return o.GetWithProgress(ctx, uri, nil)
}

// Put inserts data into Freenet with a timeout
func (o *Operations) Put(ctx context.Context, uri string, data []byte) (*OperationResult, error) {
	return o.PutWithProgress(ctx, uri, data, nil)
}

// GetWithProgress retrieves data and provides progress updates
func (o *Operations) GetWithProgress(ctx context.Context, uri string, progressFn func(succeeded, total int)) (*OperationResult, error) {
	identifier := generateIdentifier("get")
	resultChan := make(chan *OperationResult, 1)

	o.pendingLock.Lock()
	o.pending[identifier] = resultChan
	o.pendingLock.Unlock()

	defer func() {
		o.pendingLock.Lock()
		delete(o.pending, identifier)
		o.pendingLock.Unlock()
	}()

	// Store progress function if provided
	if progressFn != nil {
		progressIdentifier := "progress-" + identifier
		o.client.progressHandlers[progressIdentifier] = progressFn
		defer delete(o.client.progressHandlers, progressIdentifier)
	}

	msg := &Message{
		Name: "ClientGet",
		Fields: map[string]string{
			"URI":             uri,
					"Identifier":      identifier,
					"ReturnType":      "direct",
					"Verbosity":       "3",
				},	}

	if err := o.client.SendMessage(msg); err != nil {
		return nil, fmt.Errorf("failed to send ClientGet: %w", err)
	}

	select {
	case result := <-resultChan:
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// PutWithProgress inserts data and provides progress updates
func (o *Operations) PutWithProgress(ctx context.Context, uri string, data []byte, progressFn func(succeeded, total int)) (*OperationResult, error) {
	identifier := generateIdentifier("put")
	resultChan := make(chan *OperationResult, 1)

	o.pendingLock.Lock()
	o.pending[identifier] = resultChan
	o.pendingLock.Unlock()

	defer func() {
		o.pendingLock.Lock()
		delete(o.pending, identifier)
		o.pendingLock.Unlock()
	}()

	// Store progress function if provided
	if progressFn != nil {
		progressIdentifier := "progress-" + identifier
		o.client.progressHandlers[progressIdentifier] = progressFn
		defer delete(o.client.progressHandlers, progressIdentifier)
	}

	msg := &Message{
		Name: "ClientPut",
		Fields: map[string]string{
			"URI":        uri,
					"Identifier": identifier,
					"UploadFrom": "direct",
					"Verbosity":  "3",
				},		Data: data,
	}

	if err := o.client.SendMessage(msg); err != nil {
		return nil, fmt.Errorf("failed to send ClientPut: %w", err)
	}

	select {
	case result := <-resultChan:
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Handler functions
func (o *Operations) handleAllData(msg *Message) error {
	identifier := msg.Fields["Identifier"]
	o.sendResult(identifier, &OperationResult{
		Success:    true,
		Identifier: identifier,
		Data:       msg.Data,
		Metadata:   msg.Fields,
	})
	return nil
}

func (o *Operations) handleGetFailed(msg *Message) error {
	identifier := msg.Fields["Identifier"]
	o.sendResult(identifier, &OperationResult{
		Success:    false,
		Identifier: identifier,
		Error:      fmt.Sprintf("Get failed: %s", msg.Fields["CodeDescription"]),
		Metadata:   msg.Fields,
	})
	return nil
}

func (o *Operations) handlePutSuccessful(msg *Message) error {
	identifier := msg.Fields["Identifier"]
	o.sendResult(identifier, &OperationResult{
		Success:    true,
		Identifier: identifier,
		URI:        msg.Fields["URI"],
		Metadata:   msg.Fields,
	})
	return nil
}

func (o *Operations) handlePutFailed(msg *Message) error {
	identifier := msg.Fields["Identifier"]
	o.sendResult(identifier, &OperationResult{
		Success:    false,
		Identifier: identifier,
		Error:      fmt.Sprintf("Put failed: %s", msg.Fields["CodeDescription"]),
		Metadata:   msg.Fields,
	})
	return nil
}

func (o *Operations) handleDataFound(msg *Message) error {
	// DataFound indicates the data exists but hasn't been fully retrieved yet
	// We could use this for progress indication
	return nil
}

func (o *Operations) handleProtocolError(msg *Message) error {
	identifier := msg.Fields["Identifier"]
	if identifier != "" {
		o.sendResult(identifier, &OperationResult{
			Success:    false,
			Identifier: identifier,
			Error:      fmt.Sprintf("Protocol error: %s", msg.Fields["CodeDescription"]),
			Metadata:   msg.Fields,
		})
	}
	return nil
}

func (o *Operations) handleSimpleProgress(msg *Message) error {
    identifier := msg.Fields["Identifier"]
    progressIdentifier := "progress-" + identifier
    
    o.client.progressLock.RLock()
    progressFn, exists := o.client.progressHandlers[progressIdentifier]
    o.client.progressLock.RUnlock()

    if exists && progressFn != nil {
        succeeded := parseIntField(msg.Fields["Succeeded"])
        total := parseIntField(msg.Fields["Total"])
        progressFn(succeeded, total)
    }
    return nil
}

// Helper functions
func (o *Operations) sendResult(identifier string, result *OperationResult) {
	o.pendingLock.RLock()
	ch, exists := o.pending[identifier]
	o.pendingLock.RUnlock()

	if exists {
		select {
		case ch <- result:
		default:
			// Channel is full or closed, ignore
		}
	}
}

func generateIdentifier(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func parseIntField(s string) int {
	var i int
	fmt.Sscanf(s, "%d", &i)
	return i
}