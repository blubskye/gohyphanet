// GoSone CLI - Command-line interface for Sone on Hyphanet
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/blubskye/gohyphanet/fcp"
	"github.com/blubskye/gohyphanet/sone"
	"github.com/blubskye/gohyphanet/wot"
)

// CLIConfig holds CLI configuration
type CLIConfig struct {
	FCPHost string `json:"fcp_host"`
	FCPPort int    `json:"fcp_port"`
	DataDir string `json:"data_dir"`
	LocalID string `json:"local_id"` // Default local Sone ID
}

var (
	cliConfig  CLIConfig
	configPath string
)

func main() {
	// Global flags
	flag.StringVar(&cliConfig.FCPHost, "host", "localhost", "FCP host")
	flag.IntVar(&cliConfig.FCPPort, "port", 9481, "FCP port")
	flag.StringVar(&cliConfig.DataDir, "data", "", "Data directory")
	flag.StringVar(&configPath, "config", "", "Config file path")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "GoSone - Sone client for Hyphanet\n\n")
		fmt.Fprintf(os.Stderr, "Usage: gosone [options] <command> [arguments]\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  status              Show connection status\n")
		fmt.Fprintf(os.Stderr, "  identities          List local WoT identities\n")
		fmt.Fprintf(os.Stderr, "  sones               List known Sones\n")
		fmt.Fprintf(os.Stderr, "  feed [sone-id]      Show post feed\n")
		fmt.Fprintf(os.Stderr, "  view <sone-id>      View Sone profile\n")
		fmt.Fprintf(os.Stderr, "  post <text>         Create a post\n")
		fmt.Fprintf(os.Stderr, "  reply <post-id> <text>  Reply to a post\n")
		fmt.Fprintf(os.Stderr, "  follow <sone-id>    Follow a Sone\n")
		fmt.Fprintf(os.Stderr, "  unfollow <sone-id>  Unfollow a Sone\n")
		fmt.Fprintf(os.Stderr, "  search <query>      Search posts and Sones\n")
		fmt.Fprintf(os.Stderr, "  serve [addr]        Start web server\n")
		fmt.Fprintf(os.Stderr, "  set-id <sone-id>    Set default local Sone ID\n")
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	// Load config
	loadConfig()

	// Set default data dir
	if cliConfig.DataDir == "" {
		home, _ := os.UserHomeDir()
		cliConfig.DataDir = filepath.Join(home, ".gosone")
	}

	// Ensure data directory exists
	os.MkdirAll(cliConfig.DataDir, 0755)

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	cmd := args[0]
	cmdArgs := args[1:]

	var err error
	switch cmd {
	case "status":
		err = cmdStatus()
	case "identities":
		err = cmdIdentities()
	case "sones":
		err = cmdSones()
	case "feed":
		err = cmdFeed(cmdArgs)
	case "view":
		err = cmdView(cmdArgs)
	case "post":
		err = cmdPost(cmdArgs)
	case "reply":
		err = cmdReply(cmdArgs)
	case "follow":
		err = cmdFollow(cmdArgs)
	case "unfollow":
		err = cmdUnfollow(cmdArgs)
	case "search":
		err = cmdSearch(cmdArgs)
	case "serve":
		err = cmdServe(cmdArgs)
	case "set-id":
		err = cmdSetID(cmdArgs)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		flag.Usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func loadConfig() {
	if configPath == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".gosone", "config.json")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return // Config file is optional
	}

	json.Unmarshal(data, &cliConfig)
}

func saveConfig() error {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cliConfig, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

func connectFCP() (*fcp.Client, error) {
	config := &fcp.Config{
		Host:    cliConfig.FCPHost,
		Port:    cliConfig.FCPPort,
		Name:    "GoSone-CLI",
		Version: "2.0",
	}
	client, err := fcp.Connect(config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to FCP: %w", err)
	}
	return client, nil
}

func connectWoT() (*wot.Client, *fcp.Client, error) {
	fcpClient, err := connectFCP()
	if err != nil {
		return nil, nil, err
	}

	wotClient := wot.NewClient(fcpClient)
	if err := wotClient.Ping(); err != nil {
		fcpClient.Close()
		return nil, nil, fmt.Errorf("WoT plugin not responding: %w", err)
	}

	return wotClient, fcpClient, nil
}

func loadCore() (*sone.Core, error) {
	config := &sone.Config{
		DataDir: cliConfig.DataDir,
		FCPHost: cliConfig.FCPHost,
		FCPPort: cliConfig.FCPPort,
	}

	core := sone.NewCore(config)
	if err := core.Start(); err != nil {
		return nil, fmt.Errorf("failed to start core: %w", err)
	}

	return core, nil
}

// cmdStatus shows connection status
func cmdStatus() error {
	fmt.Println("GoSone Status")
	fmt.Println("=============")
	fmt.Printf("FCP Host: %s:%d\n", cliConfig.FCPHost, cliConfig.FCPPort)
	fmt.Printf("Data Dir: %s\n", cliConfig.DataDir)
	if cliConfig.LocalID != "" {
		fmt.Printf("Default Sone: %s\n", shortID(cliConfig.LocalID))
	}

	// Test FCP connection
	fmt.Print("\nFCP Connection: ")
	fcpClient, err := connectFCP()
	if err != nil {
		fmt.Println("FAILED")
		return err
	}
	defer fcpClient.Close()
	fmt.Println("OK")

	// Test WoT
	fmt.Print("Web of Trust: ")
	wotClient := wot.NewClient(fcpClient)
	if err := wotClient.Ping(); err != nil {
		fmt.Println("FAILED")
		return err
	}
	fmt.Println("OK")

	// Get own identities
	identities, err := wotClient.GetOwnIdentities()
	if err != nil {
		return err
	}
	fmt.Printf("Local Identities: %d\n", len(identities))

	// Count Sone identities
	soneCount := 0
	for _, id := range identities {
		for _, ctx := range id.Contexts {
			if ctx == "Sone" {
				soneCount++
				break
			}
		}
	}
	fmt.Printf("Sone Identities: %d\n", soneCount)

	return nil
}

// cmdIdentities lists local WoT identities
func cmdIdentities() error {
	wotClient, fcpClient, err := connectWoT()
	if err != nil {
		return err
	}
	defer fcpClient.Close()

	identities, err := wotClient.GetOwnIdentities()
	if err != nil {
		return err
	}

	if len(identities) == 0 {
		fmt.Println("No local identities found.")
		fmt.Println("Create an identity in Web of Trust first.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NICKNAME\tID\tSONE CONTEXT\tDEFAULT")
	fmt.Fprintln(w, "--------\t--\t------------\t-------")

	for _, id := range identities {
		hasSone := "No"
		for _, ctx := range id.Contexts {
			if ctx == "Sone" {
				hasSone = "Yes"
				break
			}
		}
		isDefault := ""
		if id.ID == cliConfig.LocalID {
			isDefault = "*"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", id.Nickname, shortID(id.ID), hasSone, isDefault)
	}
	w.Flush()

	if cliConfig.LocalID == "" {
		fmt.Println("\nTip: Use 'gosone set-id <sone-id>' to set a default identity")
	}

	return nil
}

// cmdSones lists known Sones
func cmdSones() error {
	core, err := loadCore()
	if err != nil {
		return err
	}
	defer core.Stop()

	sones := core.GetAllSones()

	if len(sones) == 0 {
		fmt.Println("No known Sones yet.")
		fmt.Println("Sones will be discovered through Web of Trust.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tID\tPOSTS\tLAST UPDATE\tLOCAL")
	fmt.Fprintln(w, "----\t--\t-----\t-----------\t-----")

	for _, s := range sones {
		lastUpdate := "Never"
		if s.Time > 0 {
			lastUpdate = time.Unix(s.Time/1000, 0).Format("2006-01-02 15:04")
		}
		isLocal := ""
		if s.IsLocal {
			isLocal = "Yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n", s.Name, shortID(s.ID), len(s.Posts), lastUpdate, isLocal)
	}
	w.Flush()

	return nil
}

// cmdFeed shows the post feed
func cmdFeed(args []string) error {
	core, err := loadCore()
	if err != nil {
		return err
	}
	defer core.Stop()

	var soneID string
	if len(args) > 0 {
		soneID = args[0]
	} else if cliConfig.LocalID != "" {
		soneID = cliConfig.LocalID
	}

	var posts []*sone.Post

	if soneID != "" {
		// Get feed for specific Sone
		posts = core.GetPostFeed(soneID)
	} else {
		// Get all recent posts
		allSones := core.GetAllSones()
		for _, s := range allSones {
			posts = append(posts, s.Posts...)
		}
	}

	// Sort by time descending
	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Time > posts[j].Time
	})

	// Limit to 20
	if len(posts) > 20 {
		posts = posts[:20]
	}

	if len(posts) == 0 {
		fmt.Println("No posts found.")
		return nil
	}

	for _, post := range posts {
		printPost(core, post)
		fmt.Println()
	}

	return nil
}

// cmdView shows a Sone's profile
func cmdView(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: gosone view <sone-id>")
	}

	soneID := args[0]

	core, err := loadCore()
	if err != nil {
		return err
	}
	defer core.Stop()

	s := core.GetSone(soneID)
	if s == nil {
		return fmt.Errorf("Sone not found: %s", soneID)
	}

	fmt.Printf("Sone: %s\n", s.Name)
	fmt.Printf("ID: %s\n", s.ID)
	if s.IsLocal {
		fmt.Println("Type: Local")
	} else {
		fmt.Println("Type: Remote")
	}
	fmt.Println(strings.Repeat("=", 50))

	if s.Profile != nil {
		if s.Profile.FirstName != "" || s.Profile.LastName != "" {
			fmt.Printf("Name: %s %s\n", s.Profile.FirstName, s.Profile.LastName)
		}
		if s.Profile.BirthYear != nil && *s.Profile.BirthYear > 0 {
			day, month, year := 0, 0, *s.Profile.BirthYear
			if s.Profile.BirthDay != nil {
				day = *s.Profile.BirthDay
			}
			if s.Profile.BirthMonth != nil {
				month = *s.Profile.BirthMonth
			}
			fmt.Printf("Born: %d/%d/%d\n", day, month, year)
		}
		for _, field := range s.Profile.Fields {
			fmt.Printf("%s: %s\n", field.Name, field.Value)
		}
	}

	fmt.Printf("\nPosts: %d\n", len(s.Posts))
	fmt.Printf("Following: %d\n", len(s.Friends))

	if s.Time > 0 {
		fmt.Printf("Last Updated: %s\n", time.Unix(s.Time/1000, 0).Format("2006-01-02 15:04:05"))
	}

	// Show recent posts
	if len(s.Posts) > 0 {
		fmt.Println("\nRecent Posts:")
		fmt.Println(strings.Repeat("-", 50))

		posts := make([]*sone.Post, len(s.Posts))
		copy(posts, s.Posts)

		sort.Slice(posts, func(i, j int) bool {
			return posts[i].Time > posts[j].Time
		})

		count := 5
		if len(posts) < count {
			count = len(posts)
		}
		for i := 0; i < count; i++ {
			printPost(core, posts[i])
			fmt.Println()
		}
	}

	return nil
}

// cmdPost creates a new post
func cmdPost(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: gosone post <text>")
	}

	text := strings.Join(args, " ")

	if cliConfig.LocalID == "" {
		return fmt.Errorf("no local Sone ID configured. Use 'gosone set-id <sone-id>' to set one")
	}

	core, err := loadCore()
	if err != nil {
		return err
	}
	defer core.Stop()

	post, err := core.CreatePost(cliConfig.LocalID, text, nil)
	if err != nil {
		return err
	}

	fmt.Printf("Post created: %s\n", shortID(post.ID))
	fmt.Printf("Text: %s\n", post.Text)

	return nil
}

// cmdReply creates a reply to a post
func cmdReply(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: gosone reply <post-id> <text>")
	}

	postID := args[0]
	text := strings.Join(args[1:], " ")

	if cliConfig.LocalID == "" {
		return fmt.Errorf("no local Sone ID configured")
	}

	core, err := loadCore()
	if err != nil {
		return err
	}
	defer core.Stop()

	reply, err := core.CreateReply(cliConfig.LocalID, postID, text)
	if err != nil {
		return err
	}

	fmt.Printf("Reply created: %s\n", shortID(reply.ID))

	return nil
}

// cmdFollow follows a Sone
func cmdFollow(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: gosone follow <sone-id>")
	}

	targetID := args[0]

	if cliConfig.LocalID == "" {
		return fmt.Errorf("no local Sone ID configured")
	}

	core, err := loadCore()
	if err != nil {
		return err
	}
	defer core.Stop()

	if err := core.FollowSone(cliConfig.LocalID, targetID); err != nil {
		return err
	}

	fmt.Printf("Now following: %s\n", shortID(targetID))

	return nil
}

// cmdUnfollow unfollows a Sone
func cmdUnfollow(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: gosone unfollow <sone-id>")
	}

	targetID := args[0]

	if cliConfig.LocalID == "" {
		return fmt.Errorf("no local Sone ID configured")
	}

	core, err := loadCore()
	if err != nil {
		return err
	}
	defer core.Stop()

	if err := core.UnfollowSone(cliConfig.LocalID, targetID); err != nil {
		return err
	}

	fmt.Printf("Unfollowed: %s\n", shortID(targetID))

	return nil
}

// cmdSearch searches posts and Sones
func cmdSearch(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: gosone search <query>")
	}

	query := strings.ToLower(strings.Join(args, " "))

	core, err := loadCore()
	if err != nil {
		return err
	}
	defer core.Stop()

	// Search Sones
	fmt.Println("Matching Sones:")
	fmt.Println(strings.Repeat("-", 40))
	soneCount := 0
	for _, s := range core.GetAllSones() {
		if strings.Contains(strings.ToLower(s.Name), query) {
			fmt.Printf("  %s (%s)\n", s.Name, shortID(s.ID))
			soneCount++
		}
	}
	if soneCount == 0 {
		fmt.Println("  (none)")
	}

	// Search posts
	fmt.Println("\nMatching Posts:")
	fmt.Println(strings.Repeat("-", 40))
	postCount := 0
	for _, s := range core.GetAllSones() {
		for _, p := range s.Posts {
			if strings.Contains(strings.ToLower(p.Text), query) {
				printPost(core, p)
				fmt.Println()
				postCount++
				if postCount >= 10 {
					fmt.Println("  ... (showing first 10 results)")
					return nil
				}
			}
		}
	}
	if postCount == 0 {
		fmt.Println("  (none)")
	}

	return nil
}

// cmdServe starts the web server
func cmdServe(args []string) error {
	addr := ":8080"
	if len(args) > 0 {
		addr = args[0]
	}

	fmt.Printf("Starting GoSone web server on %s\n", addr)
	fmt.Printf("Data directory: %s\n", cliConfig.DataDir)
	fmt.Printf("FCP: %s:%d\n", cliConfig.FCPHost, cliConfig.FCPPort)
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()

	// Start core
	config := &sone.Config{
		DataDir: cliConfig.DataDir,
		FCPHost: cliConfig.FCPHost,
		FCPPort: cliConfig.FCPPort,
	}

	core := sone.NewCore(config)
	if err := core.Start(); err != nil {
		return fmt.Errorf("failed to start core: %w", err)
	}

	// Import and start web server
	// Note: Requires web package integration
	fmt.Println("Web server integration:")
	fmt.Println("  import \"github.com/blubskye/gohyphanet/sone/web\"")
	fmt.Println("  server := web.NewServer(core, addr)")
	fmt.Println("  server.Start()")
	fmt.Println()
	fmt.Println("For now, the server will run without web UI...")

	// Keep running until interrupted
	select {}
}

// cmdSetID sets the default local Sone ID
func cmdSetID(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: gosone set-id <sone-id>")
	}

	soneID := args[0]
	cliConfig.LocalID = soneID

	if err := saveConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Default Sone ID set to: %s\n", shortID(soneID))

	return nil
}

// Helper functions

func printPost(core *sone.Core, post *sone.Post) {
	author := core.GetSone(post.SoneID)
	authorName := shortID(post.SoneID)
	if author != nil {
		authorName = author.Name
	}

	postTime := time.Unix(post.Time/1000, 0).Format("2006-01-02 15:04")

	fmt.Printf("[%s] %s\n", postTime, authorName)

	// Truncate long posts
	text := post.Text
	if len(text) > 200 {
		text = text[:200] + "..."
	}
	fmt.Printf("  %s\n", text)

	// Show post ID (truncated)
	fmt.Printf("  ID: %s\n", shortID(post.ID))
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12] + "..."
	}
	return id
}
