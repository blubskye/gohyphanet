// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

// Command gofreemail provides a CLI for GoFreemail.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blubskye/gohyphanet/freemail"
	"github.com/blubskye/gohyphanet/freemail/imap"
	"github.com/blubskye/gohyphanet/freemail/smtp"
	"github.com/blubskye/gohyphanet/freemail/web"
)

// Version info
const (
	Version   = "0.1.0"
	AppName   = "GoFreemail"
)

// Configuration
type Config struct {
	DataDir  string
	FCPHost  string
	FCPPort  int
	SMTPPort int
	IMAPPort int
	WebPort  int
}

var config = Config{
	DataDir:  defaultDataDir(),
	FCPHost:  "localhost",
	FCPPort:  9481,
	SMTPPort: freemail.DefaultSMTPPort,
	IMAPPort: freemail.DefaultIMAPPort,
	WebPort:  freemail.DefaultWebPort,
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".gofreemail"
	}
	return filepath.Join(home, ".gofreemail")
}

func main() {
	// Global flags
	flag.StringVar(&config.DataDir, "data", config.DataDir, "Data directory")
	flag.StringVar(&config.FCPHost, "fcp-host", config.FCPHost, "FCP host")
	flag.IntVar(&config.FCPPort, "fcp-port", config.FCPPort, "FCP port")
	flag.IntVar(&config.SMTPPort, "smtp-port", config.SMTPPort, "SMTP port")
	flag.IntVar(&config.IMAPPort, "imap-port", config.IMAPPort, "IMAP port")
	flag.IntVar(&config.WebPort, "web-port", config.WebPort, "Web UI port")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s v%s - Anonymous email over Freenet\n\n", AppName, Version)
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <command> [arguments]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  serve       Start SMTP, IMAP, and Web servers\n")
		fmt.Fprintf(os.Stderr, "  accounts    List accounts\n")
		fmt.Fprintf(os.Stderr, "  create      Create a new account\n")
		fmt.Fprintf(os.Stderr, "  inbox       List inbox messages\n")
		fmt.Fprintf(os.Stderr, "  read        Read a message\n")
		fmt.Fprintf(os.Stderr, "  send        Send a message\n")
		fmt.Fprintf(os.Stderr, "  status      Show status\n")
		fmt.Fprintf(os.Stderr, "  version     Show version\n")
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	command := flag.Arg(0)
	args := flag.Args()[1:]

	switch command {
	case "serve":
		cmdServe(args)
	case "accounts":
		cmdAccounts(args)
	case "create":
		cmdCreate(args)
	case "inbox":
		cmdInbox(args)
	case "read":
		cmdRead(args)
	case "send":
		cmdSend(args)
	case "status":
		cmdStatus(args)
	case "version":
		cmdVersion()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		flag.Usage()
		os.Exit(1)
	}
}

// cmdServe starts the servers
func cmdServe(args []string) {
	fmt.Printf("%s v%s\n", AppName, Version)
	fmt.Printf("Data directory: %s\n", config.DataDir)

	// Initialize storage
	storage := freemail.NewStorage(config.DataDir)
	if err := storage.Initialize(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize storage: %v\n", err)
		os.Exit(1)
	}

	// Load accounts
	accountManager := freemail.NewAccountManager(storage)
	if err := accountManager.LoadAccounts(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load accounts: %v\n", err)
		os.Exit(1)
	}

	accounts := accountManager.GetAccounts()
	fmt.Printf("Loaded %d account(s)\n", len(accounts))

	// Start SMTP server
	smtpServer := smtp.NewServer(config.SMTPPort, "localhost")
	smtpServer.SetAuthenticator(smtp.NewFreemailAuthenticator(accountManager))
	smtpServer.SetHandler(smtp.NewFreemailHandler(accountManager, nil, storage))
	if err := smtpServer.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start SMTP server: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("SMTP server started on port %d\n", config.SMTPPort)

	// Start IMAP server
	imapServer := imap.NewServer(config.IMAPPort, "localhost")
	imapServer.SetAccountManager(accountManager)
	imapServer.SetStorage(storage)
	if err := imapServer.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start IMAP server: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("IMAP server started on port %d\n", config.IMAPPort)

	// Start Web server
	webServer := web.NewServer(config.WebPort)
	webServer.SetAccountManager(accountManager)
	webServer.SetStorage(storage)
	if err := webServer.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start Web server: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Web UI available at http://localhost:%d\n", config.WebPort)

	fmt.Println("\nPress Ctrl+C to stop...")

	// Wait forever
	select {}
}

// cmdAccounts lists accounts
func cmdAccounts(args []string) {
	storage := freemail.NewStorage(config.DataDir)
	accountManager := freemail.NewAccountManager(storage)
	if err := accountManager.LoadAccounts(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load accounts: %v\n", err)
		os.Exit(1)
	}

	accounts := accountManager.GetAccounts()
	if len(accounts) == 0 {
		fmt.Println("No accounts found.")
		fmt.Println("Create one with: gofreemail create <nickname> <password>")
		return
	}

	fmt.Printf("%-20s %-40s %s\n", "NICKNAME", "EMAIL", "CREATED")
	fmt.Println(strings.Repeat("-", 80))

	for _, acc := range accounts {
		email := acc.GetEmailAddress()
		emailStr := ""
		if email != nil {
			emailStr = email.String()
		}
		fmt.Printf("%-20s %-40s %s\n",
			acc.Nickname,
			emailStr,
			acc.Created.Format("2006-01-02"),
		)
	}
}

// cmdCreate creates a new account
func cmdCreate(args []string) {
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: gofreemail create <nickname> <password>\n")
		os.Exit(1)
	}

	nickname := args[0]
	password := args[1]

	storage := freemail.NewStorage(config.DataDir)
	if err := storage.Initialize(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize storage: %v\n", err)
		os.Exit(1)
	}

	accountManager := freemail.NewAccountManager(storage)
	if err := accountManager.LoadAccounts(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load accounts: %v\n", err)
		os.Exit(1)
	}

	// Generate a simple ID for now (would use WoT identity in production)
	id := fmt.Sprintf("local-%d", time.Now().Unix())

	account, err := accountManager.CreateAccount(id, nickname, password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create account: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Account created successfully!\n")
	fmt.Printf("  Nickname: %s\n", account.Nickname)
	fmt.Printf("  ID: %s\n", account.ID)
	fmt.Printf("\nNote: To receive email, you'll need to link this account\n")
	fmt.Printf("to a Web of Trust identity.\n")
}

// cmdInbox lists inbox messages
func cmdInbox(args []string) {
	var accountID string
	if len(args) > 0 {
		accountID = args[0]
	}

	storage := freemail.NewStorage(config.DataDir)
	accountManager := freemail.NewAccountManager(storage)
	if err := accountManager.LoadAccounts(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load accounts: %v\n", err)
		os.Exit(1)
	}

	var account *freemail.Account
	if accountID != "" {
		account = accountManager.GetAccount(accountID)
		if account == nil {
			account = accountManager.GetAccountByEmail(accountID)
		}
	} else {
		accounts := accountManager.GetAccounts()
		if len(accounts) == 0 {
			fmt.Println("No accounts found.")
			return
		}
		account = accounts[0]
	}

	if account == nil {
		fmt.Fprintf(os.Stderr, "Account not found\n")
		os.Exit(1)
	}

	inbox := account.Inbox
	if inbox.Count() == 0 {
		fmt.Printf("Inbox for %s is empty.\n", account.Nickname)
		return
	}

	fmt.Printf("Inbox for %s (%d messages)\n\n", account.Nickname, inbox.Count())
	fmt.Printf("%-6s %-5s %-20s %-30s %s\n", "UID", "FLAGS", "FROM", "SUBJECT", "DATE")
	fmt.Println(strings.Repeat("-", 90))

	for _, msg := range inbox.Messages {
		flags := ""
		if !msg.HasFlag(freemail.FlagSeen) {
			flags += "N"
		}
		if msg.HasFlag(freemail.FlagFlagged) {
			flags += "*"
		}

		from := "Unknown"
		if msg.From != nil {
			from = msg.From.Local
		}

		subject := msg.Subject
		if subject == "" {
			subject = "(No Subject)"
		}
		if len(subject) > 30 {
			subject = subject[:27] + "..."
		}

		fmt.Printf("%-6d %-5s %-20s %-30s %s\n",
			msg.UID,
			flags,
			truncate(from, 20),
			subject,
			msg.Date.Format("Jan 02 15:04"),
		)
	}
}

// cmdRead reads a message
func cmdRead(args []string) {
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: gofreemail read <account> <uid>\n")
		os.Exit(1)
	}

	accountID := args[0]
	var uid uint32
	fmt.Sscanf(args[1], "%d", &uid)

	storage := freemail.NewStorage(config.DataDir)
	accountManager := freemail.NewAccountManager(storage)
	if err := accountManager.LoadAccounts(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load accounts: %v\n", err)
		os.Exit(1)
	}

	account := accountManager.GetAccount(accountID)
	if account == nil {
		account = accountManager.GetAccountByEmail(accountID)
	}
	if account == nil {
		fmt.Fprintf(os.Stderr, "Account not found: %s\n", accountID)
		os.Exit(1)
	}

	msg := account.Inbox.GetMessage(uid)
	if msg == nil {
		fmt.Fprintf(os.Stderr, "Message not found: %d\n", uid)
		os.Exit(1)
	}

	// Mark as read
	msg.SetFlag(freemail.FlagSeen)

	// Display message
	fmt.Printf("From: %s\n", formatAddress(msg.From))
	fmt.Printf("To: ")
	for i, to := range msg.To {
		if i > 0 {
			fmt.Printf(", ")
		}
		fmt.Printf("%s", formatAddress(to))
	}
	fmt.Println()
	fmt.Printf("Date: %s\n", msg.Date.Format(time.RFC1123))
	fmt.Printf("Subject: %s\n", msg.Subject)
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("%s\n", msg.Body)
}

// cmdSend sends a message
func cmdSend(args []string) {
	if len(args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: gofreemail send <from-account> <to> <subject> <body>\n")
		os.Exit(1)
	}

	accountID := args[0]
	to := args[1]
	subject := args[2]
	body := args[3]

	storage := freemail.NewStorage(config.DataDir)
	accountManager := freemail.NewAccountManager(storage)
	if err := accountManager.LoadAccounts(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load accounts: %v\n", err)
		os.Exit(1)
	}

	account := accountManager.GetAccount(accountID)
	if account == nil {
		account = accountManager.GetAccountByEmail(accountID)
	}
	if account == nil {
		fmt.Fprintf(os.Stderr, "Account not found: %s\n", accountID)
		os.Exit(1)
	}

	// Parse recipient
	toAddr, err := freemail.ParseEmailAddress(to)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid recipient address: %v\n", err)
		os.Exit(1)
	}

	// Create message
	msg := freemail.NewMessage()
	msg.From = account.GetEmailAddress()
	msg.To = []*freemail.EmailAddress{toAddr}
	msg.Subject = subject
	msg.Body = []byte(body)
	msg.MessageID = fmt.Sprintf("<%d@gofreemail>", time.Now().UnixNano())

	// Add to sent folder
	account.Sent.AddMessage(msg)

	// Save
	as := storage.GetAccountStorage(account.ID)
	as.SaveMessage(account.Sent, msg)

	fmt.Println("Message queued for delivery.")
	fmt.Printf("  To: %s\n", toAddr.String())
	fmt.Printf("  Subject: %s\n", subject)
}

// cmdStatus shows status
func cmdStatus(args []string) {
	fmt.Printf("%s v%s\n\n", AppName, Version)
	fmt.Printf("Configuration:\n")
	fmt.Printf("  Data directory: %s\n", config.DataDir)
	fmt.Printf("  FCP host: %s:%d\n", config.FCPHost, config.FCPPort)
	fmt.Printf("  SMTP port: %d\n", config.SMTPPort)
	fmt.Printf("  IMAP port: %d\n", config.IMAPPort)
	fmt.Printf("  Web port: %d\n", config.WebPort)

	// Check if data directory exists
	if _, err := os.Stat(config.DataDir); os.IsNotExist(err) {
		fmt.Printf("\nData directory does not exist.\n")
		fmt.Printf("Run 'gofreemail create' to create an account.\n")
		return
	}

	// Load accounts
	storage := freemail.NewStorage(config.DataDir)
	accountManager := freemail.NewAccountManager(storage)
	if err := accountManager.LoadAccounts(); err != nil {
		fmt.Fprintf(os.Stderr, "\nFailed to load accounts: %v\n", err)
		return
	}

	accounts := accountManager.GetAccounts()
	fmt.Printf("\nAccounts: %d\n", len(accounts))

	for _, acc := range accounts {
		unread := 0
		for _, msg := range acc.Inbox.Messages {
			if !msg.HasFlag(freemail.FlagSeen) {
				unread++
			}
		}
		fmt.Printf("  - %s: %d messages (%d unread)\n",
			acc.Nickname, acc.Inbox.Count(), unread)
	}
}

// cmdVersion shows version
func cmdVersion() {
	fmt.Printf("%s v%s\n", AppName, Version)
	fmt.Printf("Anonymous email over Freenet/Hyphanet\n")
}

// Helper functions

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func formatAddress(addr *freemail.EmailAddress) string {
	if addr == nil {
		return "Unknown"
	}
	return addr.String()
}
