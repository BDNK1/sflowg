//go:build !test
// +build !test

// Copyright 2024 SFlowG Examples
// Licensed under Apache 2.0

/*
Package uuidgen provides UUID generation functionality.

This is a local module plugin demonstrating:
- Build tags preservation
- Multi-line package comments
- Copyright headers
- Standard library imports with runtime dependency
*/
package uuidgen

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/sflowg/sflowg/runtime"
)

// UUIDGenPlugin provides UUID generation operations
type UUIDGenPlugin struct {
	config map[string]interface{}
}

// NewUuidgenPlugin creates a new UUID generator plugin instance
func NewUuidgenPlugin() runtime.Plugin {
	return &UUIDGenPlugin{}
}

// Name returns the plugin identifier
func (p *UUIDGenPlugin) Name() string {
	return "uuidgen"
}

// Initialize sets up the plugin with configuration
func (p *UUIDGenPlugin) Initialize(ctx context.Context, config map[string]interface{}) error {
	p.config = config
	return nil
}

// Shutdown cleans up plugin resources
func (p *UUIDGenPlugin) Shutdown(ctx context.Context) error {
	return nil
}

// GenerateUUID generates a new UUID v4 (random)
func (p *UUIDGenPlugin) GenerateUUID(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	uuid, err := generateUUIDv4()
	if err != nil {
		return nil, fmt.Errorf("failed to generate UUID: %w", err)
	}

	return map[string]interface{}{
		"uuid":    uuid,
		"version": 4,
	}, nil
}

// GenerateV4 generates a UUID v4 (random) - alias for Generate
func (p *UUIDGenPlugin) GenerateV4(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	return p.GenerateUUID(ctx, input)
}

// ValidateUUID checks if a string is a valid UUID format
func (p *UUIDGenPlugin) ValidateUUID(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	uuidStr, ok := input["uuid"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid uuid parameter")
	}

	valid := isValidUUID(uuidStr)

	return map[string]interface{}{
		"valid": valid,
	}, nil
}

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
