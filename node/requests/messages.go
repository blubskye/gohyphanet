package requests

import "github.com/blubskye/gohyphanet/node/keys"

// Message types for request/response protocol

// MessageAccepted indicates a request has been accepted
type MessageAccepted struct {
	UID uint64
}

// MessageDataNotFound indicates data was not found
type MessageDataNotFound struct {
	UID uint64
}

// MessageRouteNotFound indicates no route to the data
type MessageRouteNotFound struct {
	UID uint64
	HTL int16
}

// MessageRejectedOverload indicates rejection due to overload
type MessageRejectedOverload struct {
	UID    uint64
	Reason string
}

// MessageRejectedLoop indicates a routing loop was detected
type MessageRejectedLoop struct {
	UID uint64
}

// MessageRecentlyFailed indicates the key recently failed on multiple nodes
type MessageRecentlyFailed struct {
	UID        uint64
	TimeToWait int64 // Milliseconds to wait before retry
}

// CHK-specific messages

// MessageCHKDataRequest requests CHK data
type MessageCHKDataRequest struct {
	UID          uint64
	HTL          int16
	Key          *keys.NodeCHK
	RealTimeFlag bool
}

// MessageCHKDataFound indicates CHK data was found (headers only)
type MessageCHKDataFound struct {
	UID     uint64
	Headers []byte
}

// MessageCHKData contains the actual CHK data payload
type MessageCHKData struct {
	UID  uint64
	Data []byte
}

// SSK-specific messages

// MessageSSKDataRequest requests SSK data
type MessageSSKDataRequest struct {
	UID          uint64
	HTL          int16
	Key          *keys.NodeSSK
	NeedsPubKey  bool
	RealTimeFlag bool
}

// MessageSSKDataFoundHeaders contains SSK headers
type MessageSSKDataFoundHeaders struct {
	UID     uint64
	Headers []byte
}

// MessageSSKDataFoundData contains SSK data payload
type MessageSSKDataFoundData struct {
	UID  uint64
	Data []byte
}

// MessageSSKPubKey contains the SSK public key
type MessageSSKPubKey struct {
	UID    uint64
	PubKey []byte
}

// Insert messages

// MessageInsertRequest represents an insert request
type MessageInsertRequest struct {
	UID          uint64
	HTL          int16
	Key          keys.Key
	RealTimeFlag bool
}

// MessageDataInsert contains data being inserted
type MessageDataInsert struct {
	UID     uint64
	Headers []byte
	Data    []byte
}

// MessageInsertReply indicates successful insert
type MessageInsertReply struct {
	UID uint64
}

// MessageDataInsertRejected indicates insert was rejected
type MessageDataInsertRejected struct {
	UID    uint64
	Reason int
}

// Block transfer messages

// MessagePacketTransmit transmits a single packet
type MessagePacketTransmit struct {
	UID      uint64
	PacketNo int
	Data     []byte
	Sent     []bool // Bit array of which packets have been sent
}

// MessageAllSent indicates all packets have been sent
type MessageAllSent struct {
	UID uint64
}

// MessageAllReceived acknowledges all packets received
type MessageAllReceived struct {
	UID uint64
}

// MessageSendAborted indicates transfer was aborted
type MessageSendAborted struct {
	UID         uint64
	Reason      int
	Description string
}
