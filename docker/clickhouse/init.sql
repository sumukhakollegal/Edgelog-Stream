-- EdgeLog Stream - ClickHouse Initial Schema
-- This creates the core events table for Phase 1

CREATE DATABASE IF NOT EXISTS edgelog;

USE edgelog;

-- Main events table - simplified for Phase 1 MVP
CREATE TABLE IF NOT EXISTS events (
    -- Identity
    event_id String,
    idempotency_key String DEFAULT '',

    -- Temporal
    timestamp DateTime64(3, 'UTC'),
    date Date DEFAULT toDate(timestamp),
    ingestion_time DateTime64(3, 'UTC') DEFAULT now64(3),

    -- Classification
    tenant_id LowCardinality(String),
    event_type LowCardinality(String),

    -- Source Information
    source_service String DEFAULT '',
    source_version String DEFAULT '',
    source_host String DEFAULT '',

    -- Payload (JSON string, compressed)
    data String CODEC(ZSTD(3)),

    -- Metadata
    metadata String CODEC(ZSTD(3)),
    correlation_id String DEFAULT '',
    user_id String DEFAULT '',
    session_id String DEFAULT '',

    -- Geographic Data (optional)
    geo_lat Nullable(Float32),
    geo_lon Nullable(Float32),
    geo_country String DEFAULT '',
    geo_city String DEFAULT '',

    -- Processing Metrics
    processing_time_ms UInt16 DEFAULT 0,

    -- Indexes for common queries
    INDEX idx_tenant_date (tenant_id, date) TYPE minmax GRANULARITY 4,
    INDEX idx_type (event_type) TYPE minmax GRANULARITY 4,
    INDEX idx_correlation (correlation_id) TYPE bloom_filter(0.01) GRANULARITY 8

) ENGINE = MergeTree()
PARTITION BY (tenant_id, toYYYYMM(date))
ORDER BY (tenant_id, event_type, timestamp, event_id)
PRIMARY KEY (tenant_id, event_type, timestamp)
TTL date + INTERVAL 30 DAY DELETE
SETTINGS
    index_granularity = 8192;

-- Simple materialized view for events per minute aggregation
CREATE MATERIALIZED VIEW IF NOT EXISTS events_per_minute_mv
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMMDD(minute)
ORDER BY (tenant_id, event_type, minute)
TTL minute + INTERVAL 7 DAY
AS SELECT
    tenant_id,
    event_type,
    toStartOfMinute(timestamp) AS minute,
    count() AS event_count,
    avg(processing_time_ms) AS avg_processing_time
FROM events
GROUP BY tenant_id, event_type, minute;

-- Dead letter queue for failed events
CREATE TABLE IF NOT EXISTS dead_letter_queue (
    error_id UUID DEFAULT generateUUIDv4(),
    error_timestamp DateTime64(3, 'UTC') DEFAULT now64(3),
    error_type String,
    error_message String,

    -- Original event data
    original_event_id String DEFAULT '',
    original_payload String CODEC(ZSTD(3)),
    tenant_id String DEFAULT '',

    -- Processing context
    retry_count UInt8 DEFAULT 0,
    processing_stage String DEFAULT '',

    INDEX idx_tenant_error (tenant_id, error_type) TYPE minmax GRANULARITY 4

) ENGINE = MergeTree()
PARTITION BY toYYYYMM(error_timestamp)
ORDER BY (error_timestamp, tenant_id, error_type)
TTL error_timestamp + INTERVAL 30 DAY;

-- Grant permissions for application user
-- Note: In production, use separate users with limited permissions
GRANT SELECT, INSERT ON edgelog.* TO default;
