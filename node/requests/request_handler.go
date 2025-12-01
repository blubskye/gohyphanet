package requests

import (
	"fmt"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/node/keys"
	"github.com/blubskye/gohyphanet/node/routing"
	"github.com/blubskye/gohyphanet/node/store"
)

// RequestHandler processes incoming data requests from peers
type RequestHandler struct {
	mu sync.RWMutex

	// Request identification
	uid    uint64
	key    keys.Key
	htl    int16
	source PeerInterface

	// Request state
	status           RequestStatus
	searchStartTime  time.Time
	responseDeadline time.Time
	realTimeFlag     bool

	// Data transfer
	sender              *routing.RequestSender
	finalBlock          store.KeyBlock
	transferringFrom    PeerInterface
	finalTransferFailed bool

	// Byte counting
	sentBytes     int64
	receivedBytes int64

	// Dependencies
	datastore      store.FreenetStore
	peerManager    PeerManagerInterface
	htlManager     *routing.HTLManager
	requestTracker RequestTrackerInterface

	// Completion
	completed chan struct{}
	callbacks []RequestCompletionCallback
}

// RequestCompletionCallback is called when a request completes
type RequestCompletionCallback interface {
	OnRequestComplete(handler *RequestHandler, status RequestStatus, block store.KeyBlock)
}

// PeerInterface defines minimal peer operations needed
type PeerInterface interface {
	GetLocation() float64
	IsRoutable() bool
	IsDisconnecting() bool
	SendMessage(msg interface{}) error
	GetClosestPeerLocation(target float64, exclude map[float64]bool) float64
	ShallWeRouteAccordingToOurPeersLocation(htl int16) bool
	IsInMandatoryBackoff(now time.Time, realTime bool) bool
	IsRoutingBackedOff(realTime bool) bool
}

// PeerManagerInterface defines peer management operations
type PeerManagerInterface interface {
	GetConnectedPeers() []PeerInterface
	SelectCloserPeer(source PeerInterface, routedTo map[PeerInterface]bool,
		target float64, key keys.Key, htl int16, realTime bool) PeerInterface
}

// RequestTrackerInterface tracks active requests
type RequestTrackerInterface interface {
	RegisterRequest(handler *RequestHandler) error
	UnregisterRequest(uid uint64)
	GetRequest(uid uint64) *RequestHandler
}

// NewRequestHandler creates a new incoming request handler
func NewRequestHandler(
	uid uint64,
	key keys.Key,
	htl int16,
	source PeerInterface,
	realTimeFlag bool,
	datastore store.FreenetStore,
	peerManager PeerManagerInterface,
	htlManager *routing.HTLManager,
	tracker RequestTrackerInterface,
	timeout time.Duration,
) *RequestHandler {

	handler := &RequestHandler{
		uid:              uid,
		key:              key,
		htl:              htl,
		source:           source,
		realTimeFlag:     realTimeFlag,
		datastore:        datastore,
		peerManager:      peerManager,
		htlManager:       htlManager,
		requestTracker:   tracker,
		status:           StatusNotFinished,
		searchStartTime:  time.Now(),
		responseDeadline: time.Now().Add(timeout),
		completed:        make(chan struct{}),
	}

	return handler
}

// Run executes the request handling logic
func (h *RequestHandler) Run() error {
	defer func() {
		h.requestTracker.UnregisterRequest(h.uid)
		h.notifyCompletion()
	}()

	// Register this handler
	if err := h.requestTracker.RegisterRequest(h); err != nil {
		return err
	}

	// Phase 1: Check local datastore FIRST
	block := h.checkDatastore()
	if block != nil {
		// Found locally! Send acceptance and data
		h.sendAccepted()
		return h.returnLocalData(block)
	}

	// Phase 2: Not found locally - forward the request
	if h.htl <= 0 {
		// No hops left
		h.setStatus(StatusDataNotFound)
		h.sendDataNotFound()
		return nil
	}

	// Send acceptance
	h.sendAccepted()

	// Create request sender to forward
	h.sender = routing.NewRequestSender(
		h.uid,
		h.key,
		h.htl-1, // Decrement HTL
		h.source,
		time.Until(h.responseDeadline),
	)

	// Phase 3: Route the request
	return h.forwardRequest()
}

// checkDatastore checks if we have the data locally
func (h *RequestHandler) checkDatastore() store.KeyBlock {
	meta := store.NewBlockMetadata()

	// Fetch from store
	block, err := h.datastore.Fetch(
		h.key.GetRoutingKey(),
		h.key.GetFullKey(),
		false, // dontPromote
		true,  // canReadClientCache
		true,  // canReadSlashdotCache
		false, // ignoreOldBlocks
		meta,
	)

	if err != nil || block == nil {
		return nil
	}

	// Verify it's the right type
	keyBlock, ok := block.(store.KeyBlock)
	if !ok {
		return nil
	}

	return keyBlock
}

// returnLocalData sends data we found locally
func (h *RequestHandler) returnLocalData(block store.KeyBlock) error {
	h.mu.Lock()
	h.finalBlock = block
	h.mu.Unlock()

	// Send data based on type
	switch block.(type) {
	case *store.CHKBlock:
		return h.sendCHKData(block.(*store.CHKBlock))
	case *store.SSKBlock:
		return h.sendSSKData(block.(*store.SSKBlock))
	default:
		return fmt.Errorf("unknown block type")
	}
}

// forwardRequest forwards the request to the next peer
func (h *RequestHandler) forwardRequest() error {
	// Select next peer using routing algorithm
	routedTo := make(map[PeerInterface]bool)
	for peer := range h.sender.GetRoutedTo() {
		routedTo[peer.(PeerInterface)] = true
	}
	target := h.sender.GetTargetLocation()

	nextPeer := h.peerManager.SelectCloserPeer(
		h.source,
		routedTo,
		target,
		h.key,
		h.sender.GetHTL(),
		h.realTimeFlag,
	)

	if nextPeer == nil {
		// No route found
		h.setStatus(StatusRouteNotFound)
		h.sendRouteNotFound()
		return nil
	}

	// Add to routing history
	h.sender.AddRoutedTo(nextPeer)

	// Forward the request
	msg := h.createForwardMessage(nextPeer)
	if err := nextPeer.SendMessage(msg); err != nil {
		// Send failed, try to route to another peer
		return h.forwardRequest()
	}

	// Wait for response
	return h.waitForResponse()
}

// waitForResponse waits for the downstream response
func (h *RequestHandler) waitForResponse() error {
	// Set timeout
	timeout := time.NewTimer(time.Until(h.responseDeadline))
	defer timeout.Stop()

	select {
	case <-timeout.C:
		// Timed out
		h.setStatus(StatusTimedOut)
		h.sendRejectedOverload()
		return nil

	case <-h.completed:
		// Request completed (handled by message callbacks)
		return nil
	}
}

// Message sending helpers

func (h *RequestHandler) sendAccepted() error {
	msg := &MessageAccepted{
		UID: h.uid,
	}
	return h.source.SendMessage(msg)
}

func (h *RequestHandler) sendDataNotFound() error {
	msg := &MessageDataNotFound{
		UID: h.uid,
	}
	h.recordSentBytes(32) // Approx message size
	return h.source.SendMessage(msg)
}

func (h *RequestHandler) sendRouteNotFound() error {
	msg := &MessageRouteNotFound{
		UID: h.uid,
		HTL: h.sender.GetHTL(),
	}
	h.recordSentBytes(32)
	return h.source.SendMessage(msg)
}

func (h *RequestHandler) sendRejectedOverload() error {
	msg := &MessageRejectedOverload{
		UID: h.uid,
	}
	h.recordSentBytes(32)
	return h.source.SendMessage(msg)
}

func (h *RequestHandler) sendCHKData(block *store.CHKBlock) error {
	// Send headers
	msg := &MessageCHKDataFound{
		UID:     h.uid,
		Headers: block.GetRawHeaders(),
	}
	if err := h.source.SendMessage(msg); err != nil {
		return err
	}
	h.recordSentBytes(int64(len(block.GetRawHeaders())))

	// Start block transmitter for data
	// (Simplified - full implementation would use BlockTransmitter)
	data := block.GetRawData()
	dataMsg := &MessageCHKData{
		UID:  h.uid,
		Data: data,
	}
	if err := h.source.SendMessage(dataMsg); err != nil {
		return err
	}
	h.recordSentBytes(int64(len(data)))

	h.setStatus(StatusSuccess)
	return nil
}

func (h *RequestHandler) sendSSKData(block *store.SSKBlock) error {
	// Send headers
	headersMsg := &MessageSSKDataFoundHeaders{
		UID:     h.uid,
		Headers: block.GetRawHeaders(),
	}
	if err := h.source.SendMessage(headersMsg); err != nil {
		return err
	}
	h.recordSentBytes(int64(len(block.GetRawHeaders())))

	// Send data
	dataMsg := &MessageSSKDataFoundData{
		UID:  h.uid,
		Data: block.GetRawData(),
	}
	if err := h.source.SendMessage(dataMsg); err != nil {
		return err
	}
	h.recordSentBytes(int64(len(block.GetRawData())))

	// Send pubkey if available
	if block.GetPubkeyBytes() != nil {
		pubkeyMsg := &MessageSSKPubKey{
			UID:    h.uid,
			PubKey: block.GetPubkeyBytes(),
		}
		if err := h.source.SendMessage(pubkeyMsg); err != nil {
			return err
		}
		h.recordSentBytes(int64(len(block.GetPubkeyBytes())))
	}

	h.setStatus(StatusSuccess)
	return nil
}

func (h *RequestHandler) createForwardMessage(peer PeerInterface) interface{} {
	// Create appropriate request message based on key type
	switch h.key.(type) {
	case *keys.NodeCHK:
		return &MessageCHKDataRequest{
			UID:          h.uid,
			HTL:          h.sender.GetHTL(),
			Key:          h.key.(*keys.NodeCHK),
			RealTimeFlag: h.realTimeFlag,
		}
	case *keys.NodeSSK:
		return &MessageSSKDataRequest{
			UID:          h.uid,
			HTL:          h.sender.GetHTL(),
			Key:          h.key.(*keys.NodeSSK),
			NeedsPubKey:  true,
			RealTimeFlag: h.realTimeFlag,
		}
	default:
		return nil
	}
}

// Status management

func (h *RequestHandler) setStatus(status RequestStatus) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.status = status
	if status != StatusNotFinished {
		close(h.completed)
	}
}

// GetStatus returns the current request status
func (h *RequestHandler) GetStatus() RequestStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.status
}

// GetUID returns the request UID
func (h *RequestHandler) GetUID() uint64 {
	return h.uid
}

// GetKey returns the requested key
func (h *RequestHandler) GetKey() keys.Key {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.key
}

// GetElapsedTime returns how long the request has been running
func (h *RequestHandler) GetElapsedTime() time.Duration {
	return time.Since(h.searchStartTime)
}

// Byte counting

func (h *RequestHandler) recordSentBytes(bytes int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sentBytes += bytes
}

func (h *RequestHandler) recordReceivedBytes(bytes int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.receivedBytes += bytes
}

// GetSentBytes returns total bytes sent
func (h *RequestHandler) GetSentBytes() int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.sentBytes
}

// GetReceivedBytes returns total bytes received
func (h *RequestHandler) GetReceivedBytes() int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.receivedBytes
}

// Callbacks

// AddCompletionCallback adds a callback to be called on completion
func (h *RequestHandler) AddCompletionCallback(callback RequestCompletionCallback) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.callbacks = append(h.callbacks, callback)
}

func (h *RequestHandler) notifyCompletion() {
	h.mu.RLock()
	callbacks := append([]RequestCompletionCallback{}, h.callbacks...)
	status := h.status
	block := h.finalBlock
	h.mu.RUnlock()

	for _, callback := range callbacks {
		callback.OnRequestComplete(h, status, block)
	}
}

// GetStats returns request handler statistics
func (h *RequestHandler) GetStats() RequestHandlerStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return RequestHandlerStats{
		UID:           h.uid,
		Status:        h.status,
		HTL:           h.htl,
		ElapsedTime:   time.Since(h.searchStartTime),
		SentBytes:     h.sentBytes,
		ReceivedBytes: h.receivedBytes,
		RealTime:      h.realTimeFlag,
	}
}

// RequestHandlerStats contains statistics about a request handler
type RequestHandlerStats struct {
	UID           uint64
	Status        RequestStatus
	HTL           int16
	ElapsedTime   time.Duration
	SentBytes     int64
	ReceivedBytes int64
	RealTime      bool
}

// String returns a formatted string of request handler statistics
func (rhs RequestHandlerStats) String() string {
	return fmt.Sprintf("Handler UID=%d, Status=%s, HTL=%d, Time=%v, Sent=%d, Recv=%d",
		rhs.UID, rhs.Status, rhs.HTL, rhs.ElapsedTime, rhs.SentBytes, rhs.ReceivedBytes)
}
