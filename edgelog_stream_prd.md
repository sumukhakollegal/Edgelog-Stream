# Product Requirements Document: EdgeLog Stream
## High-Performance Distributed Data Ingestion and Routing System

**Version:** 1.0  
**Date:** September 2025  
**Author:** Product Engineering Team  
**Status:** Draft

---

## 1. Executive Summary

### 1.1 Problem Statement

Modern organizations generate massive volumes of event data from distributed sources - API calls, user interactions, IoT devices, and geographic systems. Current solutions often struggle with:
- **Scale limitations**: Many systems fail at 10-50K events/second
- **High latency**: P99 latencies exceeding 500ms impact real-time use cases
- **Cost inefficiency**: Over-provisioned infrastructure for peak loads
- **Operational complexity**: Difficult to maintain and monitor at scale
- **Vendor lock-in**: Proprietary formats and APIs prevent flexibility

Companies like Cloudflare process 40M+ HTTP requests/second globally, requiring purpose-built pipelines that existing solutions cannot economically handle.

### 1.2 Solution Overview

EdgeLog Stream is a high-throughput, low-latency data ingestion and routing platform designed for modern event-driven architectures. Built with Go for performance and reliability, it provides:
- **Extreme throughput**: 100K+ events/second on commodity hardware
- **Sub-100ms latency**: P99 latency under 100ms end-to-end
- **Horizontal scalability**: Linear scaling with added nodes
- **Multi-tenant isolation**: Secure data separation by design
- **Production observability**: Comprehensive metrics, logs, and traces

### 1.3 Key Differentiators

- **Performance First**: Optimized for minimal GC pressure and CPU cache efficiency
- **Geographic Intelligence**: Native support for geo-distributed event routing
- **Cost Efficient**: 10x lower infrastructure cost vs. commercial alternatives
- **Developer Experience**: Simple APIs, comprehensive documentation, local development support
- **Cloud Native**: Kubernetes-ready with auto-scaling and self-healing capabilities

---

## 2. System Architecture

### 2.1 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                          Client Applications                         │
│        (Web Apps, Mobile, IoT Devices, Geographic Systems)          │
└────────────┬──────────────────────────────────────┬─────────────────┘
             │ HTTP/gRPC                             │ HTTP/gRPC
             ▼                                       ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        EdgeLog Ingestor Service                      │
│  • Request validation & authentication                               │
│  • Schema enforcement                                                │
│  • Rate limiting (Redis-backed token buckets)                       │
│  • Request deduplication                                            │
└────────────┬──────────────────────────────────────┬─────────────────┘
             │                                       │
             ▼                                       ▼
┌──────────────────────┐                 ┌──────────────────────────┐
│    Redis Cluster     │                 │   Kafka/Redpanda         │
│ • Rate limit state   │◄────────────────│  • Event topics          │
│ • Dedup cache        │                 │  • Partitioned by region │
│ • Hot data cache     │                 │  • 3x replication       │
└──────────────────────┘                 └────────────┬──────────────┘
                                                      │
                                                      ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        EdgeLog Router Service                        │
│  • Consistent hashing for data sharding                             │
│  • Geographic routing logic                                         │
│  • Backpressure management                                          │
│  • Batch aggregation (size/time triggers)                          │
└────────────┬───────────────────────────────────────┬────────────────┘
             │                                       │
             ▼                                       ▼
┌──────────────────────┐                 ┌──────────────────────────┐
│   ClickHouse         │                 │    Dead Letter Queue     │
│ • Primary analytics  │                 │  • Failed events         │
│ • Time-series data   │                 │  • Retry logic          │
│ • Materialized views │                 │  • Alert triggers       │
└──────────────────────┘                 └──────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────────────────────┐
│                     Future: GraphQL Query Gateway                    │
│              • Real-time queries on materialized views              │
└─────────────────────────────────────────────────────────────────────┘
```

### 2.2 Component Details

#### Ingestor Service
- **Technology**: Go with fasthttp for HTTP, gRPC for binary protocol
- **Concurrency**: Worker pool pattern with bounded channels
- **Memory**: Zero-allocation path for hot code paths
- **Validation**: JSON Schema validation with compiled validators

#### Redis Layer
- **Deployment**: Redis Cluster mode for HA
- **Usage patterns**:
  - Rate limiting: Sliding window counters per tenant
  - Deduplication: 5-minute TTL bloom filters
  - Hot cache: LRU for frequently accessed data

#### Message Broker (Kafka/Redpanda)
- **Partitioning**: 50 partitions per topic for parallelism
- **Retention**: 24 hours for replay capability
- **Compression**: Snappy for optimal CPU/bandwidth trade-off

#### Router Service
- **Sharding**: Jump consistent hashing for stable routing
- **Batching**: Adaptive batching with 1000 events or 100ms timeout
- **Circuit breaker**: Hystrix-style pattern for downstream protection

#### ClickHouse Sink
- **Table engine**: MergeTree with daily partitions
- **Inserts**: Async bulk inserts with 10K event batches
- **Replication**: ReplicatedMergeTree for HA

---

## 3. Functional Requirements

### 3.1 Event Ingestion

**FR-001: Multi-Protocol Support**
- HTTP/1.1 and HTTP/2 endpoints
- gRPC for binary protocol efficiency
- WebSocket for streaming ingestion
- Batch upload via multipart/form-data

**FR-002: Schema Validation**
- JSON Schema Draft 7 support
- Custom validation rules per tenant
- Schema versioning and migration
- Validation error details in responses

**FR-003: Authentication & Authorization**
- API key authentication
- JWT token support for service accounts
- Per-tenant rate limits
- IP allowlisting capability

### 3.2 Event Routing

**FR-004: Content-Based Routing**
- Route by tenant_id field
- Geographic routing by lat/lon or region code
- Custom routing rules via CEL expressions
- Priority-based routing for critical events

**FR-005: Consistent Hashing**
```go
// Jump Consistent Hash implementation
func JumpHash(key uint64, numBuckets int) int {
    b := -1
    j := 0
    for j < numBuckets {
        b = j
        key = key*2862933555777941757 + 1
        j = int(float64(b+1) * (float64(int64(1)<<31) / float64((key>>33)+1)))
    }
    return b
}
```

### 3.3 Delivery Guarantees

**FR-006: At-Least-Once Delivery**
- Kafka acknowledgment before client response
- Automatic retry with exponential backoff
- Dead letter queue after 3 failures
- Idempotency keys for duplicate prevention

**FR-007: Backpressure Handling**
```go
// Token bucket implementation
type TokenBucket struct {
    tokens    int64
    capacity  int64
    refillRate int64
    lastRefill time.Time
    mu        sync.Mutex
}

func (tb *TokenBucket) Allow() bool {
    tb.mu.Lock()
    defer tb.mu.Unlock()
    
    now := time.Now()
    elapsed := now.Sub(tb.lastRefill)
    tokensToAdd := int64(elapsed.Seconds()) * tb.refillRate
    
    tb.tokens = min(tb.capacity, tb.tokens+tokensToAdd)
    tb.lastRefill = now
    
    if tb.tokens > 0 {
        tb.tokens--
        return true
    }
    return false
}
```

### 3.4 Batch Processing

**FR-008: Intelligent Batching**
- Size trigger: 1000 events per batch
- Time trigger: 100ms maximum wait
- Compression before storage
- Batch metadata for tracking

---

## 4. Non-Functional Requirements

### 4.1 Performance

**NFR-001: Throughput**
- Baseline: 100,000 events/second single node
- Peak: 150,000 events/second with CPU <80%
- Sustained: 24-hour run at 100K eps without degradation

**NFR-002: Latency**
- P50: <10ms end-to-end
- P95: <50ms end-to-end  
- P99: <100ms end-to-end
- P99.9: <200ms end-to-end

### 4.2 Scalability

**NFR-003: Horizontal Scaling**
- Linear scaling up to 10 nodes
- Auto-scaling based on CPU/memory metrics
- Zero-downtime scaling operations

### 4.3 Reliability

**NFR-004: Availability**
- 99.9% uptime (43.2 minutes downtime/month)
- Graceful degradation under overload
- Circuit breakers for downstream services
- Health checks with automatic recovery

### 4.4 Observability

**NFR-005: Metrics**
- Prometheus metrics on /metrics endpoint
- Custom business metrics via StatsD
- Distributed tracing with OpenTelemetry
- Structured logging with correlation IDs

### 4.5 Security

**NFR-006: Input Validation**
- Max payload size: 1MB per event
- Rate limiting: 10,000 req/min per tenant
- SQL injection prevention
- XSS protection for stored data

---

## 5. Data Model

### 5.1 Generic Event Structure

```json
{
  "event_id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": "2025-09-29T10:30:00.000Z",
  "tenant_id": "tenant_123",
  "event_type": "user.action",
  "source": {
    "service": "api-gateway",
    "version": "2.1.0",
    "host": "api-us-east-1.example.com"
  },
  "data": {
    // Flexible JSON payload
  },
  "metadata": {
    "correlation_id": "req_456",
    "user_id": "user_789",
    "session_id": "sess_012",
    "geo": {
      "lat": 37.7749,
      "lon": -122.4194,
      "city": "San Francisco",
      "country": "US"
    }
  }
}
```

### 5.2 Geographic Event Example (Flight Tracking)

```json
{
  "event_id": "flight_evt_123",
  "timestamp": "2025-09-29T10:30:00.000Z",
  "tenant_id": "flightaware",
  "event_type": "flight.position.update",
  "data": {
    "flight_id": "UA328",
    "aircraft": {
      "registration": "N12345",
      "type": "B737"
    },
    "position": {
      "lat": 40.7128,
      "lon": -74.0060,
      "altitude_ft": 35000,
      "speed_kts": 450,
      "heading": 270
    },
    "route": {
      "origin": "JFK",
      "destination": "LAX",
      "waypoints": ["KJFK", "BWZ", "DSM", "KLAX"]
    }
  }
}
```

### 5.3 Sports Event Example (Football Searches)

```json
{
  "event_id": "sports_evt_456",
  "timestamp": "2025-09-29T10:30:00.000Z",
  "tenant_id": "sportsstats",
  "event_type": "search.query",
  "data": {
    "query": "Manchester United vs Liverpool",
    "filters": {
      "sport": "football",
      "league": "Premier League",
      "season": "2025-26"
    },
    "results_count": 42,
    "response_time_ms": 23,
    "user_context": {
      "device": "mobile",
      "app_version": "4.2.0"
    }
  }
}
```

### 5.4 Comprehensive Database Schema

#### 5.4.1 ClickHouse Core Tables

**Main Events Table**
```sql
-- Primary events table with optimized storage
CREATE TABLE IF NOT EXISTS events_distributed ON CLUSTER '{cluster}' (
    -- Identity
    event_id UUID DEFAULT generateUUIDv4(),
    idempotency_key String DEFAULT '',
    
    -- Temporal
    timestamp DateTime64(3, 'UTC'),
    date Date DEFAULT toDate(timestamp),
    hour DateTime DEFAULT toStartOfHour(timestamp),
    minute DateTime DEFAULT toStartOfMinute(timestamp),
    
    -- Classification
    tenant_id LowCardinality(String) CODEC(ZSTD(1)),
    event_type LowCardinality(String) CODEC(ZSTD(1)),
    event_version UInt8 DEFAULT 1,
    
    -- Source Information
    source_service LowCardinality(String),
    source_version String,
    source_host String,
    source_datacenter LowCardinality(String),
    
    -- Payload
    data String CODEC(ZSTD(3)), -- JSON string, compressed
    data_size_bytes UInt32 DEFAULT length(data),
    
    -- Metadata
    metadata String CODEC(ZSTD(3)), -- JSON metadata
    correlation_id String,
    user_id String,
    session_id String,
    
    -- Geographic Data
    geo_lat Nullable(Float32),
    geo_lon Nullable(Float32),
    geo_country LowCardinality(String) DEFAULT '',
    geo_region LowCardinality(String) DEFAULT '',
    geo_city String DEFAULT '',
    geo_postal_code String DEFAULT '',
    
    -- Processing Metrics
    ingestion_time DateTime64(3) DEFAULT now64(3),
    processing_time_ms UInt16,
    partition_id UInt16,
    offset UInt64,
    
    -- Quality Indicators
    is_replay UInt8 DEFAULT 0,
    is_synthetic UInt8 DEFAULT 0,
    quality_score Float32 DEFAULT 1.0,
    
    -- Indexes for common queries
    INDEX idx_tenant_date (tenant_id, date) TYPE minmax GRANULARITY 4,
    INDEX idx_type_hour (event_type, hour) TYPE minmax GRANULARITY 4,
    INDEX idx_user (user_id) TYPE bloom_filter(0.01) GRANULARITY 8,
    INDEX idx_correlation (correlation_id) TYPE bloom_filter(0.01) GRANULARITY 8,
    INDEX idx_geo_hash (geo_lat, geo_lon) TYPE minmax GRANULARITY 8,
    INDEX idx_json_data (data) TYPE tokenbf_v1(32768, 3, 0) GRANULARITY 1
    
) ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/events', '{replica}')
PARTITION BY (tenant_id, toYYYYMM(date))
ORDER BY (tenant_id, event_type, timestamp, event_id)
PRIMARY KEY (tenant_id, event_type, timestamp)
TTL date + INTERVAL 30 DAY DELETE
SETTINGS 
    index_granularity = 8192,
    merge_tree_max_rows_for_concurrent_read = 163840,
    merge_tree_max_bytes_for_concurrent_read = 67108864,
    min_bytes_for_wide_part = 10485760;

-- Distributed table for queries
CREATE TABLE IF NOT EXISTS events AS events_distributed
ENGINE = Distributed('{cluster}', currentDatabase(), events_distributed, rand());
```

**Flight Tracking Events Table**
```sql
CREATE TABLE IF NOT EXISTS flight_events ON CLUSTER '{cluster}' (
    -- Event Identity
    event_id UUID DEFAULT generateUUIDv4(),
    timestamp DateTime64(3, 'UTC'),
    date Date DEFAULT toDate(timestamp),
    
    -- Flight Information
    flight_id String,
    flight_number LowCardinality(String),
    airline_code LowCardinality(String),
    aircraft_registration String,
    aircraft_type LowCardinality(String),
    
    -- Position Data
    latitude Float64,
    longitude Float64,
    altitude_feet UInt32,
    altitude_meters UInt16 MATERIALIZED round(altitude_feet * 0.3048),
    speed_knots UInt16,
    speed_kmh UInt16 MATERIALIZED round(speed_knots * 1.852),
    heading UInt16,
    vertical_rate_fpm Int16,
    
    -- Route Information
    origin_airport FixedString(4),
    destination_airport FixedString(4),
    waypoints Array(String),
    estimated_arrival DateTime,
    actual_arrival Nullable(DateTime),
    
    -- Status
    flight_phase Enum8(
        'unknown' = 0,
        'preflight' = 1,
        'taxi' = 2,
        'takeoff' = 3,
        'climb' = 4,
        'cruise' = 5,
        'descent' = 6,
        'approach' = 7,
        'landing' = 8,
        'taxi_arrival' = 9,
        'arrived' = 10
    ),
    
    -- Enrichment Data
    weather_conditions String CODEC(ZSTD(3)),
    airspace_sector LowCardinality(String),
    
    -- Indexes
    INDEX idx_flight (flight_id, date) TYPE minmax GRANULARITY 4,
    INDEX idx_geo_position (latitude, longitude) TYPE minmax GRANULARITY 8,
    INDEX idx_airports (origin_airport, destination_airport) TYPE minmax GRANULARITY 4
    
) ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/flight_events', '{replica}')
PARTITION BY toYYYYMMDD(date)
ORDER BY (date, flight_id, timestamp)
TTL date + INTERVAL 7 DAY TO DISK 'cold'
SETTINGS storage_policy = 'tiered';
```

**Sports Search Events Table**
```sql
CREATE TABLE IF NOT EXISTS sports_events ON CLUSTER '{cluster}' (
    -- Event Identity
    event_id UUID DEFAULT generateUUIDv4(),
    timestamp DateTime64(3, 'UTC'),
    date Date DEFAULT toDate(timestamp),
    
    -- Search Context
    query_text String CODEC(ZSTD(3)),
    query_tokens Array(String),
    query_intent Enum8('live_score' = 1, 'team_info' = 2, 'player_stats' = 3, 'schedule' = 4, 'other' = 5),
    
    -- Sports Classification
    sport LowCardinality(String),
    league LowCardinality(String),
    season String,
    
    -- Teams and Players
    team_home LowCardinality(String),
    team_away LowCardinality(String),
    players Array(String),
    
    -- Match Information
    match_id Nullable(String),
    match_date Nullable(Date),
    match_status Enum8('scheduled' = 1, 'live' = 2, 'finished' = 3, 'cancelled' = 4),
    
    -- Search Metrics
    results_count UInt32,
    response_time_ms UInt16,
    relevance_score Float32,
    click_through_rate Float32,
    
    -- User Context
    user_id String,
    device_type LowCardinality(String),
    app_version String,
    user_country FixedString(2),
    user_language FixedString(2),
    
    -- Indexes
    INDEX idx_query_tokens (query_tokens) TYPE bloom_filter(0.01) GRANULARITY 4,
    INDEX idx_sport_league (sport, league) TYPE minmax GRANULARITY 4,
    INDEX idx_teams (team_home, team_away) TYPE bloom_filter(0.01) GRANULARITY 4,
    INDEX idx_match (match_id) TYPE bloom_filter(0.01) GRANULARITY 8
    
) ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/sports_events', '{replica}')
PARTITION BY (sport, toYYYYMM(date))
ORDER BY (sport, league, date, timestamp)
TTL date + INTERVAL 90 DAY;
```

#### 5.4.2 ClickHouse Materialized Views

**Events Per Minute Aggregation**
```sql
CREATE MATERIALIZED VIEW IF NOT EXISTS events_per_minute_mv ON CLUSTER '{cluster}'
ENGINE = ReplicatedAggregatingMergeTree('/clickhouse/tables/{shard}/events_per_minute_mv', '{replica}')
PARTITION BY toYYYYMMDD(minute)
ORDER BY (tenant_id, event_type, minute)
TTL minute + INTERVAL 7 DAY
AS SELECT
    tenant_id,
    event_type,
    toStartOfMinute(timestamp) AS minute,
    count() AS event_count,
    avg(processing_time_ms) AS avg_processing_time,
    quantile(0.99)(processing_time_ms) AS p99_processing_time,
    sum(data_size_bytes) AS total_bytes,
    uniqExact(user_id) AS unique_users,
    uniqExact(correlation_id) AS unique_requests
FROM events_distributed
GROUP BY tenant_id, event_type, minute;
```

**Geographic Distribution View**
```sql
CREATE MATERIALIZED VIEW IF NOT EXISTS geo_distribution_mv ON CLUSTER '{cluster}'
ENGINE = ReplicatedSummingMergeTree('/clickhouse/tables/{shard}/geo_distribution_mv', '{replica}')
PARTITION BY toYYYYMM(hour)
ORDER BY (hour, geo_country, geo_region, tenant_id)
AS SELECT
    toStartOfHour(timestamp) AS hour,
    tenant_id,
    geo_country,
    geo_region,
    geo_city,
    round(geo_lat, 1) AS lat_bucket,
    round(geo_lon, 1) AS lon_bucket,
    count() AS event_count,
    uniqExact(user_id) AS unique_users,
    sum(data_size_bytes) AS total_bytes
FROM events_distributed
WHERE geo_lat IS NOT NULL AND geo_lon IS NOT NULL
GROUP BY hour, tenant_id, geo_country, geo_region, geo_city, lat_bucket, lon_bucket;
```

**Tenant Usage Summary**
```sql
CREATE MATERIALIZED VIEW IF NOT EXISTS tenant_usage_mv ON CLUSTER '{cluster}'
ENGINE = ReplicatedReplacingMergeTree('/clickhouse/tables/{shard}/tenant_usage_mv', '{replica}')
PARTITION BY toYYYYMM(date)
ORDER BY (date, tenant_id)
AS SELECT
    date,
    tenant_id,
    count() AS daily_events,
    sum(data_size_bytes) AS daily_bytes,
    uniqExact(user_id) AS daily_active_users,
    uniqExact(event_type) AS unique_event_types,
    avg(processing_time_ms) AS avg_latency,
    quantile(0.99)(processing_time_ms) AS p99_latency,
    min(timestamp) AS first_event_time,
    max(timestamp) AS last_event_time
FROM events_distributed
GROUP BY date, tenant_id;
```

**Flight Position Latest View**
```sql
CREATE MATERIALIZED VIEW IF NOT EXISTS flight_position_latest_mv ON CLUSTER '{cluster}'
ENGINE = ReplicatedReplacingMergeTree('/clickhouse/tables/{shard}/flight_position_latest_mv', '{replica}', timestamp)
ORDER BY (flight_id)
AS SELECT
    flight_id,
    argMax(timestamp, timestamp) AS last_update,
    argMax(latitude, timestamp) AS current_latitude,
    argMax(longitude, timestamp) AS current_longitude,
    argMax(altitude_feet, timestamp) AS current_altitude,
    argMax(speed_knots, timestamp) AS current_speed,
    argMax(heading, timestamp) AS current_heading,
    argMax(flight_phase, timestamp) AS current_phase,
    any(origin_airport) AS origin,
    any(destination_airport) AS destination
FROM flight_events
GROUP BY flight_id;
```

#### 5.4.3 Support Tables

**Dead Letter Queue Table**
```sql
CREATE TABLE IF NOT EXISTS dead_letter_queue ON CLUSTER '{cluster}' (
    -- Error Information
    error_id UUID DEFAULT generateUUIDv4(),
    error_timestamp DateTime64(3) DEFAULT now64(3),
    error_type LowCardinality(String),
    error_message String,
    error_stack_trace String CODEC(ZSTD(3)),
    
    -- Original Event
    original_event_id Nullable(UUID),
    original_payload String CODEC(ZSTD(3)),
    original_timestamp Nullable(DateTime64(3)),
    tenant_id LowCardinality(String),
    
    -- Processing Context
    retry_count UInt8 DEFAULT 0,
    max_retries UInt8 DEFAULT 3,
    next_retry_time DateTime,
    processing_stage LowCardinality(String),
    
    -- Resolution
    is_resolved UInt8 DEFAULT 0,
    resolved_timestamp Nullable(DateTime),
    resolution_notes String DEFAULT '',
    
    INDEX idx_tenant_error (tenant_id, error_type) TYPE minmax GRANULARITY 4,
    INDEX idx_retry (is_resolved, next_retry_time) TYPE minmax GRANULARITY 4
    
) ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/dlq', '{replica}')
PARTITION BY toYYYYMM(error_timestamp)
ORDER BY (error_timestamp, tenant_id, error_type)
TTL error_timestamp + INTERVAL 30 DAY;
```

**Schema Versions Table**
```sql
CREATE TABLE IF NOT EXISTS schema_versions ON CLUSTER '{cluster}' (
    tenant_id LowCardinality(String),
    event_type LowCardinality(String),
    version UInt32,
    schema_json String CODEC(ZSTD(3)),
    validation_rules String CODEC(ZSTD(3)),
    created_at DateTime DEFAULT now(),
    created_by String,
    is_active UInt8 DEFAULT 1,
    deprecated_at Nullable(DateTime),
    migration_script String DEFAULT '',
    
    -- Versioning
    backward_compatible UInt8 DEFAULT 1,
    forward_compatible UInt8 DEFAULT 0,
    
    PRIMARY KEY (tenant_id, event_type, version)
) ENGINE = ReplicatedReplacingMergeTree('/clickhouse/tables/{shard}/schema_versions', '{replica}', created_at)
ORDER BY (tenant_id, event_type, version);
```

**Audit Log Table**
```sql
CREATE TABLE IF NOT EXISTS audit_log ON CLUSTER '{cluster}' (
    audit_id UUID DEFAULT generateUUIDv4(),
    timestamp DateTime64(3) DEFAULT now64(3),
    
    -- Action Details
    action_type Enum8('create' = 1, 'update' = 2, 'delete' = 3, 'query' = 4, 'admin' = 5),
    resource_type LowCardinality(String),
    resource_id String,
    
    -- Actor Information
    actor_id String,
    actor_type Enum8('user' = 1, 'service' = 2, 'system' = 3),
    actor_ip IPv4,
    actor_user_agent String,
    
    -- Change Details
    before_value String CODEC(ZSTD(3)),
    after_value String CODEC(ZSTD(3)),
    change_reason String,
    
    -- Context
    tenant_id LowCardinality(String),
    correlation_id String,
    
    INDEX idx_actor (actor_id, timestamp) TYPE minmax GRANULARITY 4,
    INDEX idx_resource (resource_type, resource_id) TYPE minmax GRANULARITY 4
    
) ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/audit_log', '{replica}')
PARTITION BY toYYYYMM(timestamp)
ORDER BY (timestamp, tenant_id)
TTL timestamp + INTERVAL 90 DAY;
```

#### 5.4.4 Redis Data Structures

**Rate Limiting Structure**
```lua
-- Key pattern: rate_limit:{tenant_id}:{endpoint}:{window}
-- Implementation: Sliding window counter

-- Example keys:
rate_limit:tenant_123:ingest:minute -> 4523  -- Current minute count
rate_limit:tenant_123:ingest:hour -> 245632   -- Current hour count
rate_limit:tenant_123:ingest:day -> 8934521   -- Current day count

-- Token bucket for burst control
token_bucket:tenant_123 -> {
    "tokens": 9500,
    "capacity": 10000,
    "refill_rate": 100,
    "last_refill": 1727615234567
}
```

**Deduplication Cache**
```lua
-- Using Redis Bloom Filters
-- Key pattern: dedup:{tenant_id}:{date}:{hour}

BF.ADD dedup:tenant_123:2025-09-29:10 "event_uuid_12345"
BF.EXISTS dedup:tenant_123:2025-09-29:10 "event_uuid_12345"

-- TTL: 6 hours to handle late arrivals
EXPIRE dedup:tenant_123:2025-09-29:10 21600
```

**Hot Data Cache**
```lua
-- LRU cache for frequently accessed events
-- Key pattern: cache:event:{tenant_id}:{event_id}

SET cache:event:tenant_123:550e8400-e29b-41d4 '{
    "event_id": "550e8400-e29b-41d4",
    "timestamp": "2025-09-29T10:30:00.000Z",
    "data": {...}
}' EX 3600

-- Tenant metadata cache
HSET cache:tenant:tenant_123 
    schema_version "2"
    rate_limit "10000"
    routing_key "us-east-1"
    active "true"
```

**Circuit Breaker State**
```lua
-- Key pattern: circuit:{service}:{endpoint}
HSET circuit:clickhouse:write
    state "closed"  -- closed, open, half_open
    failures "0"
    successes "1000"
    last_failure_time "0"
    last_success_time "1727615234567"
    next_retry_time "0"
```

**Metrics Buffer**
```lua
-- Temporary metrics storage before Prometheus scrape
-- Key pattern: metrics:{metric_name}:{labels}

HINCRBY metrics:events_total "tenant=tenant_123,type=user.action" 1
HSET metrics:latency_histogram "tenant=tenant_123,p99" "45.3"

-- Sorted set for percentile calculations
ZADD latency:tenant_123 45.3 "1727615234567"
```

#### 5.4.5 Kafka Topic Schemas

**Main Events Topic**
```json
{
  "type": "record",
  "name": "Event",
  "namespace": "com.edgelog.events",
  "fields": [
    {"name": "event_id", "type": "string"},
    {"name": "timestamp", "type": "long", "logicalType": "timestamp-millis"},
    {"name": "tenant_id", "type": "string"},
    {"name": "event_type", "type": "string"},
    {"name": "event_version", "type": "int", "default": 1},
    {"name": "idempotency_key", "type": ["null", "string"], "default": null},
    {
      "name": "source",
      "type": {
        "type": "record",
        "name": "Source",
        "fields": [
          {"name": "service", "type": "string"},
          {"name": "version", "type": "string"},
          {"name": "host", "type": "string"},
          {"name": "datacenter", "type": ["null", "string"], "default": null}
        ]
      }
    },
    {"name": "data", "type": "string"},
    {
      "name": "metadata",
      "type": {
        "type": "map",
        "values": "string"
      }
    },
    {
      "name": "geo",
      "type": ["null", {
        "type": "record",
        "name": "GeoLocation",
        "fields": [
          {"name": "lat", "type": "float"},
          {"name": "lon", "type": "float"},
          {"name": "country", "type": ["null", "string"], "default": null},
          {"name": "region", "type": ["null", "string"], "default": null},
          {"name": "city", "type": ["null", "string"], "default": null}
        ]
      }],
      "default": null
    }
  ]
}
```

**Dead Letter Topic**
```json
{
  "type": "record",
  "name": "DeadLetterEvent",
  "namespace": "com.edgelog.dlq",
  "fields": [
    {"name": "error_id", "type": "string"},
    {"name": "error_timestamp", "type": "long", "logicalType": "timestamp-millis"},
    {"name": "error_type", "type": "string"},
    {"name": "error_message", "type": "string"},
    {"name": "error_stack_trace", "type": ["null", "string"], "default": null},
    {"name": "original_topic", "type": "string"},
    {"name": "original_partition", "type": "int"},
    {"name": "original_offset", "type": "long"},
    {"name": "original_payload", "type": "bytes"},
    {"name": "retry_count", "type": "int", "default": 0},
    {"name": "max_retries", "type": "int", "default": 3},
    {"name": "next_retry_time", "type": ["null", "long"], "default": null},
    {"name": "processing_stage", "type": "string"},
    {"name": "tenant_id", "type": ["null", "string"], "default": null}
  ]
}
```

#### 5.4.6 Database Optimization Settings

**ClickHouse Cluster Configuration**
```xml
<clickhouse>
    <remote_servers>
        <edgelog_cluster>
            <shard>
                <replica>
                    <host>clickhouse-01</host>
                    <port>9000</port>
                </replica>
                <replica>
                    <host>clickhouse-02</host>
                    <port>9000</port>
                </replica>
            </shard>
            <shard>
                <replica>
                    <host>clickhouse-03</host>
                    <port>9000</port>
                </replica>
                <replica>
                    <host>clickhouse-04</host>
                    <port>9000</port>
                </replica>
            </shard>
        </edgelog_cluster>
    </remote_servers>
    
    <profiles>
        <default>
            <max_memory_usage>10737418240</max_memory_usage> <!-- 10GB -->
            <max_bytes_before_external_group_by>5368709120</max_bytes_before_external_group_by>
            <max_parallel_replicas>2</max_parallel_replicas>
            <distributed_product_mode>global</distributed_product_mode>
        </default>
    </profiles>
    
    <merge_tree>
        <max_bytes_to_merge_at_max_space_in_pool>161061273600</max_bytes_to_merge_at_max_space_in_pool>
        <number_of_free_entries_in_pool_to_execute_mutation>10</number_of_free_entries_in_pool_to_execute_mutation>
    </merge_tree>
</clickhouse>
```

**Storage Policies**
```xml
<storage_configuration>
    <disks>
        <hot>
            <path>/var/lib/clickhouse/hot/</path>
            <max_data_part_size_bytes>10737418240</max_data_part_size_bytes>
        </hot>
        <cold>
            <path>/var/lib/clickhouse/cold/</path>
        </cold>
    </disks>
    <policies>
        <tiered>
            <volumes>
                <hot_volume>
                    <disk>hot</disk>
                    <max_data_part_size_bytes>10737418240</max_data_part_size_bytes>
                </hot_volume>
                <cold_volume>
                    <disk>cold</disk>
                </cold_volume>
            </volumes>
            <move_factor>0.1</move_factor>
        </tiered>
    </policies>
</storage_configuration>
```

#### 5.4.7 Migration Scripts

**Initial Schema Setup**
```sql
-- V1__initial_schema.sql
CREATE DATABASE IF NOT EXISTS edgelog ON CLUSTER '{cluster}';

-- Create all tables in order
-- 1. Core tables
-- 2. Materialized views
-- 3. Support tables

-- Grant permissions
GRANT SELECT, INSERT ON edgelog.* TO 'ingestor';
GRANT SELECT ON edgelog.* TO 'reader';
GRANT ALL ON edgelog.* TO 'admin';
```

**Adding New Index**
```sql
-- V2__add_user_session_index.sql
ALTER TABLE events ON CLUSTER '{cluster}'
ADD INDEX IF NOT EXISTS idx_user_session (user_id, session_id) 
TYPE bloom_filter(0.01) GRANULARITY 8;
```

**Data Retention Update**
```sql
-- V3__update_retention_policy.sql
ALTER TABLE events ON CLUSTER '{cluster}'
MODIFY TTL date + INTERVAL 60 DAY DELETE;

-- Add archive table for long-term storage
CREATE TABLE IF NOT EXISTS events_archive AS events
ENGINE = S3('s3://edgelog-archive/events/*.parquet');
```

#### 5.4.8 Sample Queries and Performance Patterns

**High-Performance Query Examples**

```sql
-- Real-time event stream for specific tenant (uses primary key)
SELECT 
    event_id,
    timestamp,
    event_type,
    data
FROM events
WHERE tenant_id = 'tenant_123'
    AND timestamp >= now() - INTERVAL 5 MINUTE
ORDER BY timestamp DESC
LIMIT 100
FORMAT JSONEachRow;
-- Expected performance: <10ms

-- Geographic distribution with heatmap data
SELECT 
    round(geo_lat, 1) as lat_bucket,
    round(geo_lon, 1) as lon_bucket,
    count() as event_count,
    avg(processing_time_ms) as avg_latency
FROM events
WHERE date >= today() - 7
    AND geo_lat IS NOT NULL
GROUP BY lat_bucket, lon_bucket
HAVING event_count > 100
ORDER BY event_count DESC
FORMAT JSONEachRow;
-- Expected performance: <50ms with geo_distribution_mv

-- Top event types by tenant with percentages
WITH total_events AS (
    SELECT count() as total
    FROM tenant_usage_mv
    WHERE date = today()
        AND tenant_id = 'tenant_123'
)
SELECT 
    event_type,
    count() as count,
    round(count() * 100.0 / total_events.total, 2) as percentage
FROM events, total_events
WHERE date = today()
    AND tenant_id = 'tenant_123'
GROUP BY event_type, total_events.total
ORDER BY count DESC
LIMIT 20;
-- Expected performance: <20ms

-- Flight tracking - current positions of all flights
SELECT 
    flight_id,
    current_latitude,
    current_longitude,
    current_altitude,
    current_speed,
    current_phase,
    origin,
    destination,
    formatDateTime(last_update, '%Y-%m-%d %H:%i:%s') as last_update_formatted
FROM flight_position_latest_mv
WHERE last_update >= now() - INTERVAL 10 MINUTE
FORMAT JSONEachRow;
-- Expected performance: <5ms (materialized view)

-- Sports trending searches in last hour
SELECT 
    query_text,
    count() as search_count,
    avg(results_count) as avg_results,
    avg(response_time_ms) as avg_response_time,
    sum(click_through_rate * search_count) / sum(search_count) as weighted_ctr
FROM sports_events
WHERE timestamp >= now() - INTERVAL 1 HOUR
GROUP BY query_text
ORDER BY search_count DESC
LIMIT 50;
-- Expected performance: <30ms

-- Error analysis from dead letter queue
SELECT 
    error_type,
    processing_stage,
    count() as error_count,
    avg(retry_count) as avg_retries,
    countIf(is_resolved = 1) as resolved_count,
    round(resolved_count * 100.0 / error_count, 2) as resolution_rate
FROM dead_letter_queue
WHERE error_timestamp >= now() - INTERVAL 24 HOUR
GROUP BY error_type, processing_stage
ORDER BY error_count DESC;
-- Expected performance: <15ms
```

**Query Optimization Patterns**

```sql
-- GOOD: Uses partition pruning and primary key
SELECT * FROM events
WHERE tenant_id = 'tenant_123'
    AND date >= '2025-09-01'
    AND date <= '2025-09-30'
    AND event_type = 'user.action';

-- BAD: Full table scan without partition pruning
SELECT * FROM events
WHERE JSONExtractString(data, 'user_name') = 'john';

-- GOOD: Pre-aggregated materialized view
SELECT * FROM events_per_minute_mv
WHERE tenant_id = 'tenant_123'
    AND minute >= now() - INTERVAL 1 HOUR;

-- BAD: Real-time aggregation on raw data
SELECT 
    toStartOfMinute(timestamp) as minute,
    count()
FROM events
WHERE timestamp >= now() - INTERVAL 1 HOUR
GROUP BY minute;
```

#### 5.4.9 Data Lifecycle Management

**Automated Data Tiering**

```sql
-- Hot tier: Last 7 days on SSD
ALTER TABLE events ON CLUSTER '{cluster}'
MODIFY TTL 
    date + INTERVAL 7 DAY TO DISK 'cold',
    date + INTERVAL 30 DAY TO VOLUME 'archive',
    date + INTERVAL 90 DAY DELETE;

-- Archive to S3 before deletion
CREATE MATERIALIZED VIEW events_to_s3_mv
ENGINE = S3(
    's3://edgelog-archive/{tenant_id}/{date}/*.parquet',
    'AWS_KEY',
    'AWS_SECRET'
)
AS SELECT * FROM events
WHERE date = yesterday();
```

**Continuous Optimization Jobs**

```sql
-- Daily optimization of frequently queried partitions
OPTIMIZE TABLE events 
PARTITION ('tenant_123', '202509')
FINAL;

-- Weekly deduplication
OPTIMIZE TABLE events_distributed 
DEDUPLICATE BY event_id;

-- Monthly statistics update for query planner
ANALYZE TABLE events;
```

#### 5.4.10 Redis Operation Patterns

**Rate Limiting Implementation**

```go
// Go implementation of sliding window rate limiter
type RateLimiter struct {
    client *redis.Client
}

func (r *RateLimiter) CheckLimit(tenantID string, limit int, window time.Duration) (bool, error) {
    key := fmt.Sprintf("rate:%s:%d", tenantID, time.Now().Unix()/int64(window.Seconds()))
    
    pipe := r.client.Pipeline()
    incr := pipe.Incr(ctx, key)
    pipe.Expire(ctx, key, window*2)
    
    _, err := pipe.Exec(ctx)
    if err != nil {
        return false, err
    }
    
    return incr.Val() <= int64(limit), nil
}
```

**Deduplication Implementation**

```go
// Bloom filter based deduplication
func (r *RedisCache) CheckDuplicate(eventID string, tenantID string) (bool, error) {
    hourKey := fmt.Sprintf("dedup:%s:%s", 
        tenantID, 
        time.Now().Format("2006010215"))
    
    // Try to add to bloom filter
    added, err := r.client.Do(ctx, "BF.ADD", hourKey, eventID).Bool()
    if err != nil {
        // Bloom filter doesn't exist, create it
        if strings.Contains(err.Error(), "not found") {
            // Create with 0.01% false positive rate, 1M capacity
            _, err = r.client.Do(ctx, "BF.RESERVE", hourKey, "0.0001", "1000000").Result()
            if err != nil {
                return false, err
            }
            // Try adding again
            added, err = r.client.Do(ctx, "BF.ADD", hourKey, eventID).Bool()
        }
    }
    
    // Set TTL on first add
    if added {
        r.client.Expire(ctx, hourKey, 6*time.Hour)
    }
    
    return !added, nil // Return true if duplicate
}
```

**Hot Cache Pattern**

```go
// LRU cache with Redis
func (r *RedisCache) GetOrSet(key string, fetch func() ([]byte, error)) ([]byte, error) {
    // Try cache first
    val, err := r.client.Get(ctx, "cache:"+key).Bytes()
    if err == nil {
        return val, nil
    }
    
    // Cache miss - fetch and set
    data, err := fetch()
    if err != nil {
        return nil, err
    }
    
    // Set with sliding expiration
    r.client.Set(ctx, "cache:"+key, data, 5*time.Minute)
    
    // Update access pattern for analytics
    r.client.ZIncrBy(ctx, "cache:access", 1, key)
    
    return data, nil
}
```

#### 5.4.11 Performance Benchmarks and Capacity Planning

**Database Operation Benchmarks**

| Operation | Target Latency | Throughput | Notes |
|-----------|---------------|------------|-------|
| Single Event Insert | <1ms | 100K/sec | Async batch inserts |
| Batch Insert (1000 events) | <10ms | 1M events/sec | Optimal batch size |
| Simple Query (by primary key) | <5ms | 10K/sec | Direct shard query |
| Aggregation Query (1 day) | <50ms | 1K/sec | Uses materialized views |
| Geographic Query | <30ms | 500/sec | Spatial index utilized |
| Full Text Search | <100ms | 200/sec | Token bloom filter |
| Redis Rate Check | <0.5ms | 50K/sec | In-memory operation |
| Bloom Filter Check | <0.3ms | 100K/sec | Probabilistic structure |
| Cache Hit | <0.5ms | 50K/sec | LRU cache |

**Storage Capacity Planning**

```yaml
Event Size Calculations:
  Average Event Size: 1KB
  Compression Ratio: 5:1 (ZSTD)
  Stored Event Size: ~200 bytes
  
  Daily Volume (100K events/sec):
    Raw: 8.64 billion events
    Storage: ~1.7TB/day compressed
    With Replication (3x): ~5.1TB/day
  
  Monthly Storage:
    Hot Tier (7 days): ~36TB
    Warm Tier (23 days): ~117TB
    Total: ~153TB/month
  
  Redis Memory:
    Rate Limiting: ~100MB (sliding windows)
    Bloom Filters: ~500MB (1M items × 6 hours)
    Hot Cache: ~2GB (10K items × 200KB)
    Total: ~3GB per node
  
  Kafka Retention:
    24 hours × 100K/sec × 1KB = ~8.6TB
    With Replication (3x): ~26TB
```

**Scaling Guidelines**

```yaml
ClickHouse Cluster:
  Minimum: 4 nodes (2 shards × 2 replicas)
  Recommended: 8 nodes (4 shards × 2 replicas)
  
  Per Node Requirements:
    CPU: 16 cores
    RAM: 64GB
    SSD: 2TB NVMe for hot data
    HDD: 10TB for cold data
    Network: 10Gbps
  
  Scaling Triggers:
    - CPU > 70% sustained
    - Disk I/O > 80% utilization
    - Query latency P99 > 100ms
    - Storage > 80% capacity

Redis Cluster:
  Minimum: 3 nodes
  Recommended: 6 nodes (3 masters × 1 replica)
  
  Per Node Requirements:
    CPU: 4 cores
    RAM: 16GB
    Network: 10Gbps
  
  Scaling Triggers:
    - Memory > 80% usage
    - CPU > 60% sustained
    - Network > 5Gbps sustained

Kafka Cluster:
  Minimum: 3 brokers
  Recommended: 5 brokers
  
  Per Broker Requirements:
    CPU: 8 cores
    RAM: 32GB
    SSD: 1TB for logs
    Network: 10Gbps
  
  Scaling Triggers:
    - Disk usage > 70%
    - Network > 7Gbps sustained
    - Consumer lag > 100K messages
```

#### 5.4.12 Database Monitoring Queries

**System Health Monitoring**

```sql
-- ClickHouse system metrics
SELECT
    formatReadableSize(sum(bytes)) as total_size,
    formatReadableQuantity(sum(rows)) as total_rows,
    count() as parts_count,
    maxIf(modification_time, active) as last_modified
FROM system.parts
WHERE database = 'edgelog'
    AND table = 'events'
GROUP BY table;

-- Query performance monitoring
SELECT
    query_id,
    user,
    formatDateTime(query_start_time, '%Y-%m-%d %H:%i:%s') as start_time,
    query_duration_ms,
    formatReadableSize(memory_usage) as memory,
    read_rows,
    result_rows,
    substring(query, 1, 100) as query_preview
FROM system.query_log
WHERE query_start_time >= now() - INTERVAL 1 HOUR
    AND type = 'QueryFinish'
    AND query_duration_ms > 1000
ORDER BY query_duration_ms DESC
LIMIT 20;

-- Replication lag monitoring
SELECT
    database,
    table,
    replica_name,
    absolute_delay,
    total_replicas,
    active_replicas,
    queue_size,
    inserts_in_queue,
    merges_in_queue
FROM system.replicas
WHERE database = 'edgelog';

-- Disk usage by partition
SELECT
    partition,
    formatReadableSize(sum(bytes_on_disk)) as size,
    count() as parts,
    min(min_date) as oldest_data,
    max(max_date) as newest_data
FROM system.parts
WHERE database = 'edgelog'
    AND table = 'events'
    AND active
GROUP BY partition
ORDER BY sum(bytes_on_disk) DESC;
```

**Redis Monitoring Commands**

```lua
-- Memory analysis
INFO memory

-- Slow queries
SLOWLOG GET 10

-- Key distribution
EVAL "
    local cursor = '0'
    local pattern_counts = {}
    repeat
        local result = redis.call('SCAN', cursor, 'COUNT', 1000)
        cursor = result[1]
        local keys = result[2]
        for _, key in ipairs(keys) do
            local pattern = string.match(key, '^([^:]+)')
            pattern_counts[pattern] = (pattern_counts[pattern] or 0) + 1
        end
    until cursor == '0'
    
    local results = {}
    for pattern, count in pairs(pattern_counts) do
        table.insert(results, pattern .. ':' .. count)
    end
    return results
" 0

-- Cache hit rate
EVAL "
    local hits = redis.call('GET', 'stats:cache:hits') or 0
    local misses = redis.call('GET', 'stats:cache:misses') or 0
    local total = hits + misses
    if total > 0 then
        return string.format('%.2f%%', (hits * 100.0) / total)
    else
        return 'No data'
    end
" 0
```

---

## 6. API Specifications

### 6.1 Ingestion API

#### REST Endpoint

**POST /v1/events**
```yaml
Request:
  Headers:
    Content-Type: application/json
    X-API-Key: ${API_KEY}
    X-Idempotency-Key: ${UNIQUE_KEY} # Optional
  Body:
    event_id: string (UUID)
    timestamp: string (ISO8601)
    tenant_id: string
    event_type: string
    data: object
    metadata: object

Response:
  Success (202):
    {
      "status": "accepted",
      "event_id": "550e8400-e29b-41d4-a716-446655440000",
      "partition": 12,
      "offset": 456789
    }
  
  Rate Limited (429):
    {
      "error": "rate_limit_exceeded",
      "retry_after": 60,
      "limit": 10000,
      "remaining": 0
    }
```

**POST /v1/events/batch**
```yaml
Request:
  Headers:
    Content-Type: application/json
    X-API-Key: ${API_KEY}
  Body:
    events: array[Event] # Max 1000 events

Response:
  Success (202):
    {
      "status": "accepted",
      "accepted": 998,
      "rejected": 2,
      "errors": [
        {
          "index": 456,
          "error": "invalid_timestamp"
        }
      ]
    }
```

#### gRPC Service

```protobuf
syntax = "proto3";

service EventIngestion {
  rpc IngestEvent(Event) returns (IngestResponse);
  rpc IngestEventStream(stream Event) returns (stream IngestResponse);
  rpc IngestBatch(EventBatch) returns (BatchResponse);
}

message Event {
  string event_id = 1;
  google.protobuf.Timestamp timestamp = 2;
  string tenant_id = 3;
  string event_type = 4;
  google.protobuf.Struct data = 5;
  map<string, string> metadata = 6;
}
```

### 6.2 Health & Metrics

**GET /health**
```json
{
  "status": "healthy",
  "checks": {
    "kafka": "connected",
    "redis": "connected",
    "clickhouse": "connected"
  },
  "version": "1.2.3",
  "uptime_seconds": 86400
}
```

**GET /metrics** (Prometheus format)
```
# HELP edgelog_events_total Total events processed
# TYPE edgelog_events_total counter
edgelog_events_total{tenant="tenant_123",status="success"} 1234567

# HELP edgelog_latency_seconds Event processing latency
# TYPE edgelog_latency_seconds histogram
edgelog_latency_seconds_bucket{le="0.01"} 456789
edgelog_latency_seconds_bucket{le="0.05"} 567890
edgelog_latency_seconds_bucket{le="0.1"} 678901
```

---

## 7. Monitoring & Alerting

### 7.1 Key Metrics

#### Golden Signals
- **Latency**: P50, P95, P99 per endpoint
- **Traffic**: Events/second, Bytes/second
- **Errors**: 4xx, 5xx, Kafka failures, ClickHouse errors  
- **Saturation**: CPU, Memory, Disk I/O, Network

#### Business Metrics
```yaml
Events Processed:
  - edgelog_events_total
  - edgelog_events_per_tenant
  - edgelog_events_per_type
  
Pipeline Health:
  - edgelog_kafka_lag
  - edgelog_batch_size_histogram
  - edgelog_dead_letter_queue_size
  
Resource Usage:
  - edgelog_goroutines_count
  - edgelog_heap_alloc_bytes
  - edgelog_gc_pause_seconds
```

### 7.2 Grafana Dashboard Requirements

**Main Dashboard Panels:**
1. Events/sec timeseries (with tenant breakdown)
2. Latency percentiles heatmap
3. Error rate percentage
4. Kafka consumer lag
5. ClickHouse insert performance
6. Redis hit rate
7. Resource utilization gauges
8. Geographic event distribution map

### 7.3 Alert Conditions

```yaml
Critical Alerts:
  - name: HighErrorRate
    condition: rate(errors) > 1% for 5m
    action: PagerDuty
    
  - name: HighLatency
    condition: p99_latency > 200ms for 10m
    action: PagerDuty
    
  - name: KafkaConsumerLag
    condition: lag > 100000 for 5m
    action: PagerDuty

Warning Alerts:
  - name: HighMemoryUsage
    condition: memory > 80% for 15m
    action: Slack
    
  - name: DiskSpaceLow
    condition: disk_free < 20% for 30m
    action: Email
```

---

## 8. Development Phases

### Phase 1: MVP Foundation (Days 1-2)
**Goal**: Basic event flow from ingestion to storage

**Deliverables**:
- Basic HTTP ingestion endpoint
- Kafka producer/consumer setup
- Simple ClickHouse writer
- Docker Compose environment
- 10K events/sec benchmark

**Code structure**:
```
edgelog-stream/
├── cmd/
│   ├── ingestor/
│   └── router/
├── pkg/
│   ├── models/
│   ├── kafka/
│   └── clickhouse/
├── docker-compose.yml
└── Makefile
```

### Phase 2: Production Features (Days 3-4)
**Goal**: Add reliability and performance features

**Deliverables**:
- Redis integration for rate limiting
- Deduplication logic
- Batch processing with size/time triggers
- Consistent hashing router
- Backpressure management
- Dead letter queue
- 50K events/sec benchmark

### Phase 3: Observability (Days 5-6)
**Goal**: Complete monitoring and testing

**Deliverables**:
- Prometheus metrics integration
- Grafana dashboards
- Distributed tracing
- Unit test coverage >80%
- Integration test suite
- Load testing with vegeta
- 100K events/sec benchmark achieved

### Phase 4: Optimization & Enhancement (Days 7-8)
**Goal**: Performance tuning and advanced features

**Deliverables**:
- Memory optimization (object pooling)
- CPU profiling and optimization
- gRPC endpoint
- Geographic routing logic
- GraphQL query gateway prototype
- Chaos testing scenarios
- 150K events/sec stretch goal

---

## 9. Testing Strategy

### 9.1 Unit Tests

```go
// Example test for token bucket
func TestTokenBucket_RateLimit(t *testing.T) {
    bucket := NewTokenBucket(100, 10) // 100 capacity, 10/sec refill
    
    // Should allow initial burst
    for i := 0; i < 100; i++ {
        assert.True(t, bucket.Allow())
    }
    
    // Should block when exhausted
    assert.False(t, bucket.Allow())
    
    // Should refill after time
    time.Sleep(1 * time.Second)
    assert.True(t, bucket.Allow())
}
```

### 9.2 Integration Tests

```yaml
Test Scenarios:
  - End-to-end event flow
  - Kafka partition failover
  - Redis connection loss
  - ClickHouse write failures
  - Rate limit enforcement
  - Deduplication verification
```

### 9.3 Load Testing

```bash
# Vegeta load test example
echo "POST http://localhost:8080/v1/events" | \
  vegeta attack -duration=60s -rate=100000 \
  -header "Content-Type: application/json" \
  -body event.json | \
  vegeta report
```

**Target benchmarks**:
- 100K req/sec sustained for 1 hour
- P99 latency <100ms under load
- Memory usage <4GB
- CPU usage <80%

### 9.4 Chaos Testing

```yaml
Failure Scenarios:
  - Network partition between services
  - Kafka broker failure
  - Redis node failure
  - 50% packet loss
  - CPU throttling
  - Memory pressure
  - Disk I/O saturation
```

---

## 10. Deployment

### 10.1 Local Development

```yaml
# docker-compose.yml
version: '3.8'
services:
  redpanda:
    image: vectorized/redpanda:latest
    command:
      - redpanda start
      - --kafka-addr PLAINTEXT://0.0.0.0:9092
      - --advertise-kafka-addr PLAINTEXT://redpanda:9092
    ports:
      - "9092:9092"
      - "9644:9644"
  
  clickhouse:
    image: clickhouse/clickhouse-server:latest
    ports:
      - "8123:8123"
      - "9000:9000"
    volumes:
      - ./clickhouse/init.sql:/docker-entrypoint-initdb.d/init.sql
  
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
  
  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
  
  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
```

### 10.2 Configuration Management

```yaml
# config.yaml
server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 10s
  write_timeout: 10s

kafka:
  brokers:
    - localhost:9092
  topic: events
  partitions: 50
  replication: 3

clickhouse:
  host: localhost:9000
  database: edgelog
  batch_size: 10000
  batch_timeout: 100ms

redis:
  addr: localhost:6379
  max_retries: 3
  pool_size: 100

observability:
  metrics_port: 9191
  trace_sampling: 0.1
```

### 10.3 Future: Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: edgelog-ingestor
spec:
  replicas: 3
  selector:
    matchLabels:
      app: edgelog-ingestor
  template:
    spec:
      containers:
      - name: ingestor
        image: edgelog/ingestor:latest
        resources:
          requests:
            memory: "2Gi"
            cpu: "2"
          limits:
            memory: "4Gi"
            cpu: "4"
        env:
        - name: KAFKA_BROKERS
          value: "kafka-0:9092,kafka-1:9092"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
```

---

## 11. Success Metrics

### 11.1 Performance Benchmarks

| Metric | Target | Stretch Goal |
|--------|--------|-------------|
| Throughput | 100K events/sec | 150K events/sec |
| P99 Latency | <100ms | <50ms |
| Memory Usage | <4GB | <2GB |
| CPU Efficiency | <80% at peak | <60% at peak |
| Startup Time | <5 seconds | <2 seconds |

### 11.2 Code Quality

- Test coverage: >80%
- Go report card: A+
- No critical security vulnerabilities
- Documentation coverage: 100% of public APIs
- Lint-free codebase

### 11.3 Demo Scenarios

**Scenario 1: Flight Tracking Dashboard**
- Ingest 50K flight position updates/second
- Show real-time map with aircraft positions
- Query flights by region with <10ms response
- Demonstrate geographic sharding

**Scenario 2: Sports Event Analytics**
- Process 100K search queries/second during match
- Show trending queries in real-time
- Aggregate statistics by team/league
- Demonstrate multi-tenant isolation

**Scenario 3: Chaos Resilience**
- Kill Kafka broker during ingestion
- Show automatic failover
- Verify zero data loss
- Demonstrate self-healing

---

## 12. Risk Mitigation

### 12.1 Technical Risks

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| GC pressure under load | High | Medium | Use object pooling, minimize allocations |
| Kafka partition skew | Medium | High | Monitor partition lag, implement rebalancing |
| ClickHouse write amplification | High | Low | Tune batch sizes, use async inserts |
| Redis memory exhaustion | Medium | Medium | Implement eviction policies, monitor memory |
| Network saturation | High | Low | Implement compression, optimize payloads |

### 12.2 Scope Management

**In Scope**:
- Core ingestion pipeline
- Basic routing and sharding
- Essential monitoring
- Performance optimization

**Out of Scope (Future)**:
- Multi-region replication
- Advanced analytics queries
- Machine learning pipelines
- Custom transformation DSL

### 12.3 Complexity Management

- Start with single-node deployment
- Add distribution incrementally
- Use standard libraries where possible
- Avoid premature optimization
- Document all design decisions

---

## 13. Future Platform Evolution

### 13.1 ClickScope (Days 9-10)
**Advanced ClickHouse Operations**

Features:
- Distributed table management
- Automated sharding and replication
- Materialized view optimization
- Query performance analyzer
- Storage tiering (hot/warm/cold)

Technical Implementation:
- ZooKeeper coordination for replicas
- Distributed DDL execution
- Automatic partition management
- Query result caching layer

### 13.2 GraphAlert (Days 11-12)
**Real-time Query and Alerting Platform**

Features:
- GraphQL API with subscriptions
- Complex event processing (CEP)
- Alert rule engine with Rego
- Multi-channel notifications
- Alert suppression and deduplication

Technical Implementation:
```graphql
type Query {
  events(
    tenant: String!
    timeRange: TimeRange!
    filters: [EventFilter!]
  ): EventConnection!
  
  metrics(
    metric: String!
    groupBy: [String!]
    timeRange: TimeRange!
  ): TimeSeriesData!
}

type Subscription {
  eventStream(
    tenant: String!
    eventTypes: [String!]
  ): Event!
  
  alertTrigger(
    alertId: String!
  ): Alert!
}
```

### 13.3 Cardinality Tamer (Days 12-13)
**High-Cardinality Metrics Management**

Features:
- Cardinality analysis and budgeting
- Adaptive sampling for high-cardinality series
- Recording rules for aggregation
- Exemplar tracing integration
- Cost attribution per tenant

Technical Implementation:
- Prometheus recording rules
- HyperLogLog for cardinality estimation
- Adaptive sampling algorithms
- Series lifecycle management

### 13.4 Complete Platform Integration

**Unified Data Platform Architecture**:
```
EdgeLog Stream → ClickScope → GraphAlert → Cardinality Tamer
     ↓              ↓            ↓              ↓
  Ingestion    Analytics    Querying    Observability
```

**End-to-End Capabilities**:
- 1M+ events/second across cluster
- Sub-second analytics queries
- Real-time alerting with <1s detection
- Multi-region data replication
- 99.99% availability SLA

---

## 14. Resume Bullet Points

Based on successful completion of this project, the following achievements can be highlighted:

• **Architected and implemented high-performance distributed data pipeline** processing 100K+ events/second with sub-100ms P99 latency using Go, Kafka, and ClickHouse

• **Designed geographic-aware event routing system** with consistent hashing and multi-tenant isolation, reducing infrastructure costs by 10x compared to commercial solutions

• **Built comprehensive observability platform** with Prometheus metrics, distributed tracing, and Grafana dashboards, achieving 99.9% system availability

• **Optimized Go application performance** through memory pooling and CPU cache optimization, reducing GC pause times by 90% and memory usage by 60%

• **Implemented production-grade reliability features** including circuit breakers, backpressure handling, and chaos testing, ensuring zero data loss under failure scenarios

• **Developed full-stack data platform** with GraphQL API gateway and real-time alerting engine, supporting complex analytics queries with <10ms response times

---

## 15. Glossary

**Backpressure**: Flow control mechanism that prevents system overload by signaling upstream components to slow down

**Cardinality**: The number of unique values in a dataset; high cardinality can impact metric storage systems

**CEL (Common Expression Language)**: Google's expression language for evaluating expressions in configuration

**Consistent Hashing**: Distributed hashing scheme that minimizes remapping when nodes are added/removed

**Dead Letter Queue (DLQ)**: Queue for messages that cannot be processed successfully after multiple attempts

**Exemplar**: Representative trace linked to a metric data point for debugging

**Jump Hash**: Consistent hashing algorithm with minimal memory overhead and no storage requirements

**Materialized View**: Pre-computed query results stored as a table for faster access

**P99 Latency**: The latency value below which 99% of requests complete

**Partitioning**: Dividing data across multiple storage units for parallel processing

**Rate Limiting**: Controlling the rate of requests to prevent abuse and ensure fair resource usage

**Redpanda**: Kafka-compatible streaming platform written in C++ for better performance

**Token Bucket**: Algorithm for rate limiting that allows burst traffic while maintaining average rate

**Write Amplification**: Ratio of data written to storage versus data written by application

---

## Appendix A: Sample Configuration Files

### A.1 Prometheus Configuration
```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'edgelog'
    static_configs:
      - targets:
        - 'ingestor-1:9191'
        - 'ingestor-2:9191'
        - 'router-1:9191'
        
rule_files:
  - 'alerts.yml'
```

### A.2 Alert Rules
```yaml
groups:
  - name: edgelog_alerts
    interval: 30s
    rules:
      - alert: HighErrorRate
        expr: rate(edgelog_errors_total[5m]) > 0.01
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "High error rate detected"
          description: "Error rate is {{ $value | humanizePercentage }}"
```

---

## Appendix B: Performance Tuning Guide

### B.1 Go Optimizations
```go
// Object pooling example
var eventPool = sync.Pool{
    New: func() interface{} {
        return &Event{}
    },
}

func processEvent(data []byte) {
    event := eventPool.Get().(*Event)
    defer func() {
        event.Reset()
        eventPool.Put(event)
    }()
    
    // Process event without allocation
}
```

### B.2 Linux Kernel Tuning
```bash
# /etc/sysctl.conf
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.core.netdev_max_backlog = 65536
net.ipv4.tcp_tw_reuse = 1
fs.file-max = 2097152
```

---

This PRD provides a comprehensive blueprint for building EdgeLog Stream, balancing technical sophistication with practical achievability for a solo developer project. The phased approach ensures steady progress while maintaining production-grade quality standards.