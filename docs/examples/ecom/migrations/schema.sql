CREATE TABLE IF NOT EXISTS orders (
    id SERIAL PRIMARY KEY,
    customer_email VARCHAR(255) NOT NULL,
    customer_name VARCHAR(255),
    amount_cents BIGINT NOT NULL,
    currency VARCHAR(10) NOT NULL DEFAULT 'usd',
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    stripe_payment_id VARCHAR(255),
    stripe_payment_intent_id VARCHAR(255),
    stripe_client_secret TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    paid_at TIMESTAMP WITH TIME ZONE,
    canceled_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_orders_customer_email ON orders(customer_email);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
