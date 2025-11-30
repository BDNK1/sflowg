// Copyright 2024 SFlowG Examples
// Licensed under Apache 2.0

/*
Package uuidgen provides UUID generation functionality.

This is a local module plugin demonstrating:
- Minimal plugin structure
- Lifecycle management
- Multiple task variants
- Build tags preservation
*/
package uuidgen

import (
	"crypto/rand"
	"fmt"
	"log"

	"github.com/sflowg/sflowg/runtime"
)

// UUIDGenPlugin provides UUID generation operations
// This plugin demonstrates:
// - Simple plugin with no configuration
// - Multiple tasks (Generate, GenerateV4, Validate)
// - Lifecycle hooks
type UUIDGenPlugin struct {
	totalGenerated int
	initialized    bool
}

// Initialize implements runtime.Initializer interface
func (p *UUIDGenPlugin) Initialize(exec *runtime.Execution) error {
	log.Printf("[UUID Plugin] Initializing...")
	p.totalGenerated = 0
	p.initialized = true
	log.Printf("[UUID Plugin] ✓ Initialization complete")
	return nil
}

// Shutdown implements runtime.Shutdowner interface
func (p *UUIDGenPlugin) Shutdown(exec *runtime.Execution) error {
	log.Printf("[UUID Plugin] Shutting down...")
	log.Printf("[UUID Plugin] Total UUIDs generated: %d", p.totalGenerated)
	p.initialized = false
	log.Printf("[UUID Plugin] ✓ Shutdown complete")
	return nil
}

// --- Task 1: GenerateUUID (Typed Task) ---

// GenerateUUIDOutput defines the typed output for UUID generation
type GenerateUUIDOutput struct {
	UUID    string `json:"uuid"`
	Version int    `json:"version"`
}

// GenerateUUID generates a new UUID v4 (random)
// This is a TYPED task with no input required
func (p *UUIDGenPlugin) GenerateUUID(exec *runtime.Execution, input map[string]any) (GenerateUUIDOutput, error) {
	uuid, err := generateUUIDv4()
	if err != nil {
		return GenerateUUIDOutput{}, fmt.Errorf("failed to generate UUID: %w", err)
	}

	p.totalGenerated++

	return GenerateUUIDOutput{
		UUID:    uuid,
		Version: 4,
	}, nil
}

// --- Task 2: GenerateV4 (Alias) ---

// GenerateV4 generates a UUID v4 (random) - alias for GenerateUUID
func (p *UUIDGenPlugin) GenerateV4(exec *runtime.Execution, input map[string]any) (GenerateUUIDOutput, error) {
	return p.GenerateUUID(exec, input)
}

// --- Task 3: ValidateUUID (Untyped Task) ---

// ValidateUUID checks if a string is a valid UUID format
// This is an UNTYPED task demonstrating map-based input/output
func (p *UUIDGenPlugin) ValidateUUID(exec *runtime.Execution, input map[string]any) (map[string]any, error) {
	uuidStr, ok := input["uuid"].(string)
	if !ok || uuidStr == "" {
		return map[string]any{
			"valid": false,
			"error": "uuid parameter is required",
		}, nil
	}

	valid := isValidUUID(uuidStr)

	return map[string]any{
		"valid": valid,
		"uuid":  uuidStr,
	}, nil
}

// --- Helper Functions ---

// generateUUIDv4 generates a UUID v4 using crypto/rand
func generateUUIDv4() (string, error) {
	// UUID v4 is 16 bytes (128 bits)
	uuid := make([]byte, 16)

	// Generate random bytes
	if _, err := rand.Read(uuid); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Set version (4) and variant (RFC 4122)
	// Version 4: bits 12-15 of time_hi_and_version field = 0100
	uuid[6] = (uuid[6] & 0x0f) | 0x40

	// Variant: bits 6-7 of clock_seq_hi_and_reserved field = 10
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	// Format as string: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4],
		uuid[4:6],
		uuid[6:8],
		uuid[8:10],
		uuid[10:16],
	), nil
}

// isValidUUID checks if a string matches UUID format
func isValidUUID(s string) bool {
	// Basic format check: 8-4-4-4-12
	if len(s) != 36 {
		return false
	}

	// Check hyphens at correct positions
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}

	// Check all other characters are valid hex
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !isHexChar(c) {
			return false
		}
	}

	return true
}

// isHexChar checks if a character is a valid hexadecimal digit
func isHexChar(c rune) bool {
	return (c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'f') ||
		(c >= 'A' && c <= 'F')
}
