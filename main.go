package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const VERSION = "1.0.0"

type ESPCommand string

const (
	CommandPulse  ESPCommand = "pulse"
	CommandForce  ESPCommand = "force"
	CommandStatus ESPCommand = "status"
)

type ESP struct {
	ID       string
	Command  ESPCommand
	LastSeen time.Time
	Online   bool
}

var (
	espMap          = make(map[string]*ESP)
	mu              sync.Mutex
	serverPort      string
	serverURL       string
	timeoutDuration time.Duration
)

func main() {
	// Define flags
	portFlag := flag.String("port", "8080", "Server port")
	serverFlag := flag.String("server", "http://localhost:8080", "Server URL for client commands")
	timeoutFlag := flag.Duration("timeout", 30*time.Second, "ESP timeout duration")
	versionFlag := flag.Bool("version", false, "Print version")
	helpFlag := flag.Bool("help", false, "Show help")

	flag.Usage = printUsage
	flag.Parse()

	if *versionFlag {
		fmt.Printf("wake-on-demand v%s\n", VERSION)
		return
	}

	if *helpFlag {
		printUsage()
		return
	}

	serverPort = *portFlag
	serverURL = *serverFlag
	timeoutDuration = *timeoutFlag

	args := flag.Args()
	if len(args) < 1 {
		printUsage()
		os.Exit(1)
	}

	cmd := args[0]

	switch cmd {
	case "server":
		runServer()
	case "on", "off", "status":
		if len(args) < 2 {
			fmt.Printf("Usage: wake-on-demand %s <esp_id>\n", cmd)
			os.Exit(1)
		}
		sendCommand(cmd, args[1])
	case "list":
		listESPs()
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`wake-on-demand v%s - Remote server power control

USAGE:
    wake-on-demand [OPTIONS] <COMMAND> [ARGS]

COMMANDS:
    server              Start the server
    on <esp_id>         Send power on command (short pulse)
    off <esp_id>        Send force shutdown command (long pulse)
    status <esp_id>     Check target server connectivity
    list                List all registered ESPs

OPTIONS:
    -port <port>        Server port (default: 8080)
    -server <url>       Server URL for client commands (default: http://localhost:8080)
    -timeout <duration> ESP timeout duration (default: 30s)
    -version            Print version
    -help               Show this help

EXAMPLES:
    # Start server on default port
    wake-on-demand server

    # Start server on custom port
    wake-on-demand -port 9090 server

    # Send commands to custom server
    wake-on-demand -server http://192.168.1.100:8080 on bedroom

    # List ESPs
    wake-on-demand list

    # Power on server
    wake-on-demand on trashbin

`, VERSION)
}

// --- Server Mode ---

func runServer() {
	http.HandleFunc("/register", registerHandler)
	http.HandleFunc("/command", commandHandler)
	http.HandleFunc("/set-command", setCommandHandler)
	http.HandleFunc("/list", listHandler)
	http.HandleFunc("/health", healthHandler)

	go monitorESPs()

	log.Println("==============================================")
	log.Printf("Wake-On-Demand Server v%s", VERSION)
	log.Println("==============================================")
	log.Printf("Listening on: :%s", serverPort)
	log.Printf("ESP timeout: %v", timeoutDuration)
	log.Println("==============================================")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("\n[SHUTDOWN] Received shutdown signal")
		log.Println("[SHUTDOWN] Server stopping...")
		os.Exit(0)
	}()

	log.Fatal(http.ListenAndServe(":"+serverPort, nil))
}

func monitorESPs() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		mu.Lock()
		now := time.Now()
		for id, esp := range espMap {
			timeSinceLastSeen := now.Sub(esp.LastSeen)
			wasOnline := esp.Online
			esp.Online = timeSinceLastSeen < timeoutDuration

			if wasOnline && !esp.Online {
				log.Printf("[MONITOR] ESP went OFFLINE - ID: %s (last seen %v ago)", id, timeSinceLastSeen.Round(time.Second))
			} else if !wasOnline && esp.Online {
				log.Printf("[MONITOR] ESP is back ONLINE - ID: %s", id)
			}
		}
		mu.Unlock()
	}
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	clientIP := r.RemoteAddr
	log.Printf("[REGISTER] Request from %s", clientIP)

	if r.Method != http.MethodPost {
		log.Printf("[REGISTER] ERROR: Method not allowed from %s", clientIP)
		http.Error(w, "only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var data struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		log.Printf("[REGISTER] ERROR: Invalid JSON from %s: %v", clientIP, err)
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if data.ID == "" {
		log.Printf("[REGISTER] ERROR: Empty ID from %s", clientIP)
		http.Error(w, "id cannot be empty", http.StatusBadRequest)
		return
	}

	mu.Lock()
	if _, exists := espMap[data.ID]; !exists {
		espMap[data.ID] = &ESP{
			ID:       data.ID,
			Command:  "",
			LastSeen: time.Now(),
			Online:   true,
		}
		log.Printf("[REGISTER] SUCCESS: New ESP registered - ID: %s, IP: %s", data.ID, clientIP)
	} else {
		espMap[data.ID].LastSeen = time.Now()
		espMap[data.ID].Online = true
		log.Printf("[REGISTER] SUCCESS: ESP re-registered - ID: %s, IP: %s", data.ID, clientIP)
	}
	mu.Unlock()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
}

func commandHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	clientIP := r.RemoteAddr

	if id == "" {
		log.Printf("[POLL] ERROR: Missing ID from %s", clientIP)
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	esp, exists := espMap[id]
	if !exists {
		log.Printf("[POLL] ERROR: ESP not registered - ID: %s, IP: %s", id, clientIP)
		http.Error(w, "ESP not registered", http.StatusNotFound)
		return
	}

	esp.LastSeen = time.Now()
	esp.Online = true

	cmd := esp.Command
	esp.Command = ""

	if cmd != "" {
		log.Printf("[POLL] Command sent to ESP - ID: %s, Command: %s, IP: %s", id, cmd, clientIP)
	}

	resp := map[string]string{"command": string(cmd)}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func setCommandHandler(w http.ResponseWriter, r *http.Request) {
	clientIP := r.RemoteAddr
	log.Printf("[SET-COMMAND] Request from %s", clientIP)

	if r.Method != http.MethodPost {
		log.Printf("[SET-COMMAND] ERROR: Method not allowed from %s", clientIP)
		http.Error(w, "only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var data struct {
		ID      string `json:"id"`
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		log.Printf("[SET-COMMAND] ERROR: Invalid JSON from %s: %v", clientIP, err)
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	esp, exists := espMap[data.ID]
	if !exists {
		log.Printf("[SET-COMMAND] ERROR: ESP not found - ID: %s, IP: %s", data.ID, clientIP)
		http.Error(w, "ESP not registered", http.StatusNotFound)
		return
	}

	if !esp.Online {
		log.Printf("[SET-COMMAND] ERROR: ESP offline - ID: %s, IP: %s", data.ID, clientIP)
		http.Error(w, fmt.Sprintf("ESP '%s' is offline", data.ID), http.StatusServiceUnavailable)
		return
	}

	esp.Command = ESPCommand(data.Command)
	log.Printf("[SET-COMMAND] SUCCESS: Command queued - ID: %s, Command: %s, IP: %s", data.ID, data.Command, clientIP)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "queued",
		"id":      data.ID,
		"command": data.Command,
	})
}

func listHandler(w http.ResponseWriter, r *http.Request) {
	clientIP := r.RemoteAddr
	log.Printf("[LIST] Request from %s", clientIP)

	mu.Lock()
	defer mu.Unlock()

	type ESPInfo struct {
		ID       string `json:"id"`
		Online   bool   `json:"online"`
		LastSeen string `json:"last_seen"`
	}

	esps := make([]ESPInfo, 0, len(espMap))
	for id, esp := range espMap {
		esps = append(esps, ESPInfo{
			ID:       id,
			Online:   esp.Online,
			LastSeen: time.Since(esp.LastSeen).Round(time.Second).String() + " ago",
		})
	}

	log.Printf("[LIST] SUCCESS: Returned %d ESP(s) to %s", len(esps), clientIP)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string][]ESPInfo{"esps": esps})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	espCount := len(espMap)
	onlineCount := 0
	for _, esp := range espMap {
		if esp.Online {
			onlineCount++
		}
	}
	mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"version": VERSION,
		"esps": map[string]int{
			"total":  espCount,
			"online": onlineCount,
		},
	})
}

// --- Client Mode ---

func sendCommand(cmd, espID string) {
	var command string
	switch cmd {
	case "on":
		command = "pulse"
	case "off":
		command = "force"
	case "status":
		command = "status"
	}

	data := map[string]string{
		"id":      espID,
		"command": command,
	}
	jsonData, _ := json.Marshal(data)

	resp, err := http.Post(serverURL+"/set-command", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("Error: Could not connect to server at %s\n", serverURL)
		fmt.Println("Is the server running? Start with: wake-on-demand server")
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("✓ Command '%s' queued for %s\n", cmd, espID)
	} else if resp.StatusCode == http.StatusNotFound {
		fmt.Printf("✗ ESP '%s' not registered\n", espID)
		os.Exit(1)
	} else if resp.StatusCode == http.StatusServiceUnavailable {
		fmt.Printf("✗ ESP '%s' is offline\n", espID)
		os.Exit(1)
	} else {
		fmt.Printf("✗ Error: %s\n", resp.Status)
		os.Exit(1)
	}
}

func listESPs() {
	resp, err := http.Get(serverURL + "/list")
	if err != nil {
		fmt.Printf("Error: Could not connect to server at %s\n", serverURL)
		fmt.Println("Is the server running? Start with: wake-on-demand server")
		os.Exit(1)
	}
	defer resp.Body.Close()

	var result struct {
		ESPs []struct {
			ID       string `json:"id"`
			Online   bool   `json:"online"`
			LastSeen string `json:"last_seen"`
		} `json:"esps"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Println("Error decoding response")
		os.Exit(1)
	}

	if len(result.ESPs) == 0 {
		fmt.Println("No ESPs registered")
	} else {
		fmt.Println("Registered ESPs:")
		for _, esp := range result.ESPs {
			status := "●"
			statusColor := "\033[32m" // green
			if !esp.Online {
				statusColor = "\033[31m" // red
			}
			fmt.Printf("  %s%s\033[0m %-20s [last seen: %s]\n", statusColor, status, esp.ID, esp.LastSeen)
		}
	}
}
