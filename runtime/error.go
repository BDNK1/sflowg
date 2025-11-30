package runtime

// TaskError wraps task execution errors with metadata
// Allows plugins to return execution metadata alongside errors for:
// - Retry hints (retryable, retry_after)
// - Error categorization (type: transient, permanent, user_error)
// - Execution metrics (duration_ms, attempt)
// - Warnings without errors (Err = nil, Metadata with warnings)
type TaskError struct {
	Err      error          // The underlying error (can be nil for warnings-only)
	Metadata map[string]any // Execution metadata (warnings, retry hints, metrics, etc.)
}

// Error implements the error interface
func (e *TaskError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "task completed with metadata"
}

// Unwrap returns the underlying error for errors.Is and errors.As
func (e *TaskError) Unwrap() error {
	return e.Err
}

// NewTaskError creates a new task error with the given underlying error
func NewTaskError(err error) *TaskError {
	return &TaskError{
		Err:      err,
		Metadata: make(map[string]any),
	}
}

// WithMetadata adds metadata to the error
func (e *TaskError) WithMetadata(key string, value any) *TaskError {
	e.Metadata[key] = value
	return e
}

// WithMetadataMap adds multiple metadata entries
func (e *TaskError) WithMetadataMap(metadata map[string]any) *TaskError {
	for k, v := range metadata {
		e.Metadata[k] = v
	}
	return e
}

// WithRetryHint adds retry hint metadata
func (e *TaskError) WithRetryHint(retryable bool, retryAfter string) *TaskError {
	e.Metadata["retryable"] = retryable
	if retryAfter != "" {
		e.Metadata["retry_after"] = retryAfter
	}
	return e
}

// WithType sets the error type (e.g., "transient", "permanent", "user_error")
func (e *TaskError) WithType(errorType string) *TaskError {
	e.Metadata["type"] = errorType
	return e
}

// IsRetryable checks if the error is marked as retryable
func (e *TaskError) IsRetryable() bool {
	if val, ok := e.Metadata["retryable"]; ok {
		if retryable, ok := val.(bool); ok {
			return retryable
		}
	}
	return false
}

// GetRetryAfter returns the retry_after duration if set
func (e *TaskError) GetRetryAfter() string {
	if val, ok := e.Metadata["retry_after"]; ok {
		if retryAfter, ok := val.(string); ok {
			return retryAfter
		}
	}
	return ""
}

// GetType returns the error type if set
func (e *TaskError) GetType() string {
	if val, ok := e.Metadata["type"]; ok {
		if errorType, ok := val.(string); ok {
			return errorType
		}
	}
	return ""
}
