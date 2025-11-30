// Package payment provides payment processing functionality
// This plugin demonstrates all plugin system features:
// - Multiple tasks per plugin
// - Typed and untyped tasks
// - Plugin configuration with defaults and validation
// - Lifecycle management (Initialize/Shutdown)
// - Plugin dependencies (HTTP plugin injection)
// - Shared state across tasks
package payment

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sflowg/sflowg/runtime"
)

// Config defines payment plugin configuration with declarative tags
type Config struct {
	// Provider settings
	ProviderName string `yaml:"provider_name" default:"StripeProvider" validate:"required"`
	APIBaseURL   string `yaml:"api_base_url" default:"https://api.stripe.com/v1" validate:"required,url"`
	APIKey       string `yaml:"api_key" validate:"required"`

	// Processing settings
	Timeout         time.Duration `yaml:"timeout" default:"30s" validate:"gte=1s,lte=2m"`
	MaxRetries      int           `yaml:"max_retries" default:"3" validate:"gte=0,lte=10"`
	MinAmount       float64       `yaml:"min_amount" default:"0.01" validate:"gte=0"`
	MaxAmount       float64       `yaml:"max_amount" default:"999999.99" validate:"gte=0"`
	DefaultCurrency string        `yaml:"default_currency" default:"USD" validate:"required,oneof=USD EUR GBP"`

	// Feature flags
	EnableRefunds    bool `yaml:"enable_refunds" default:"true"`
	EnableValidation bool `yaml:"enable_validation" default:"true"`
	DebugMode        bool `yaml:"debug_mode" default:"false"`
}

// PaymentPlugin implements payment processing operations
// Tests:
// - Multiple tasks (ValidateCard, ProcessPayment, RefundPayment, GetStatus)
// - Shared state (config, statistics)
// - Lifecycle (Initialize/Shutdown)
// - Plugin dependencies (will have HTTP plugin injected in Phase 2)
type PaymentPlugin struct {
	Config Config // Exported so CLI can set it during initialization

	// Shared state across all tasks
	initialized        bool
	totalTransactions  int
	successfulPayments int
	failedPayments     int

	// In Phase 2, this will be injected automatically by the framework
	// For Phase 1, plugins can access via exec.Container.GetPlugin("http")
	// http *HTTPPlugin
}

// Initialize implements runtime.Initializer interface
// This is called once when the container starts up
func (p *PaymentPlugin) Initialize(exec *runtime.Execution) error {
	log.Printf("[Payment Plugin] Initializing with provider: %s", p.Config.ProviderName)
	log.Printf("[Payment Plugin] API Base URL: %s", p.Config.APIBaseURL)
	log.Printf("[Payment Plugin] Debug Mode: %v", p.Config.DebugMode)

	// Simulate connection setup
	if p.Config.DebugMode {
		log.Printf("[Payment Plugin] Debug mode enabled - additional logging active")
	}

	// Initialize counters
	p.totalTransactions = 0
	p.successfulPayments = 0
	p.failedPayments = 0

	p.initialized = true
	log.Printf("[Payment Plugin] ✓ Initialization complete")
	return nil
}

// Shutdown implements runtime.Shutdowner interface
// This is called during graceful shutdown
func (p *PaymentPlugin) Shutdown(exec *runtime.Execution) error {
	log.Printf("[Payment Plugin] Shutting down...")
	log.Printf("[Payment Plugin] Statistics:")
	log.Printf("[Payment Plugin]   Total Transactions: %d", p.totalTransactions)
	log.Printf("[Payment Plugin]   Successful: %d", p.successfulPayments)
	log.Printf("[Payment Plugin]   Failed: %d", p.failedPayments)

	p.initialized = false
	log.Printf("[Payment Plugin] ✓ Shutdown complete")
	return nil
}

// --- Task 1: ValidateCard (Typed Task) ---

// ValidateCardInput defines typed input for card validation
type ValidateCardInput struct {
	CardNumber string `json:"card_number" validate:"required"`
	ExpiryDate string `json:"expiry_date" validate:"required"` // Format: MM/YY
	CVV        string `json:"cvv" validate:"required,len=3"`
}

// ValidateCardOutput defines typed output for card validation
type ValidateCardOutput struct {
	Valid        bool     `json:"valid"`
	CardType     string   `json:"card_type"` // visa, mastercard, amex, etc.
	LastFourDigs string   `json:"last_four_digits"`
	ExpiryValid  bool     `json:"expiry_valid"`
	Errors       []string `json:"errors,omitempty"`
}

// ValidateCard validates credit card information
// This is a TYPED task demonstrating struct-based input/output
func (p *PaymentPlugin) ValidateCard(exec *runtime.Execution, input ValidateCardInput) (ValidateCardOutput, error) {
	if !p.Config.EnableValidation {
		return ValidateCardOutput{
			Valid:  true,
			Errors: []string{"validation disabled"},
		}, nil
	}

	output := ValidateCardOutput{
		Valid:       true,
		ExpiryValid: true,
		Errors:      []string{},
	}

	// Validate card number format
	cardNumber := strings.ReplaceAll(input.CardNumber, " ", "")
	if len(cardNumber) < 13 || len(cardNumber) > 19 {
		output.Valid = false
		output.Errors = append(output.Errors, "invalid card number length")
	} else {
		// Extract last 4 digits
		output.LastFourDigs = cardNumber[len(cardNumber)-4:]

		// Detect card type
		output.CardType = detectCardType(cardNumber)

		// Basic Luhn algorithm validation
		if !luhnCheck(cardNumber) {
			output.Valid = false
			output.Errors = append(output.Errors, "card number failed Luhn check")
		}
	}

	// Validate expiry date
	if !validateExpiryDate(input.ExpiryDate) {
		output.Valid = false
		output.ExpiryValid = false
		output.Errors = append(output.Errors, "card is expired or invalid expiry format")
	}

	// Validate CVV
	if len(input.CVV) != 3 && len(input.CVV) != 4 {
		output.Valid = false
		output.Errors = append(output.Errors, "invalid CVV length")
	}

	if p.Config.DebugMode {
		log.Printf("[ValidateCard] Card Type: %s, Valid: %v", output.CardType, output.Valid)
	}

	return output, nil
}

// --- Task 2: ProcessPayment (Typed Task) ---

// ProcessPaymentInput defines typed input for payment processing
type ProcessPaymentInput struct {
	Amount        float64 `json:"amount" validate:"required,gt=0"`
	Currency      string  `json:"currency" validate:"required,len=3"`
	CardNumber    string  `json:"card_number" validate:"required"`
	ExpiryDate    string  `json:"expiry_date" validate:"required"`
	CVV           string  `json:"cvv" validate:"required"`
	Description   string  `json:"description"`
	CustomerEmail string  `json:"customer_email" validate:"omitempty,email"`
}

// ProcessPaymentOutput defines typed output for payment processing
type ProcessPaymentOutput struct {
	Success           bool    `json:"success"`
	TransactionID     string  `json:"transaction_id"`
	Amount            float64 `json:"amount"`
	Currency          string  `json:"currency"`
	Status            string  `json:"status"` // pending, completed, failed
	Message           string  `json:"message"`
	ProcessedAt       string  `json:"processed_at"`
	AuthorizationCode string  `json:"authorization_code,omitempty"`
}

// ProcessPayment processes a payment transaction
// This is a TYPED task with complex validation logic
func (p *PaymentPlugin) ProcessPayment(exec *runtime.Execution, input ProcessPaymentInput) (ProcessPaymentOutput, error) {
	p.totalTransactions++

	// Validate amount range
	if input.Amount < p.Config.MinAmount {
		p.failedPayments++
		return ProcessPaymentOutput{
			Success:  false,
			Amount:   input.Amount,
			Currency: input.Currency,
			Status:   "failed",
			Message:  fmt.Sprintf("amount below minimum: %v", p.Config.MinAmount),
		}, fmt.Errorf("amount below minimum threshold")
	}

	if input.Amount > p.Config.MaxAmount {
		p.failedPayments++
		return ProcessPaymentOutput{
			Success:  false,
			Amount:   input.Amount,
			Currency: input.Currency,
			Status:   "failed",
			Message:  fmt.Sprintf("amount exceeds maximum: %v", p.Config.MaxAmount),
		}, fmt.Errorf("amount exceeds maximum threshold")
	}

	// Generate transaction ID
	transactionID := fmt.Sprintf("txn_%d_%d", time.Now().Unix(), p.totalTransactions)

	// Simulate payment processing
	// In real implementation, this would call the payment provider API
	authCode := fmt.Sprintf("AUTH_%d", time.Now().Unix())

	p.successfulPayments++

	if p.Config.DebugMode {
		log.Printf("[ProcessPayment] Transaction: %s, Amount: %.2f %s", transactionID, input.Amount, input.Currency)
	}

	return ProcessPaymentOutput{
		Success:           true,
		TransactionID:     transactionID,
		Amount:            input.Amount,
		Currency:          input.Currency,
		Status:            "completed",
		Message:           "payment processed successfully",
		ProcessedAt:       time.Now().Format(time.RFC3339),
		AuthorizationCode: authCode,
	}, nil
}

// --- Task 3: RefundPayment (Untyped Task) ---

// RefundPayment processes a refund for a previous transaction
// This is an UNTYPED task demonstrating map-based input/output
// Tests compatibility between typed and untyped tasks in same plugin
func (p *PaymentPlugin) RefundPayment(exec *runtime.Execution, input map[string]any) (map[string]any, error) {
	if !p.Config.EnableRefunds {
		return map[string]any{
			"success": false,
			"message": "refunds are disabled",
		}, fmt.Errorf("refunds disabled")
	}

	// Extract and validate input
	transactionID, ok := input["transaction_id"].(string)
	if !ok || transactionID == "" {
		return map[string]any{
			"success": false,
			"message": "transaction_id is required",
		}, fmt.Errorf("missing transaction_id")
	}

	// Amount is optional - if not provided, refund full amount
	refundAmount := 0.0
	if amt, ok := input["amount"].(float64); ok {
		refundAmount = amt
	}

	reason, _ := input["reason"].(string)

	// Generate refund ID
	refundID := fmt.Sprintf("refund_%s_%d", transactionID, time.Now().Unix())

	if p.Config.DebugMode {
		log.Printf("[RefundPayment] Refund ID: %s, Transaction: %s, Amount: %.2f",
			refundID, transactionID, refundAmount)
	}

	// Simulate refund processing
	return map[string]any{
		"success":        true,
		"refund_id":      refundID,
		"transaction_id": transactionID,
		"amount":         refundAmount,
		"status":         "refunded",
		"reason":         reason,
		"refunded_at":    time.Now().Format(time.RFC3339),
	}, nil
}

// --- Task 4: GetStatus (Untyped Task) ---

// GetStatus returns the current plugin statistics
// This is an UNTYPED task that accesses shared plugin state
func (p *PaymentPlugin) GetStatus(exec *runtime.Execution, input map[string]any) (map[string]any, error) {
	successRate := 0.0
	if p.totalTransactions > 0 {
		successRate = float64(p.successfulPayments) / float64(p.totalTransactions) * 100
	}

	return map[string]any{
		"initialized":        p.initialized,
		"provider":           p.Config.ProviderName,
		"total_transactions": p.totalTransactions,
		"successful":         p.successfulPayments,
		"failed":             p.failedPayments,
		"success_rate":       fmt.Sprintf("%.2f%%", successRate),
		"refunds_enabled":    p.Config.EnableRefunds,
		"validation_enabled": p.Config.EnableValidation,
	}, nil
}

// --- Helper Functions ---

// detectCardType detects the card type based on number prefix
func detectCardType(cardNumber string) string {
	// Remove spaces
	cardNumber = strings.ReplaceAll(cardNumber, " ", "")

	// Visa: starts with 4
	if strings.HasPrefix(cardNumber, "4") {
		return "visa"
	}

	// Mastercard: starts with 51-55 or 2221-2720
	if matched, _ := regexp.MatchString("^5[1-5]", cardNumber); matched {
		return "mastercard"
	}
	if matched, _ := regexp.MatchString("^2[2-7]", cardNumber); matched {
		return "mastercard"
	}

	// American Express: starts with 34 or 37
	if strings.HasPrefix(cardNumber, "34") || strings.HasPrefix(cardNumber, "37") {
		return "amex"
	}

	// Discover: starts with 6011, 622126-622925, 644-649, or 65
	if strings.HasPrefix(cardNumber, "6011") || strings.HasPrefix(cardNumber, "65") {
		return "discover"
	}

	return "unknown"
}

// luhnCheck validates card number using Luhn algorithm
func luhnCheck(cardNumber string) bool {
	var sum int
	var alternate bool

	// Process from right to left
	for i := len(cardNumber) - 1; i >= 0; i-- {
		digit, err := strconv.Atoi(string(cardNumber[i]))
		if err != nil {
			return false
		}

		if alternate {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}

		sum += digit
		alternate = !alternate
	}

	return sum%10 == 0
}

// validateExpiryDate checks if the expiry date is valid and not expired
func validateExpiryDate(expiry string) bool {
	// Expected format: MM/YY
	parts := strings.Split(expiry, "/")
	if len(parts) != 2 {
		return false
	}

	month, err := strconv.Atoi(parts[0])
	if err != nil || month < 1 || month > 12 {
		return false
	}

	year, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}

	// Convert YY to full year (assumes 20YY)
	if year < 100 {
		year += 2000
	}

	// Check if card is expired
	now := time.Now()
	expiryDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)

	// Card expires at the end of the month
	expiryDate = expiryDate.AddDate(0, 1, 0).Add(-time.Second)

	return expiryDate.After(now)
}
