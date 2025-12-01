// GoHyphanet - Hyphanet Node Implementation
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

package transport

import (
	"fmt"
	"net"
)

// UDPTransport handles UDP packet transmission
type UDPTransport struct {
	conn     *net.UDPConn
	listenAddr *net.UDPAddr
}

// NewUDPTransport creates a new UDP transport
func NewUDPTransport(port int) (*UDPTransport, error) {
	addr := &net.UDPAddr{
		Port: port,
		IP:   net.ParseIP("0.0.0.0"),
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to create UDP socket: %w", err)
	}

	return &UDPTransport{
		conn:       conn,
		listenAddr: addr,
	}, nil
}

// SendTo sends a packet to a specific address
func (t *UDPTransport) SendTo(data []byte, addr *net.UDPAddr) error {
	_, err := t.conn.WriteToUDP(data, addr)
	if err != nil {
		return fmt.Errorf("failed to send UDP packet: %w", err)
	}
	return nil
}

// ReceiveFrom receives a packet
func (t *UDPTransport) ReceiveFrom(buffer []byte) (int, *net.UDPAddr, error) {
	n, addr, err := t.conn.ReadFromUDP(buffer)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to receive UDP packet: %w", err)
	}
	return n, addr, nil
}

// Close closes the UDP socket
func (t *UDPTransport) Close() error {
	return t.conn.Close()
}

// GetLocalAddr returns the local address
func (t *UDPTransport) GetLocalAddr() *net.UDPAddr {
	return t.listenAddr
}
