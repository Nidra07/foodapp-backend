-- Search module (domain module ~29: Search & Discovery).
--
-- Uses Postgres full-text search (tsvector + GIN index) rather than a
-- separate search engine (Elasticsearch/Meilisearch/Typesense). This is
-- a deliberate simplicity choice: the platform's search needs (restaurant
-- name/cuisine/description matching, menu item name/description
-- matching, combined with existing PostGIS distance filtering) fit
-- comfortably within what Postgres FTS can do, and adding a second
-- datastore + sync pipeline just for search would be a lot of new
-- operational surface for a benefit that hasn't been shown to be
-- needed yet. Revisit if relevance quality or query volume outgrows
-- what a GIN index can serve — see docs/assumptions.md.
--
-- This module is also a deliberate, documented exception to the "read
-- through a narrow consumer-defined interface" pattern used everywhere
-- else in this codebase for cross-module reads: full-text ranking +
-- geo-filtering + joins across restaurants and menu_items is inherently
-- a SQL-level concern that doesn't decompose cleanly into per-record
-- interface calls. Search's own repository queries the restaurants and
-- menu_items tables directly — same class of exception already made for
-- Notifications' DeviceLookupAdapter/ContactLookupAdapter in Phase 6.

ALTER TABLE restaurants ADD COLUMN search_vector tsvector
  GENERATED ALWAYS AS (
    setweight(to_tsvector('english', coalesce(name, '')), 'A') ||
    setweight(to_tsvector('english', array_to_string(cuisine_tags, ' ')), 'B') ||
    setweight(to_tsvector('english', coalesce(description, '')), 'C')
  ) STORED;

CREATE INDEX idx_restaurants_search_vector ON restaurants USING GIN (search_vector);

ALTER TABLE menu_items ADD COLUMN search_vector tsvector
  GENERATED ALWAYS AS (
    setweight(to_tsvector('english', coalesce(name, '')), 'A') ||
    setweight(to_tsvector('english', coalesce(description, '')), 'C')
  ) STORED;

CREATE INDEX idx_menu_items_search_vector ON menu_items USING GIN (search_vector);

-- Search query log: powers "trending searches" / autocomplete suggestions
-- and gives the eventual admin/analytics view visibility into what
-- customers are actually searching for (including zero-result queries,
-- which are a signal for menu/restaurant gaps). user_id is nullable
-- since search should work for unauthenticated browsing too.
CREATE TABLE search_logs (
  id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id       UUID REFERENCES users(id),
  query         VARCHAR(200) NOT NULL,
  search_type   VARCHAR(20) NOT NULL DEFAULT 'restaurant', -- 'restaurant' | 'item'
  result_count  INTEGER NOT NULL DEFAULT 0,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_search_logs_created ON search_logs (created_at DESC);
CREATE INDEX idx_search_logs_query ON search_logs (lower(query));
