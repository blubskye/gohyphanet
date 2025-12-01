// GoHyphanet - NPF Message Types
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

package npf

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Message priorities (matching Fred's priorities)
const (
	PriorityNow          = 0 // Very urgent
	PriorityHigh         = 1 // Short timeout, urgent
	PriorityUnspecified  = 2 // Normal priority
	PriorityLow          = 3 // Request initiation
	PriorityRealtimeData = 4 // Realtime bulk data
	PriorityBulkData     = 5 // Bulk data transfer
	NumPriorities        = 6
)

// MessageType represents a type of message
type MessageType uint16

// Basic message types
const (
	// Connection management
	MsgTypeVoid         MessageType = 0  // Keepalive/void message
	MsgTypeDisconnect   MessageType = 1  // Disconnect notification
	MsgTypeNodeToNode   MessageType = 2  // Node-to-node custom message

	// Testing/debugging
	MsgTypePing         MessageType = 10 // Ping request
	MsgTypePong         MessageType = 11 // Ping response

	// Data transfer (simplified for now)
	MsgTypeDataRequest  MessageType = 20 // Request data
	MsgTypeDataReply    MessageType = 21 // Reply with data
	MsgTypeDataNotFound MessageType = 22 // Data not found

	// Routing (placeholders for future implementation)
	MsgTypeRouteRequest MessageType = 30 // Routing request
	MsgTypeRouteReply   MessageType = 31 // Routing reply
)

// MessageSpec defines a message specification
type MessageSpec struct {
	Type     MessageType
	Name     string
	Priority int
	Fields   []FieldSpec
}

// FieldSpec defines a message field
type FieldSpec struct {
	Name     string
	DataType FieldType
}

// FieldType represents the type of a field
type FieldType int

const (
	FieldTypeInt8    FieldType = 0
	FieldTypeInt16   FieldType = 1
	FieldTypeInt32   FieldType = 2
	FieldTypeInt64   FieldType = 3
	FieldTypeString  FieldType = 4
	FieldTypeBytes   FieldType = 5
	FieldTypeFloat64 FieldType = 6
	FieldTypeBool    FieldType = 7
)

// NPFMessage represents a complete message
type NPFMessage struct {
	Type     MessageType
	Priority int
	Fields   map[string]interface{}
}

// NewMessage creates a new message of the given type
func NewMessage(msgType MessageType) *NPFMessage {
	spec, ok := messageSpecs[msgType]
	if !ok {
		return &NPFMessage{
			Type:     msgType,
			Priority: PriorityUnspecified,
			Fields:   make(map[string]interface{}),
		}
	}

	return &NPFMessage{
		Type:     msgType,
		Priority: spec.Priority,
		Fields:   make(map[string]interface{}),
	}
}

// Set sets a field value
func (m *NPFMessage) Set(name string, value interface{}) {
	m.Fields[name] = value
}

// Get gets a field value
func (m *NPFMessage) Get(name string) (interface{}, bool) {
	val, ok := m.Fields[name]
	return val, ok
}

// GetString gets a string field
func (m *NPFMessage) GetString(name string) (string, bool) {
	val, ok := m.Fields[name]
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}

// GetInt64 gets an int64 field
func (m *NPFMessage) GetInt64(name string) (int64, bool) {
	val, ok := m.Fields[name]
	if !ok {
		return 0, false
	}
	i64, ok := val.(int64)
	return i64, ok
}

// GetBytes gets a bytes field
func (m *NPFMessage) GetBytes(name string) ([]byte, bool) {
	val, ok := m.Fields[name]
	if !ok {
		return nil, false
	}
	b, ok := val.([]byte)
	return b, ok
}

// Serialize serializes the message to bytes
func (m *NPFMessage) Serialize() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write message type (2 bytes)
	if err := binary.Write(buf, binary.BigEndian, uint16(m.Type)); err != nil {
		return nil, err
	}

	// Write number of fields (2 bytes)
	if err := binary.Write(buf, binary.BigEndian, uint16(len(m.Fields))); err != nil {
		return nil, err
	}

	// Write each field
	for name, value := range m.Fields {
		// Write field name length (1 byte) and name
		if len(name) > 255 {
			return nil, fmt.Errorf("field name too long: %s", name)
		}
		buf.WriteByte(byte(len(name)))
		buf.WriteString(name)

		// Write field value based on type
		if err := writeField(buf, value); err != nil {
			return nil, fmt.Errorf("failed to write field %s: %w", name, err)
		}
	}

	return buf.Bytes(), nil
}

// Parse parses a message from bytes
func ParseMessage(data []byte) (*NPFMessage, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("message too short")
	}

	buf := bytes.NewReader(data)

	// Read message type
	var msgType uint16
	if err := binary.Read(buf, binary.BigEndian, &msgType); err != nil {
		return nil, err
	}

	msg := NewMessage(MessageType(msgType))

	// Read number of fields
	var numFields uint16
	if err := binary.Read(buf, binary.BigEndian, &numFields); err != nil {
		return nil, err
	}

	// Read each field
	for i := 0; i < int(numFields); i++ {
		// Read field name length
		nameLenByte, err := buf.ReadByte()
		if err != nil {
			return nil, err
		}
		nameLen := int(nameLenByte)

		// Read field name
		nameBytes := make([]byte, nameLen)
		if _, err := buf.Read(nameBytes); err != nil {
			return nil, err
		}
		name := string(nameBytes)

		// Read field value
		value, err := readField(buf)
		if err != nil {
			return nil, fmt.Errorf("failed to read field %s: %w", name, err)
		}

		msg.Fields[name] = value
	}

	return msg, nil
}

// writeField writes a field value to the buffer
func writeField(buf *bytes.Buffer, value interface{}) error {
	switch v := value.(type) {
	case int8:
		buf.WriteByte(byte(FieldTypeInt8))
		return binary.Write(buf, binary.BigEndian, v)
	case int16:
		buf.WriteByte(byte(FieldTypeInt16))
		return binary.Write(buf, binary.BigEndian, v)
	case int32:
		buf.WriteByte(byte(FieldTypeInt32))
		return binary.Write(buf, binary.BigEndian, v)
	case int64:
		buf.WriteByte(byte(FieldTypeInt64))
		return binary.Write(buf, binary.BigEndian, v)
	case int:
		buf.WriteByte(byte(FieldTypeInt64))
		return binary.Write(buf, binary.BigEndian, int64(v))
	case string:
		buf.WriteByte(byte(FieldTypeString))
		if len(v) > 65535 {
			return fmt.Errorf("string too long")
		}
		binary.Write(buf, binary.BigEndian, uint16(len(v)))
		buf.WriteString(v)
		return nil
	case []byte:
		buf.WriteByte(byte(FieldTypeBytes))
		if len(v) > 65535 {
			return fmt.Errorf("bytes too long")
		}
		binary.Write(buf, binary.BigEndian, uint16(len(v)))
		buf.Write(v)
		return nil
	case float64:
		buf.WriteByte(byte(FieldTypeFloat64))
		return binary.Write(buf, binary.BigEndian, v)
	case bool:
		buf.WriteByte(byte(FieldTypeBool))
		if v {
			buf.WriteByte(1)
		} else {
			buf.WriteByte(0)
		}
		return nil
	default:
		return fmt.Errorf("unsupported field type: %T", value)
	}
}

// readField reads a field value from the buffer
func readField(buf *bytes.Reader) (interface{}, error) {
	typeByte, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}

	fieldType := FieldType(typeByte)

	switch fieldType {
	case FieldTypeInt8:
		var v int8
		err := binary.Read(buf, binary.BigEndian, &v)
		return v, err
	case FieldTypeInt16:
		var v int16
		err := binary.Read(buf, binary.BigEndian, &v)
		return v, err
	case FieldTypeInt32:
		var v int32
		err := binary.Read(buf, binary.BigEndian, &v)
		return v, err
	case FieldTypeInt64:
		var v int64
		err := binary.Read(buf, binary.BigEndian, &v)
		return v, err
	case FieldTypeString:
		var length uint16
		if err := binary.Read(buf, binary.BigEndian, &length); err != nil {
			return nil, err
		}
		strBytes := make([]byte, length)
		if _, err := buf.Read(strBytes); err != nil {
			return nil, err
		}
		return string(strBytes), nil
	case FieldTypeBytes:
		var length uint16
		if err := binary.Read(buf, binary.BigEndian, &length); err != nil {
			return nil, err
		}
		data := make([]byte, length)
		if _, err := buf.Read(data); err != nil {
			return nil, err
		}
		return data, nil
	case FieldTypeFloat64:
		var v float64
		err := binary.Read(buf, binary.BigEndian, &v)
		return v, err
	case FieldTypeBool:
		b, err := buf.ReadByte()
		if err != nil {
			return nil, err
		}
		return b != 0, nil
	default:
		return nil, fmt.Errorf("unknown field type: %d", fieldType)
	}
}

// Message specifications
var messageSpecs = map[MessageType]*MessageSpec{
	MsgTypeVoid: {
		Type:     MsgTypeVoid,
		Name:     "Void",
		Priority: PriorityUnspecified,
		Fields:   []FieldSpec{},
	},
	MsgTypeDisconnect: {
		Type:     MsgTypeDisconnect,
		Name:     "Disconnect",
		Priority: PriorityHigh,
		Fields: []FieldSpec{
			{Name: "reason", DataType: FieldTypeString},
		},
	},
	MsgTypePing: {
		Type:     MsgTypePing,
		Name:     "Ping",
		Priority: PriorityUnspecified,
		Fields: []FieldSpec{
			{Name: "seqno", DataType: FieldTypeInt64},
			{Name: "timestamp", DataType: FieldTypeInt64},
		},
	},
	MsgTypePong: {
		Type:     MsgTypePong,
		Name:     "Pong",
		Priority: PriorityUnspecified,
		Fields: []FieldSpec{
			{Name: "seqno", DataType: FieldTypeInt64},
			{Name: "timestamp", DataType: FieldTypeInt64},
		},
	},
}

// Helper functions to create common messages

// CreateVoidMessage creates a keepalive/void message
func CreateVoidMessage() *NPFMessage {
	return NewMessage(MsgTypeVoid)
}

// CreateDisconnectMessage creates a disconnect message
func CreateDisconnectMessage(reason string) *NPFMessage {
	msg := NewMessage(MsgTypeDisconnect)
	msg.Set("reason", reason)
	return msg
}

// CreatePingMessage creates a ping message
func CreatePingMessage(seqno, timestamp int64) *NPFMessage {
	msg := NewMessage(MsgTypePing)
	msg.Set("seqno", seqno)
	msg.Set("timestamp", timestamp)
	return msg
}

// CreatePongMessage creates a pong message
func CreatePongMessage(seqno, timestamp int64) *NPFMessage {
	msg := NewMessage(MsgTypePong)
	msg.Set("seqno", seqno)
	msg.Set("timestamp", timestamp)
	return msg
}
