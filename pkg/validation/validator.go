package validation

import (
	"fmt"
	"strings"
	"time"

	"github.com/sumukha/edgelog-stream/pkg/models"
)

// Validator validates events
type Validator struct {
	maxPayloadSizeBytes int
}

// NewValidator creates a new validator
func NewValidator(maxPayloadSize int) *Validator {
	return &Validator{
		maxPayloadSizeBytes: maxPayloadSize,
	}
}

// ValidateEvent validates an event and returns validation errors
func (v *Validator) ValidateEvent(event *models.Event) []models.ValidationError {
	var errors []models.ValidationError

	// Required field validations
	if strings.TrimSpace(event.EventID) == "" {
		errors = append(errors, models.ValidationError{
			Field:   "event_id",
			Message: "event_id is required",
		})
	}

	if strings.TrimSpace(event.TenantID) == "" {
		errors = append(errors, models.ValidationError{
			Field:   "tenant_id",
			Message: "tenant_id is required",
		})
	}

	if strings.TrimSpace(event.EventType) == "" {
		errors = append(errors, models.ValidationError{
			Field:   "event_type",
			Message: "event_type is required",
		})
	}

	// Timestamp validation
	if event.Timestamp.IsZero() {
		errors = append(errors, models.ValidationError{
			Field:   "timestamp",
			Message: "timestamp is required",
		})
	} else {
		// Check if timestamp is not too far in the future (more than 1 hour)
		if event.Timestamp.After(time.Now().Add(1 * time.Hour)) {
			errors = append(errors, models.ValidationError{
				Field:   "timestamp",
				Message: "timestamp cannot be more than 1 hour in the future",
			})
		}

		// Check if timestamp is not too far in the past (more than 7 days)
		if event.Timestamp.Before(time.Now().Add(-7 * 24 * time.Hour)) {
			errors = append(errors, models.ValidationError{
				Field:   "timestamp",
				Message: "timestamp cannot be more than 7 days in the past",
			})
		}
	}

	// Data payload validation
	if event.Data == nil {
		errors = append(errors, models.ValidationError{
			Field:   "data",
			Message: "data payload is required",
		})
	}

	// Geographic validation (if provided)
	if event.Metadata.Geo != nil {
		if event.Metadata.Geo.Lat < -90 || event.Metadata.Geo.Lat > 90 {
			errors = append(errors, models.ValidationError{
				Field:   "metadata.geo.lat",
				Message: "latitude must be between -90 and 90",
			})
		}
		if event.Metadata.Geo.Lon < -180 || event.Metadata.Geo.Lon > 180 {
			errors = append(errors, models.ValidationError{
				Field:   "metadata.geo.lon",
				Message: "longitude must be between -180 and 180",
			})
		}
	}

	return errors
}

// FormatErrors formats validation errors into a readable string
func FormatErrors(errors []models.ValidationError) string {
	if len(errors) == 0 {
		return ""
	}

	var sb strings.Builder
	for i, err := range errors {
		if i > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(fmt.Sprintf("%s: %s", err.Field, err.Message))
	}
	return sb.String()
}
