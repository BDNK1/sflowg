-- Stripe Integration Schema
-- Run this before starting the application

-- Create the payments table
CREATE TABLE IF NOT EXISTS payments (
    id SERIAL PRIMARY KEY,

    -- Payment details (from request)
    amount INTEGER NOT NULL,              -- Amount in cents
    currency VARCHAR(3) NOT NULL DEFAULT 'usd',
    description TEXT,
    customer_email VARCHAR(255),
    customer_name VARCHAR(255),
    metadata_order_id VARCHAR(255),

    -- Stripe data (from API response)
    stripe_payment_intent_id VARCHAR(255) UNIQUE,
    stripe_client_secret TEXT,
    stripe_status VARCHAR(50),            -- Stripe's payment intent status

    -- Our internal status
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    -- Possible values: pending, requires_payment_method, succeeded, failed, canceled

    -- Error handling
    error_message TEXT,
    error_code VARCHAR(100),

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE
);

-- Index for looking up by Stripe payment intent ID (used by webhooks)
CREATE INDEX IF NOT EXISTS idx_payments_stripe_payment_intent_id
    ON payments(stripe_payment_intent_id);

-- Index for looking up by status
CREATE INDEX IF NOT EXISTS idx_payments_status
    ON payments(status);

-- Index for customer lookups
CREATE INDEX IF NOT EXISTS idx_payments_customer_email
    ON payments(customer_email);