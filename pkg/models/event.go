package models

import "time"

// Event represents a generic event ingested by EdgeLog Stream
type Event struct {
	// Identity
	EventID         string `json:"event_id"`
	IdempotencyKey  string `json:"idempotency_key,omitempty"`

	// Temporal
	Timestamp      time.Time `json:"timestamp"`
	IngestionTime  time.Time `json:"ingestion_time,omitempty"`

	// Classification
	TenantID       string `json:"tenant_id"`
	EventType      string `json:"event_type"`

	// Source Information
	Source         Source `json:"source,omitempty"`

	// Payload - flexible JSON data
	Data           map[string]interface{} `json:"data"`

	// Metadata
	Metadata       Metadata `json:"metadata,omitempty"`

	// Processing Metrics
	ProcessingTimeMs uint16 `json:"processing_time_ms,omitempty"`
}

// Source contains information about the event source
type Source struct {
	Service    string `json:"service,omitempty"`
	Version    string `json:"version,omitempty"`
	Host       string `json:"host,omitempty"`
	Datacenter string `json:"datacenter,omitempty"`
}

// Metadata contains additional contextual information
type Metadata struct {
	CorrelationID string       `json:"correlation_id,omitempty"`
	UserID        string       `json:"user_id,omitempty"`
	SessionID     string       `json:"session_id,omitempty"`
	Geo           *GeoLocation `json:"geo,omitempty"`
}

// GeoLocation represents geographic coordinates and location info
type GeoLocation struct {
	Lat        float32 `json:"lat"`
	Lon        float32 `json:"lon"`
	Country    string  `json:"country,omitempty"`
	Region     string  `json:"region,omitempty"`
	City       string  `json:"city,omitempty"`
	PostalCode string  `json:"postal_code,omitempty"`
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// IngestResponse is the response returned after event ingestion
type IngestResponse struct {
	Status    string `json:"status"`
	EventID   string `json:"event_id"`
	Partition int32  `json:"partition,omitempty"`
	Offset    int64  `json:"offset,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error      string             `json:"error"`
	Message    string             `json:"message,omitempty"`
	Details    []ValidationError  `json:"details,omitempty"`
	RetryAfter int                `json:"retry_after,omitempty"`
}
