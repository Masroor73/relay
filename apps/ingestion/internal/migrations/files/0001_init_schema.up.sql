CREATE TABLE events (
    event_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key VARCHAR(255) UNIQUE NOT NULL,
    source VARCHAR(50) NOT NULL,
    payload JSONB NOT NULL,
    received_at TIMESTAMPTZ DEFAULT now() NOT NULL,
    status VARCHAR(20) DEFAULT 'pending' NOT NULL
);

CREATE TABLE outbox (
    outbox_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(event_id),
    attempt_count INT DEFAULT 0 NOT NULL,
    next_attempt_at TIMESTAMPTZ DEFAULT now() NOT NULL,
    last_error TEXT
);

CREATE TABLE dead_letter_events (
    dlq_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(event_id),
    final_error TEXT NOT NULL,
    moved_at TIMESTAMPTZ DEFAULT now() NOT NULL
);

CREATE TABLE orders (
    order_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    stripe_event_id UUID NOT NULL REFERENCES events(event_id),
    amount_cents BIGINT NOT NULL,
    status VARCHAR(20) DEFAULT 'confirmed' NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now() NOT NULL
);