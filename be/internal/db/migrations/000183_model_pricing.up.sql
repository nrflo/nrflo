-- Per-MTok pricing (USD) for cost accounting and PriceClass tier
-- classification. Nullable: a model with no seeded row falls back to
-- name-based tier classification and shows no cost.
ALTER TABLE models ADD COLUMN price_in REAL;
ALTER TABLE models ADD COLUMN price_out REAL;
ALTER TABLE models ADD COLUMN price_cache_write REAL;
ALTER TABLE models ADD COLUMN price_cache_read REAL;

UPDATE models SET price_in = 10, price_out = 50, price_cache_write = 12.5, price_cache_read = 1.0
WHERE id = 'fable-5';

UPDATE models SET price_in = 5, price_out = 25, price_cache_write = 6.25, price_cache_read = 0.5
WHERE id IN ('opus-4-6', 'opus-4-6-1m', 'opus-4-7', 'opus-4-7-1m', 'opus-4-8', 'opus-4-8-1m');

UPDATE models SET price_in = 3, price_out = 15, price_cache_write = 3.75, price_cache_read = 0.3
WHERE id = 'sonnet-5';

UPDATE models SET price_in = 1, price_out = 5, price_cache_write = 1.25, price_cache_read = 0.1
WHERE id = 'haiku-4-5';

-- OpenAI rows: cache_read = 10% of price_in, cache_write = 1.25x price_in.
UPDATE models SET price_in = 5, price_out = 30, price_cache_write = 6.25, price_cache_read = 0.5
WHERE id = 'gpt-5.6-sol';

UPDATE models SET price_in = 2.5, price_out = 15, price_cache_write = 3.125, price_cache_read = 0.25
WHERE id = 'gpt-5.6-terra';

UPDATE models SET price_in = 1, price_out = 6, price_cache_write = 1.25, price_cache_read = 0.1
WHERE id = 'gpt-5.6-luna';
