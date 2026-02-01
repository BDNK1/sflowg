package checkout

import (
	"fmt"
	"html/template"

	"github.com/sflowg/sflowg/runtime/plugin"
)

// Config holds checkout plugin configuration
type Config struct {
	StripePublishableKey string `yaml:"stripe_publishable_key" validate:"required"`
	ReturnURL            string `yaml:"return_url" default:"http://localhost:8080/payment-success"`
}

// RenderInput defines input for checkout.render task
type RenderInput struct {
	ClientSecret string `json:"client_secret" validate:"required"`
	PaymentID    int64  `json:"payment_id" validate:"required"`
	Amount       int64  `json:"amount" validate:"required"`
	Currency     string `json:"currency" validate:"required"`
}

// RenderOutput defines output for checkout.render task
type RenderOutput struct {
	HTML string `json:"html"`
}

// CheckoutPlugin provides Stripe checkout page rendering
type CheckoutPlugin struct {
	Config Config
}

// Initialize is called when the plugin is loaded
func (p *CheckoutPlugin) Initialize(exec *plugin.Execution) error {
	if p.Config.StripePublishableKey == "" {
		return fmt.Errorf("checkout: stripe_publishable_key is required")
	}
	return nil
}

// Shutdown is called when the plugin is unloaded
func (p *CheckoutPlugin) Shutdown(exec *plugin.Execution) error {
	return nil
}

// Render generates an HTML checkout page with Stripe Elements
func (p *CheckoutPlugin) Render(exec *plugin.Execution, input RenderInput) (RenderOutput, error) {
	tmpl := template.Must(template.New("checkout").Parse(checkoutTemplate))

	data := struct {
		StripePublishableKey string
		ClientSecret         string
		PaymentID            int64
		Amount               float64
		Currency             string
		ReturnURL            string
	}{
		StripePublishableKey: p.Config.StripePublishableKey,
		ClientSecret:         input.ClientSecret,
		PaymentID:            input.PaymentID,
		Amount:               float64(input.Amount) / 100.0, // Convert cents to dollars
		Currency:             input.Currency,
		ReturnURL:            p.Config.ReturnURL,
	}

	var buf []byte
	writer := &bytesWriter{buf: buf}
	if err := tmpl.Execute(writer, data); err != nil {
		return RenderOutput{}, fmt.Errorf("checkout: failed to render template: %w", err)
	}

	return RenderOutput{HTML: string(writer.buf)}, nil
}

// bytesWriter implements io.Writer for []byte
type bytesWriter struct {
	buf []byte
}

func (w *bytesWriter) Write(p []byte) (n int, err error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}

const checkoutTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Checkout - Payment #{{.PaymentID}}</title>
    <script src="https://js.stripe.com/v3/"></script>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 20px;
        }
        .container {
            background: white;
            border-radius: 12px;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
            max-width: 500px;
            width: 100%;
            padding: 40px;
        }
        h1 {
            color: #1a202c;
            font-size: 28px;
            margin-bottom: 8px;
        }
        .amount {
            color: #667eea;
            font-size: 36px;
            font-weight: bold;
            margin: 16px 0 24px;
        }
        .payment-info {
            background: #f7fafc;
            border-radius: 8px;
            padding: 16px;
            margin-bottom: 24px;
        }
        .payment-info p {
            color: #4a5568;
            margin: 8px 0;
            font-size: 14px;
        }
        .payment-info strong {
            color: #2d3748;
        }
        #payment-form {
            margin-top: 24px;
        }
        #payment-element {
            margin-bottom: 24px;
        }
        #submit-button {
            width: 100%;
            background: #667eea;
            color: white;
            border: none;
            border-radius: 6px;
            padding: 14px;
            font-size: 16px;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.3s;
        }
        #submit-button:hover:not(:disabled) {
            background: #5568d3;
            transform: translateY(-1px);
            box-shadow: 0 4px 12px rgba(102, 126, 234, 0.4);
        }
        #submit-button:disabled {
            opacity: 0.6;
            cursor: not-allowed;
        }
        #error-message {
            color: #e53e3e;
            margin-top: 16px;
            font-size: 14px;
            display: none;
        }
        #error-message.show {
            display: block;
        }
        .spinner {
            display: none;
            width: 20px;
            height: 20px;
            border: 3px solid #ffffff;
            border-top-color: transparent;
            border-radius: 50%;
            animation: spin 0.8s linear infinite;
            margin: 0 auto;
        }
        @keyframes spin {
            to { transform: rotate(360deg); }
        }
        .loading #submit-button span { display: none; }
        .loading .spinner { display: block; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Complete Payment</h1>
        <div class="amount">{{.Currency}} ${{printf "%.2f" .Amount}}</div>

        <div class="payment-info">
            <p><strong>Payment ID:</strong> #{{.PaymentID}}</p>
            <p><strong>Status:</strong> Awaiting payment</p>
        </div>

        <form id="payment-form">
            <div id="payment-element"></div>
            <button type="submit" id="submit-button">
                <span>Pay Now</span>
                <div class="spinner"></div>
            </button>
            <div id="error-message"></div>
        </form>
    </div>

    <script>
        const stripe = Stripe('{{.StripePublishableKey}}');

        const options = {
            clientSecret: '{{.ClientSecret}}',
            appearance: {
                theme: 'stripe',
                variables: {
                    colorPrimary: '#667eea',
                    borderRadius: '6px',
                }
            }
        };

        const elements = stripe.elements(options);
        const paymentElement = elements.create('payment');
        paymentElement.mount('#payment-element');

        const form = document.getElementById('payment-form');
        const submitButton = document.getElementById('submit-button');
        const errorMessage = document.getElementById('error-message');

        form.addEventListener('submit', async (e) => {
            e.preventDefault();

            // Disable button and show loading
            submitButton.disabled = true;
            form.classList.add('loading');
            errorMessage.classList.remove('show');

            const {error} = await stripe.confirmPayment({
                elements,
                confirmParams: {
                    return_url: '{{.ReturnURL}}?payment_id={{.PaymentID}}',
                },
            });

            // This point will only be reached if there's an immediate error
            if (error) {
                errorMessage.textContent = error.message;
                errorMessage.classList.add('show');
                submitButton.disabled = false;
                form.classList.remove('loading');
            }
        });
    </script>
</body>
</html>`
