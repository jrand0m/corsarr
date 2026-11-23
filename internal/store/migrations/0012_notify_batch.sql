-- 0012: deliveries for one download are batched into one message per connection.

ALTER TABLE notify_outbox ADD COLUMN batch_key TEXT NOT NULL DEFAULT '';
CREATE INDEX notify_outbox_batch ON notify_outbox (connection, batch_key, status);
