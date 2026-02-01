package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v3"

	"github.com/power-edge/power-edge/pkg/config"
)

var (
	// Build-time variables (set via -ldflags)
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

// Server represents the power-edge control plane server
type Server struct {
	redis   *redis.Client
	version string // Schema version (e.g., "v1")
}

// NodeStateKey returns the Redis key for a node's state
func (s *Server) NodeStateKey(nodeID string) string {
	return fmt.Sprintf("%s:nodes:%s:state", s.version, nodeID)
}

// NodeVersionsKey returns the Redis key for a node's system versions
func (s *Server) NodeVersionsKey(nodeID string) string {
	return fmt.Sprintf("%s:nodes:%s:versions", s.version, nodeID)
}

// NodeComplianceKey returns the Redis key for a node's compliance status
func (s *Server) NodeComplianceKey(nodeID string) string {
	return fmt.Sprintf("%s:nodes:%s:compliance", s.version, nodeID)
}

// NodeHeartbeatKey returns the Redis key for a node's last heartbeat
func (s *Server) NodeHeartbeatKey(nodeID string) string {
	return fmt.Sprintf("%s:nodes:%s:heartbeat", s.version, nodeID)
}

func main() {
	// Flags
	redisAddr := flag.String("redis-addr", "localhost:6379", "Redis server address")
	redisPassword := flag.String("redis-password", "", "Redis password")
	redisDB := flag.Int("redis-db", 0, "Redis database number")
	listenAddr := flag.String("listen", ":8080", "HTTP server listen address")
	schemaVersion := flag.String("schema-version", "v1", "Control plane schema version")
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	// Version info
	if *versionFlag {
		fmt.Printf("power-edge-server %s\n", Version)
		fmt.Printf("  Git Commit: %s\n", GitCommit)
		fmt.Printf("  Build Time: %s\n", BuildTime)
		os.Exit(0)
	}

	log.Printf("🚀 Starting power-edge-server %s", Version)
	log.Printf("   Redis:         %s (DB %d)", *redisAddr, *redisDB)
	log.Printf("   Listen:        %s", *listenAddr)
	log.Printf("   Schema:        %s", *schemaVersion)

	// Initialize Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:     *redisAddr,
		Password: *redisPassword,
		DB:       *redisDB,
	})

	// Test Redis connection
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("❌ Failed to connect to Redis: %v", err)
	}
	log.Println("✅ Connected to Redis")

	// Create server instance
	server := &Server{
		redis:   rdb,
		version: *schemaVersion,
	}

	// Setup HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/version", versionHandler)
	mux.HandleFunc("/api/v1/nodes", server.listNodesHandler)
	mux.HandleFunc("/api/v1/nodes/", server.nodeHandler) // Note: trailing slash for node-specific routes

	// Start HTTP server
	httpServer := &http.Server{
		Addr:         *listenAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("📊 HTTP server listening on %s", *listenAddr)
		log.Println("   API Endpoints:")
		log.Println("     GET  /health              - Health check")
		log.Println("     GET  /version             - Version info")
		log.Println("     GET  /api/v1/nodes        - List all nodes")
		log.Println("     GET  /api/v1/nodes/{id}   - Get node state")
		log.Println("     PUT  /api/v1/nodes/{id}   - Update node state")
		log.Println("     GET  /api/v1/nodes/{id}/versions - Get system versions")
		log.Println("     GET  /api/v1/nodes/{id}/compliance - Get compliance status")

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("🛑 Shutting down gracefully...")

	// Shutdown HTTP server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	// Close Redis connection
	if err := rdb.Close(); err != nil {
		log.Printf("Redis close error: %v", err)
	}

	log.Println("✅ Shutdown complete")
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"healthy","version":"%s"}`, Version)
}

func versionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"version":"%s","git_commit":"%s","build_time":"%s"}`, Version, GitCommit, BuildTime)
}

// listNodesHandler returns list of all nodes in Redis
func (s *Server) listNodesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Scan for all node state keys
	pattern := fmt.Sprintf("%s:nodes:*:state", s.version)
	var nodes []string

	iter := s.redis.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		// Extract node ID from key: v1:nodes:{node-id}:state
		parts := strings.Split(key, ":")
		if len(parts) >= 3 {
			nodeID := parts[2]
			nodes = append(nodes, nodeID)
		}
	}

	if err := iter.Err(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to scan nodes: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes": nodes,
		"count": len(nodes),
	})
}

// nodeHandler handles node-specific routes
func (s *Server) nodeHandler(w http.ResponseWriter, r *http.Request) {
	// Extract node ID from path: /api/v1/nodes/{id} or /api/v1/nodes/{id}/subresource
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/nodes/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Node ID required", http.StatusBadRequest)
		return
	}

	nodeID := parts[0]
	subresource := ""
	if len(parts) > 1 {
		subresource = parts[1]
	}

	ctx := r.Context()

	// Route to appropriate handler
	switch subresource {
	case "versions":
		s.getNodeVersions(ctx, w, r, nodeID)
	case "compliance":
		s.getNodeCompliance(ctx, w, r, nodeID)
	case "":
		// Node state CRUD
		switch r.Method {
		case http.MethodGet:
			s.getNodeState(ctx, w, r, nodeID)
		case http.MethodPut:
			s.putNodeState(ctx, w, r, nodeID)
		case http.MethodDelete:
			s.deleteNodeState(ctx, w, r, nodeID)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	default:
		http.Error(w, "Unknown subresource", http.StatusNotFound)
	}
}

// getNodeState retrieves node state from Redis
func (s *Server) getNodeState(ctx context.Context, w http.ResponseWriter, r *http.Request, nodeID string) {
	key := s.NodeStateKey(nodeID)

	data, err := s.redis.Get(ctx, key).Bytes()
	if err == redis.Nil {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get state: %v", err), http.StatusInternalServerError)
		return
	}

	// Return YAML data
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Write(data)
}

// putNodeState updates node state in Redis
func (s *Server) putNodeState(ctx context.Context, w http.ResponseWriter, r *http.Request, nodeID string) {
	// Read request body (should be YAML)
	var state config.State
	if err := yaml.NewDecoder(r.Body).Decode(&state); err != nil {
		http.Error(w, fmt.Sprintf("Invalid YAML: %v", err), http.StatusBadRequest)
		return
	}

	// Validate state (basic check)
	if state.Version == "" {
		http.Error(w, "State version required", http.StatusBadRequest)
		return
	}

	// Marshal to YAML for storage
	yamlData, err := yaml.Marshal(&state)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to marshal state: %v", err), http.StatusInternalServerError)
		return
	}

	// Store in Redis
	key := s.NodeStateKey(nodeID)
	if err := s.redis.Set(ctx, key, yamlData, 0).Err(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to store state: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Updated state for node: %s", nodeID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"node_id": nodeID,
	})
}

// deleteNodeState removes node state from Redis
func (s *Server) deleteNodeState(ctx context.Context, w http.ResponseWriter, r *http.Request, nodeID string) {
	key := s.NodeStateKey(nodeID)

	if err := s.redis.Del(ctx, key).Err(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete state: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("🗑️  Deleted state for node: %s", nodeID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"node_id": nodeID,
	})
}

// getNodeVersions retrieves system versions from Redis
func (s *Server) getNodeVersions(ctx context.Context, w http.ResponseWriter, r *http.Request, nodeID string) {
	key := s.NodeVersionsKey(nodeID)

	data, err := s.redis.Get(ctx, key).Bytes()
	if err == redis.Nil {
		http.Error(w, "Versions not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get versions: %v", err), http.StatusInternalServerError)
		return
	}

	// Return JSON data
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// getNodeCompliance retrieves compliance status from Redis
func (s *Server) getNodeCompliance(ctx context.Context, w http.ResponseWriter, r *http.Request, nodeID string) {
	key := s.NodeComplianceKey(nodeID)

	data, err := s.redis.Get(ctx, key).Bytes()
	if err == redis.Nil {
		http.Error(w, "Compliance status not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get compliance: %v", err), http.StatusInternalServerError)
		return
	}

	// Return JSON data
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
