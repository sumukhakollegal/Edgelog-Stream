package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sumukha/edgelog-stream/pkg/models"
	"github.com/sumukha/edgelog-stream/pkg/validation"
)

const (
	defaultPort            = "8080"
	defaultMaxPayloadBytes = 1048576 // 1MB
	shutdownTimeout        = 30 * time.Second
)

type Ingestor struct {
	validator *validation.Validator
	server    *http.Server
}

func main() {
	log.Println("Starting EdgeLog Stream Ingestor...")

	// Initialize validator
	validator := validation.NewValidator(defaultMaxPayloadBytes)

	// Initialize ingestor
	ingestor := &Ingestor{
		validator: validator,
	}

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/health", ingestor.handleHealth)
	mux.HandleFunc("/v1/events", ingestor.handleIngestEvent)

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	ingestor.server = &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Ingestor listening on port %s", port)
		if err := ingestor.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := ingestor.server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

// handleHealth handles health check requests
func (i *Ingestor) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := map[string]interface{}{
		"status":  "healthy",
		"service": "ingestor",
		"version": "0.1.0",
		"time":    time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleIngestEvent handles event ingestion requests
func (i *Ingestor) handleIngestEvent(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// Only accept POST requests
	if r.Method != http.MethodPost {
		i.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST method is allowed")
		return
	}

	// Check Content-Type
	if r.Header.Get("Content-Type") != "application/json" {
		i.writeError(w, http.StatusBadRequest, "invalid_content_type", "Content-Type must be application/json")
		return
	}

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, int64(defaultMaxPayloadBytes))

	// Parse event
	var event models.Event
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		i.writeError(w, http.StatusBadRequest, "invalid_json", fmt.Sprintf("Failed to parse JSON: %v", err))
		return
	}

	// Set ingestion time
	event.IngestionTime = time.Now().UTC()

	// Validate event
	validationErrors := i.validator.ValidateEvent(&event)
	if len(validationErrors) > 0 {
		i.writeValidationError(w, validationErrors)
		return
	}

	// For Phase 1: Log event to stdout (no Kafka yet)
	// In Phase 2, this will be sent to Kafka
	eventJSON, err := json.Marshal(event)
	if err != nil {
		i.writeError(w, http.StatusInternalServerError, "internal_error", "Failed to serialize event")
		return
	}

	log.Printf("EVENT_RECEIVED: tenant=%s type=%s id=%s", event.TenantID, event.EventType, event.EventID)
	log.Printf("EVENT_DATA: %s", string(eventJSON))

	// Calculate processing time
	processingTime := time.Since(startTime)
	event.ProcessingTimeMs = uint16(processingTime.Milliseconds())

	// Return success response
	response := models.IngestResponse{
		Status:  "accepted",
		EventID: event.EventID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(response)

	log.Printf("EVENT_PROCESSED: id=%s processing_time=%dms", event.EventID, event.ProcessingTimeMs)
}

// writeError writes a JSON error response
func (i *Ingestor) writeError(w http.ResponseWriter, statusCode int, errorType, message string) {
	response := models.ErrorResponse{
		Error:   errorType,
		Message: message,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// writeValidationError writes validation errors
func (i *Ingestor) writeValidationError(w http.ResponseWriter, errors []models.ValidationError) {
	response := models.ErrorResponse{
		Error:   "validation_failed",
		Message: "Event validation failed",
		Details: errors,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(response)
}
