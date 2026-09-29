-- Claude Sonnet 5 per the Anthropic model page: $2/$10, cache $2.50 write /
-- $0.20 read, released 2026-06-30.
UPDATE models SET price_in = 2, price_out = 10, price_cache_write = 2.5, price_cache_read = 0.2,
    release_date = '2026-06-30', updated_at = '2026-09-29T00:00:00Z'
WHERE id = 'sonnet-5';
