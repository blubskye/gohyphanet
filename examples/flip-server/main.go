package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/blubskye/gohyphanet/flip"
)

func main() {
	fmt.Println("===========================================")
	fmt.Println("    FLIP - Freenet/Hyphanet IRC Proxy")
	fmt.Println("===========================================")
	fmt.Println()

	// Create IRC server
	fmt.Println("Starting FLIP IRC server...")
	ircConfig := flip.DefaultServerConfig()
	ircServer := flip.NewIRCServer(ircConfig)

	if err := ircServer.Start(); err != nil {
		log.Fatalf("Failed to start FLIP IRC server: %v", err)
	}

	fmt.Println()
	fmt.Println("===========================================")
	fmt.Println("      FLIP STARTED SUCCESSFULLY")
	fmt.Println("===========================================")
	fmt.Println()
	fmt.Println("📡 IRC Server:       127.0.0.1:6668")
	fmt.Println("🔌 FCP Backend:      127.0.0.1:9481")
	fmt.Println()
	fmt.Println("===========================================")
	fmt.Println()
	fmt.Println("To connect with an IRC client:")
	fmt.Println("  Server: 127.0.0.1")
	fmt.Println("  Port:   6668")
	fmt.Println()
	fmt.Println("Popular IRC clients:")
	fmt.Println("  - HexChat (GUI)")
	fmt.Println("  - irssi (Terminal)")
	fmt.Println("  - WeeChat (Terminal)")
	fmt.Println()
	fmt.Println("Quick test with netcat:")
	fmt.Println("  nc 127.0.0.1 6668")
	fmt.Println("  NICK testuser")
	fmt.Println("  USER test 0 * :Test User")
	fmt.Println("  JOIN #test")
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop the server...")
	fmt.Println("===========================================")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println()
	fmt.Println("Shutting down...")
	ircServer.Stop()
	fmt.Println("FLIP stopped. Goodbye!")
}
