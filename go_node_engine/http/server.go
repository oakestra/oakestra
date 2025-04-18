package http

import (
	"context"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"net/http"
	"time"
)

var server *http.Server

// StartServer initializes and starts the HTTP server on the specified port
func startServer(port string) error {
	mux := http.NewServeMux()

	// Register the /id route
	mux.HandleFunc("/id", handleIDRequest)

	// Create the server
	server = &http.Server{
		Addr:    fmt.Sprintf(":%s", port),
		Handler: mux,
	}

	// Start the server
	logger.InfoLogger().Printf("Starting HTTP server on port %s", port)
	return server.ListenAndServe()
}

// StartServerNonBlocking initializes and starts the HTTP server in a goroutine
func StartServerNonBlocking(port string) {
	go func() {
		if err := startServer(port); err != nil && err != http.ErrServerClosed {
			logger.ErrorLogger().Printf("HTTP server error: %v", err)
		}
	}()
	logger.InfoLogger().Printf("HTTP server started in background on port %s", port)
}

// StopServer gracefully shuts down the HTTP server
func StopServer() error {
	if server != nil {
		logger.InfoLogger().Println("Shutting down HTTP server")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	}
	return nil
}

// handleIDRequest handles the /id route and returns the node's ID
func handleIDRequest(w http.ResponseWriter, r *http.Request) {
	nodeID := model.GetNodeInfo().Id
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, nodeID)
}
