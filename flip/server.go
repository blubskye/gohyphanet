package flip

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultIRCPort is the standard IRC port
	DefaultIRCPort = 6668

	// ServerName is the IRC server name
	ServerName = "flip.hyphanet"

	// Version is the flip version
	Version = "0.1.0-go"
)

// IRCServer is the flip IRC server
type IRCServer struct {
	mu sync.RWMutex

	// Configuration
	bindAddr string
	port     int
	enabled  bool

	// Server state
	listener net.Listener
	clients  map[*IRCClient]bool
	channels map[string]*IRCChannel

	// FCP connection
	fcpAddr string
	fcpPort int

	// Shutdown
	shutdown chan struct{}
}

// ServerConfig contains IRC server configuration
type ServerConfig struct {
	BindAddr string
	Port     int
	Enabled  bool
	FCPAddr  string
	FCPPort  int
}

// DefaultServerConfig returns default IRC server configuration
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		BindAddr: "127.0.0.1",
		Port:     DefaultIRCPort,
		Enabled:  true,
		FCPAddr:  "127.0.0.1",
		FCPPort:  9481,
	}
}

// NewIRCServer creates a new flip IRC server
func NewIRCServer(config *ServerConfig) *IRCServer {
	if config == nil {
		config = DefaultServerConfig()
	}

	return &IRCServer{
		bindAddr: config.BindAddr,
		port:     config.Port,
		enabled:  config.Enabled,
		fcpAddr:  config.FCPAddr,
		fcpPort:  config.FCPPort,
		clients:  make(map[*IRCClient]bool),
		channels: make(map[string]*IRCChannel),
		shutdown: make(chan struct{}),
	}
}

// Start starts the IRC server
func (s *IRCServer) Start() error {
	if !s.enabled {
		log.Printf("IRC server disabled, not starting")
		return nil
	}

	addr := fmt.Sprintf("%s:%d", s.bindAddr, s.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start IRC server: %v", err)
	}

	s.listener = listener
	log.Printf("FLIP IRC server listening on %s", addr)
	log.Printf("Users can connect with IRC clients to: %s", addr)

	go s.acceptConnections()

	return nil
}

// Stop stops the IRC server
func (s *IRCServer) Stop() error {
	if s.listener == nil {
		return nil
	}

	log.Println("Stopping FLIP IRC server...")
	close(s.shutdown)

	// Close all client connections
	s.mu.Lock()
	for client := range s.clients {
		client.conn.Close()
	}
	s.mu.Unlock()

	return s.listener.Close()
}

// acceptConnections accepts incoming IRC client connections
func (s *IRCServer) acceptConnections() {
	for {
		select {
		case <-s.shutdown:
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.shutdown:
				return
			default:
				log.Printf("Error accepting connection: %v", err)
				continue
			}
		}

		client := NewIRCClient(conn, s)
		s.mu.Lock()
		s.clients[client] = true
		s.mu.Unlock()

		go client.Handle()
	}
}

// IRCClient represents an IRC client connection
type IRCClient struct {
	conn     net.Conn
	server   *IRCServer
	nick     string
	user     string
	realname string
	channels map[string]bool
	writer   *bufio.Writer
	mu       sync.Mutex
}

// NewIRCClient creates a new IRC client
func NewIRCClient(conn net.Conn, server *IRCServer) *IRCClient {
	return &IRCClient{
		conn:     conn,
		server:   server,
		channels: make(map[string]bool),
		writer:   bufio.NewWriter(conn),
	}
}

// Handle handles an IRC client connection
func (c *IRCClient) Handle() {
	defer func() {
		c.server.mu.Lock()
		delete(c.server.clients, c)
		c.server.mu.Unlock()
		c.conn.Close()
	}()

	// Send welcome when connected
	c.sendWelcome()

	scanner := bufio.NewScanner(c.conn)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			continue
		}

		log.Printf("[IRC] <- %s: %s", c.conn.RemoteAddr(), line)
		c.handleCommand(line)
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Client %s disconnected: %v", c.conn.RemoteAddr(), err)
	}
}

// sendWelcome sends initial connection message
func (c *IRCClient) sendWelcome() {
	c.send(fmt.Sprintf(":%s NOTICE * :*** Looking up your hostname...", ServerName))
	c.send(fmt.Sprintf(":%s NOTICE * :*** Found your hostname", ServerName))
}

// handleCommand processes IRC commands
func (c *IRCClient) handleCommand(line string) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return
	}

	command := strings.ToUpper(parts[0])

	switch command {
	case "NICK":
		c.handleNick(parts)
	case "USER":
		c.handleUser(parts)
	case "PING":
		c.handlePing(parts)
	case "JOIN":
		c.handleJoin(parts)
	case "PART":
		c.handlePart(parts)
	case "PRIVMSG":
		c.handlePrivmsg(parts, line)
	case "QUIT":
		c.handleQuit(parts)
	case "MODE":
		c.handleMode(parts)
	case "WHO":
		c.handleWho(parts)
	case "WHOIS":
		c.handleWhois(parts)
	case "MOTD":
		c.sendMotd()
	default:
		c.send(fmt.Sprintf(":%s 421 %s %s :Unknown command", ServerName, c.getNick(), command))
	}
}

// handleNick handles NICK command
func (c *IRCClient) handleNick(parts []string) {
	if len(parts) < 2 {
		c.send(fmt.Sprintf(":%s 431 :No nickname given", ServerName))
		return
	}

	oldNick := c.nick
	c.nick = parts[1]

	if oldNick != "" {
		// Nick change
		c.send(fmt.Sprintf(":%s!%s@%s NICK :%s", oldNick, c.user, "flip", c.nick))
	} else if c.user != "" {
		// Registration complete
		c.sendRegistration()
	}
}

// handleUser handles USER command
func (c *IRCClient) handleUser(parts []string) {
	if len(parts) < 5 {
		c.send(fmt.Sprintf(":%s 461 USER :Not enough parameters", ServerName))
		return
	}

	c.user = parts[1]
	// parts[2] and parts[3] are mode and unused
	c.realname = strings.Join(parts[4:], " ")
	if strings.HasPrefix(c.realname, ":") {
		c.realname = c.realname[1:]
	}

	if c.nick != "" {
		c.sendRegistration()
	}
}

// handlePing handles PING command
func (c *IRCClient) handlePing(parts []string) {
	if len(parts) > 1 {
		c.send(fmt.Sprintf(":%s PONG %s :%s", ServerName, ServerName, parts[1]))
	} else {
		c.send(fmt.Sprintf(":%s PONG %s", ServerName, ServerName))
	}
}

// handleJoin handles JOIN command
func (c *IRCClient) handleJoin(parts []string) {
	if len(parts) < 2 {
		c.send(fmt.Sprintf(":%s 461 JOIN :Not enough parameters", ServerName))
		return
	}

	channelName := parts[1]
	if !strings.HasPrefix(channelName, "#") {
		channelName = "#" + channelName
	}

	c.server.mu.Lock()
	channel, exists := c.server.channels[channelName]
	if !exists {
		channel = NewIRCChannel(channelName)
		c.server.channels[channelName] = channel
	}
	c.server.mu.Unlock()

	channel.Join(c)
	c.channels[channelName] = true

	// Send JOIN confirmation
	c.send(fmt.Sprintf(":%s!%s@%s JOIN :%s", c.nick, c.user, "flip", channelName))

	// Send topic
	if channel.topic != "" {
		c.send(fmt.Sprintf(":%s 332 %s %s :%s", ServerName, c.nick, channelName, channel.topic))
	} else {
		c.send(fmt.Sprintf(":%s 331 %s %s :No topic is set", ServerName, c.nick, channelName))
	}

	// Send names list
	names := channel.GetNames()
	c.send(fmt.Sprintf(":%s 353 %s = %s :%s", ServerName, c.nick, channelName, strings.Join(names, " ")))
	c.send(fmt.Sprintf(":%s 366 %s %s :End of /NAMES list", ServerName, c.nick, channelName))
}

// handlePart handles PART command
func (c *IRCClient) handlePart(parts []string) {
	if len(parts) < 2 {
		return
	}

	channelName := parts[1]
	c.server.mu.RLock()
	channel, exists := c.server.channels[channelName]
	c.server.mu.RUnlock()

	if !exists {
		return
	}

	delete(c.channels, channelName)
	channel.Part(c)

	c.send(fmt.Sprintf(":%s!%s@%s PART %s", c.nick, c.user, "flip", channelName))
}

// handlePrivmsg handles PRIVMSG command
func (c *IRCClient) handlePrivmsg(parts []string, line string) {
	if len(parts) < 3 {
		return
	}

	target := parts[1]
	msgStart := strings.Index(line, ":")
	if msgStart == -1 {
		return
	}
	message := line[msgStart+1:]

	if strings.HasPrefix(target, "#") {
		// Channel message
		c.server.mu.RLock()
		channel, exists := c.server.channels[target]
		c.server.mu.RUnlock()

		if exists {
			channel.SendMessage(c, message)
		}
	} else {
		// Private message - not implemented yet
		c.send(fmt.Sprintf(":%s 401 %s %s :No such nick/channel", ServerName, c.nick, target))
	}
}

// handleQuit handles QUIT command
func (c *IRCClient) handleQuit(parts []string) {
	quitMsg := "Client quit"
	if len(parts) > 1 {
		quitMsg = strings.Join(parts[1:], " ")
		if strings.HasPrefix(quitMsg, ":") {
			quitMsg = quitMsg[1:]
		}
	}

	// Part all channels
	for channelName := range c.channels {
		c.server.mu.RLock()
		channel, exists := c.server.channels[channelName]
		c.server.mu.RUnlock()

		if exists {
			channel.Part(c)
		}
	}

	c.send(fmt.Sprintf(":%s!%s@%s QUIT :%s", c.nick, c.user, "flip", quitMsg))
	c.conn.Close()
}

// handleMode handles MODE command
func (c *IRCClient) handleMode(parts []string) {
	if len(parts) < 2 {
		return
	}

	target := parts[1]
	if strings.HasPrefix(target, "#") {
		// Channel mode
		c.send(fmt.Sprintf(":%s 324 %s %s +nt", ServerName, c.nick, target))
	} else {
		// User mode
		c.send(fmt.Sprintf(":%s 221 %s +i", ServerName, c.nick))
	}
}

// handleWho handles WHO command
func (c *IRCClient) handleWho(parts []string) {
	if len(parts) < 2 {
		return
	}

	target := parts[1]
	c.server.mu.RLock()
	channel, exists := c.server.channels[target]
	c.server.mu.RUnlock()

	if exists {
		for client := range channel.clients {
			c.send(fmt.Sprintf(":%s 352 %s %s %s %s %s %s H :0 %s",
				ServerName, c.nick, target, client.user, "flip", ServerName,
				client.nick, client.realname))
		}
	}

	c.send(fmt.Sprintf(":%s 315 %s %s :End of /WHO list", ServerName, c.nick, target))
}

// handleWhois handles WHOIS command
func (c *IRCClient) handleWhois(parts []string) {
	if len(parts) < 2 {
		return
	}

	nick := parts[1]
	c.send(fmt.Sprintf(":%s 311 %s %s %s flip * :%s", ServerName, c.nick, nick, nick, "Flip User"))
	c.send(fmt.Sprintf(":%s 312 %s %s %s :Flip IRC Bridge", ServerName, c.nick, nick, ServerName))
	c.send(fmt.Sprintf(":%s 318 %s %s :End of /WHOIS list", ServerName, c.nick, nick))
}

// sendRegistration sends registration complete messages
func (c *IRCClient) sendRegistration() {
	c.send(fmt.Sprintf(":%s 001 %s :Welcome to the Flip IRC bridge %s!%s@flip",
		ServerName, c.nick, c.nick, c.user))
	c.send(fmt.Sprintf(":%s 002 %s :Your host is %s, running version %s",
		ServerName, c.nick, ServerName, Version))
	c.send(fmt.Sprintf(":%s 003 %s :This server was created %s",
		ServerName, c.nick, time.Now().Format("Mon Jan 2 2006")))
	c.send(fmt.Sprintf(":%s 004 %s %s %s i nt",
		ServerName, c.nick, ServerName, Version))

	c.sendMotd()
}

// sendMotd sends the message of the day
func (c *IRCClient) sendMotd() {
	c.send(fmt.Sprintf(":%s 375 %s :- %s Message of the day -", ServerName, c.nick, ServerName))
	c.send(fmt.Sprintf(":%s 372 %s :- Welcome to FLIP v%s", ServerName, c.nick, Version))
	c.send(fmt.Sprintf(":%s 372 %s :- ", ServerName, c.nick))
	c.send(fmt.Sprintf(":%s 372 %s :- FLIP (Freenet/Hyphanet IRC Proxy)", ServerName, c.nick))
	c.send(fmt.Sprintf(":%s 372 %s :- Messages are bridged to/from Freenet", ServerName, c.nick))
	c.send(fmt.Sprintf(":%s 372 %s :- Join a channel to start chatting!", ServerName, c.nick))
	c.send(fmt.Sprintf(":%s 376 %s :End of /MOTD command", ServerName, c.nick))
}

// send sends a message to the client
func (c *IRCClient) send(msg string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	log.Printf("[IRC] -> %s: %s", c.conn.RemoteAddr(), msg)
	c.writer.WriteString(msg + "\r\n")
	c.writer.Flush()
}

// getNick returns the client's nick or * if not set
func (c *IRCClient) getNick() string {
	if c.nick == "" {
		return "*"
	}
	return c.nick
}

// IRCChannel represents an IRC channel
type IRCChannel struct {
	mu      sync.RWMutex
	name    string
	topic   string
	clients map[*IRCClient]bool
}

// NewIRCChannel creates a new IRC channel
func NewIRCChannel(name string) *IRCChannel {
	return &IRCChannel{
		name:    name,
		clients: make(map[*IRCClient]bool),
		topic:   "Freenet/Hyphanet IRC channel - Messages are bridged to Freenet",
	}
}

// Join adds a client to the channel
func (ch *IRCChannel) Join(client *IRCClient) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.clients[client] = true
}

// Part removes a client from the channel
func (ch *IRCChannel) Part(client *IRCClient) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	delete(ch.clients, client)
}

// SendMessage sends a message to all clients in the channel
func (ch *IRCChannel) SendMessage(sender *IRCClient, message string) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	msg := fmt.Sprintf(":%s!%s@%s PRIVMSG %s :%s",
		sender.nick, sender.user, "flip", ch.name, message)

	for client := range ch.clients {
		if client != sender {
			client.send(msg)
		}
	}

	// TODO: Bridge message to Freenet via FCP
	log.Printf("[FLIP] Channel %s: <%s> %s", ch.name, sender.nick, message)
}

// GetNames returns a list of nicks in the channel
func (ch *IRCChannel) GetNames() []string {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	names := make([]string, 0, len(ch.clients))
	for client := range ch.clients {
		names = append(names, client.nick)
	}
	return names
}
