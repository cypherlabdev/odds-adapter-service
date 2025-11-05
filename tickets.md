# odds-adapter-service - Ticket Tracking

## Service Overview
**Repository**: github.com/cypherlabdev/odds-adapter-service
**Purpose**: Ingest odds data from multiple providers (Trade360 RabbitMQ, OptiOdds HTTP) and publish to Kafka
**Implementation Status**: 70% complete (adapters working, missing persistence, validation, resilience)
**Language**: Go 1.21+
**Critical Blockers**: No README documentation, missing readiness checks, no persistence layer

## Current Implementation

### ✅ Completed (70%)
- **Trade360 Adapter**: RabbitMQ consumer with manual ack, QoS limiting, graceful shutdown
- **OptiOdds Adapter**: HTTP polling with configurable interval
- **Odds Aggregator**: Batching (100 events or 5s timeout), Kafka publishing with compression (Snappy)
- **Data Transformation**: Convert provider-specific formats to unified RawOdds model
- **Market Type Mapping**: Maps Trade360/OptiOdds market types to internal enum
- **Kafka Integration**: segmentio/kafka-go with RequireAll acks, partitioning by provider
- **HTTP Server**: Health checks (/health, /ready) and Prometheus metrics (/metrics)
- **Structured Logging**: zerolog with component-level loggers
- **Unit Tests**: Test coverage for adapters and aggregator
- **Graceful Shutdown**: Context cancellation propagates to all components

### ❌ Missing (30%)
- **No README.md**: No documentation at all
- **Readiness Check Not Implemented**: TODO comment in [main.go:166](cmd/server/main.go#L166)
- **No Persistence Layer**: No caching in Redis or PostgreSQL (all data lost on restart)
- **No Data Validation**: Raw provider data sent to Kafka without sanitization
- **No Resilience Patterns**: Missing circuit breaker, retry logic, rate limiting
- **No Metrics**: Beyond basic Prometheus endpoint, no adapter-specific metrics
- **No Dead Letter Queue**: Failed messages not retried or logged for debugging
- **No Schema Validation**: Provider data formats not validated before processing

## Existing Asana Tickets

### 1. [1211394356065955] ENG-66: Odds Adapter
**Task ID**: 1211394356065955
**ENG Field**: ENG-66
**URL**: https://app.asana.com/0/1211254851871080/1211394356065955
**Type**: feature
**Assignee**: sj@cypherlab.tech
**Labels**: Backend, odds-adapter-service

**Implementation Status**: 70% complete
- ✅ Trade360 RabbitMQ adapter
- ✅ OptiOdds HTTP adapter
- ✅ Odds aggregator with batching
- ✅ Kafka publishing
- ✅ Health checks (/health, /ready endpoints exist)
- ✅ Prometheus metrics endpoint
- ✅ Unit tests
- ❌ No README documentation
- ❌ Readiness check not implemented (TODO in code)
- ❌ No persistence layer (Redis/PostgreSQL)
- ❌ No data validation or sanitization
- ❌ No resilience patterns (circuit breaker, retry, rate limiting)
- ❌ No comprehensive metrics
- ❌ No dead letter queue for failed messages

**Dependencies**:
- ⬆️ Depends on: None (independent data ingestion service)
- ⬇️ Blocks: [1211394356065966] ENG-70 (data-normalizer-service) - consumes from odds-adapter Kafka topic

**Code References**:
- [main.go:166](cmd/server/main.go#L166) - TODO: readiness check not implemented
- [trade360_adapter.go](internal/adapter/trade360_adapter.go) - RabbitMQ consumer
- [optiodds_adapter.go](internal/adapter/optiodds_adapter.go) - HTTP polling adapter
- [odds_aggregator.go](internal/aggregator/odds_aggregator.go) - Batching and Kafka publishing

## Tickets to Create in Asana

### 1. [NEW] Create README and Documentation (P0)
**Proposed Priority**: P0 (No documentation exists)
**Type**: documentation
**Assignee**: sj@cypherlab.tech
**Labels**: Backend, odds-adapter-service, Documentation
**Depends On**: [1211394356065955] ENG-66 (Odds Adapter)

**Rationale**:
Service has NO README.md file. Developers cannot understand:
- What providers are supported (Trade360, OptiOdds)
- How to configure the service
- What environment variables are required
- How data flows through the system
- How to run locally for development
- What Kafka topics are published to

**Acceptance Criteria**:
1. Create README.md with sections:
   - Overview: Purpose and architecture
   - Supported Providers: Trade360 (RabbitMQ), OptiOdds (HTTP)
   - Data Flow: Adapter → Channel → Aggregator → Kafka
   - Configuration: Environment variables and config.yaml
   - Quick Start: Local development setup
   - Deployment: Kubernetes deployment guide
   - Health Checks: /health, /ready, /metrics endpoints
   - Monitoring: Prometheus metrics and alerting
   - Troubleshooting: Common issues
2. Create ARCHITECTURE.md documenting:
   - Provider adapters design
   - Channel-based buffering (10,000 capacity)
   - Batching strategy (100 events or 5s timeout)
   - Kafka partitioning by provider
   - RawOdds data model
3. Create PROVIDERS.md documenting:
   - Trade360: RabbitMQ connection, message format, error handling
   - OptiOdds: HTTP API, polling interval, rate limits
   - Adding new providers: Interface and implementation guide
4. Add inline code documentation (godoc comments) for public APIs
5. Create config/config.example.yaml with all configuration options documented

---

### 2. [NEW] Implement Proper Readiness Check (P1)
**Proposed Priority**: P1 (Required for Kubernetes deployment)
**Type**: feature
**Assignee**: sj@cypherlab.tech
**Labels**: Backend, odds-adapter-service, Health Checks
**Depends On**: [1211394356065955] ENG-66 (Odds Adapter)

**Rationale**:
Readiness handler has TODO comment ([main.go:166](cmd/server/main.go#L166)) and always returns 200. Kubernetes will route traffic to unhealthy pods. Real readiness check should verify:
- Kafka connection is healthy
- RabbitMQ connection is healthy (if Trade360 enabled)
- HTTP client can reach OptiOdds (if enabled)
- No critical errors in last N seconds

**Acceptance Criteria**:
1. Implement readiness check in readyHandler:
   - Check Kafka writer is connected (attempt test write or metadata fetch)
   - Check Trade360 RabbitMQ connection status (if enabled)
   - Check OptiOdds HTTP client can reach API (if enabled)
   - Return 503 if any critical component is down
   - Return 200 only if all enabled components are healthy
2. Add health status tracking in each adapter:
   - Trade360Adapter.IsHealthy() bool
   - OptiOddsAdapter.IsHealthy() bool
   - OddsAggregator.IsHealthy() bool
3. Add structured logging for readiness check results
4. Add Prometheus metric: odds_adapter_ready{component} (gauge, 0 or 1)
5. Add configurable grace period (don't fail readiness immediately after transient error)
6. Update Kubernetes deployment YAML with proper readinessProbe configuration

**Technical Notes**:
- Readiness failures should not cause panic or service crash
- Use context with timeout for health checks (5 seconds max)
- Consider exponential backoff for repeated readiness failures

---

### 3. [NEW] Add Redis Persistence Layer (P1)
**Proposed Priority**: P1 (Performance and reliability)
**Type**: feature
**Assignee**: sj@cypherlab.tech
**Labels**: Backend, odds-adapter-service, Redis, Caching
**Depends On**: [1211394356065955] ENG-66 (Odds Adapter)

**Rationale**:
Service has no persistence layer. Issues:
- All odds data lost on restart (no recovery)
- Cannot deduplicate incoming odds
- Cannot track last processed message ID (replay on restart)
- No way to query recent odds for debugging
- Cannot implement rate limiting or throttling

**Acceptance Criteria**:
1. Add Redis client initialization in main.go
2. Implement odds caching with TTL:
   - Key: `odds:{provider}:{event_id}:{market}:{selection}`
   - Value: JSON-serialized RawOdds
   - TTL: 5 minutes (configurable)
3. Store last processed message ID per provider:
   - Trade360: Last RabbitMQ delivery tag
   - OptiOdds: Last poll timestamp
   - Use for recovery on restart
4. Implement deduplication:
   - Check Redis before processing odds
   - Skip if odds already processed in last N seconds
5. Add Redis health check to readiness endpoint
6. Add Prometheus metrics:
   - `odds_adapter_redis_operations_total{operation,status}`
   - `odds_adapter_redis_cache_hits_total`
   - `odds_adapter_redis_cache_misses_total`
7. Add configuration:
   - REDIS_HOST, REDIS_PORT, REDIS_PASSWORD
   - REDIS_DB, REDIS_TTL
   - Cache enabled/disabled flag
8. Handle Redis connection failures gracefully (continue without cache)

**Technical Notes**:
- Use github.com/redis/go-redis/v9
- Implement connection pooling (min 5, max 20 connections)
- Use Redis pipelining for batch operations
- Consider Redis Streams for message replay capability

---

### 4. [NEW] Add Data Validation and Sanitization (P1)
**Proposed Priority**: P1 (Data quality and security)
**Type**: feature
**Assignee**: sj@cypherlab.tech
**Labels**: Backend, odds-adapter-service, Data Quality
**Depends On**: [1211394356065955] ENG-66 (Odds Adapter)

**Rationale**:
Raw provider data is sent to Kafka without validation:
- Invalid odds values (negative, zero, NaN) can break downstream services
- Malformed timestamps cause parsing errors
- Missing required fields cause nil pointer panics
- Injection attacks possible through unsanitized strings

**Acceptance Criteria**:
1. Add validation layer before sending to aggregator:
   - Validate BackPrice > 0 and LayPrice > 0
   - Validate BackSize >= 0 and LaySize >= 0
   - Validate EventID, EventName, Sport, Market, Selection are non-empty
   - Validate timestamp is within reasonable range (not future, not > 24h old)
   - Sanitize string fields (trim whitespace, limit length, escape HTML)
2. Add validation errors logging:
   - Log validation failures with provider, event_id, reason
   - Add Prometheus metric: `odds_adapter_validation_failures_total{provider,reason}`
3. Implement dead letter queue for invalid odds:
   - Write invalid odds to separate Kafka topic: `raw_odds_invalid`
   - Include validation error in message payload
   - Allows manual review and debugging
4. Add configurable validation rules:
   - Min/max odds values (e.g., 1.01 to 1000.00)
   - Required fields list
   - String length limits
5. Add unit tests for validation:
   - Test all validation rules
   - Test edge cases (zero, negative, NaN, empty strings)
6. Document validation rules in README.md

**Technical Notes**:
- Use validator library (e.g., github.com/go-playground/validator/v10)
- Consider defining JSON Schema for provider data formats
- Validation should be fast (<1ms per odds)

---

### 5. [NEW] Add Resilience Patterns (P2)
**Proposed Priority**: P2 (Production reliability)
**Type**: feature
**Assignee**: sj@cypherlab.tech
**Labels**: Backend, odds-adapter-service, Reliability
**Depends On**: [1211394356065955] ENG-66 (Odds Adapter)

**Rationale**:
Service has no resilience patterns:
- OptiOdds HTTP failures cause adapter to crash
- RabbitMQ disconnections not handled
- Kafka publishing failures not retried
- No circuit breaker for downstream dependencies
- No rate limiting for upstream providers

**Acceptance Criteria**:
1. Add circuit breaker for OptiOdds HTTP calls:
   - Use github.com/sony/gobreaker
   - Trip after 5 consecutive failures
   - Half-open after 30 seconds
   - Add metric: `odds_adapter_circuit_breaker_state{provider}` (0=closed, 1=open, 2=half-open)
2. Add retry logic for OptiOdds HTTP calls:
   - Exponential backoff: 1s, 2s, 4s, 8s, 16s
   - Max 5 retries
   - Add metric: `odds_adapter_http_retries_total{provider}`
3. Add RabbitMQ reconnection logic:
   - Detect connection loss
   - Reconnect with exponential backoff
   - Resume consuming from last acknowledged message
   - Add metric: `odds_adapter_rabbitmq_reconnects_total`
4. Add Kafka publishing retry logic:
   - Already has MaxAttempts: 3 in aggregator
   - Add exponential backoff between attempts
   - Add metric: `odds_adapter_kafka_publish_retries_total`
5. Add rate limiting for OptiOdds:
   - Use golang.org/x/time/rate
   - Limit to 10 requests/second (configurable)
   - Add metric: `odds_adapter_rate_limit_exceeded_total{provider}`
6. Add graceful degradation:
   - If one provider fails, others continue
   - Log failures but don't crash service
   - Add metric: `odds_adapter_provider_failures_total{provider,reason}`

**Technical Notes**:
- Circuit breaker should be per-provider
- Retry backoff should use jitter to avoid thundering herd
- Rate limiting should be per-provider

---

### 6. [NEW] Add Comprehensive Metrics (P2)
**Proposed Priority**: P2 (Observability)
**Type**: feature
**Assignee**: sj@cypherlab.tech
**Labels**: Backend, odds-adapter-service, Observability
**Depends On**: [1211394356065955] ENG-66 (Odds Adapter)

**Rationale**:
Service has basic Prometheus endpoint but no adapter-specific metrics. Cannot answer:
- How many odds ingested per provider?
- What's the latency from provider to Kafka?
- What's the batch size distribution?
- How many odds dropped due to channel full?
- What's the success/failure rate per provider?

**Acceptance Criteria**:
1. Add provider adapter metrics:
   - `odds_adapter_odds_received_total{provider}` - Counter
   - `odds_adapter_odds_processed_total{provider,status}` - Counter (status: success/failure)
   - `odds_adapter_odds_latency_seconds{provider}` - Histogram (time from receivedAt to Kafka publish)
   - `odds_adapter_channel_size{provider}` - Gauge (current channel buffer size)
   - `odds_adapter_channel_drops_total{provider}` - Counter (dropped due to full channel)
2. Add aggregator metrics:
   - `odds_adapter_batch_size` - Histogram
   - `odds_adapter_batch_publish_duration_seconds` - Histogram
   - `odds_adapter_kafka_publish_failures_total` - Counter
3. Add provider-specific metrics:
   - Trade360: `odds_adapter_rabbitmq_messages_total{status}` - Counter
   - OptiOdds: `odds_adapter_http_requests_total{status}` - Counter
4. Add Grafana dashboard template:
   - Odds ingestion rate per provider
   - Latency percentiles (p50, p95, p99)
   - Batch size distribution
   - Channel utilization
   - Error rates
5. Create alerting rules:
   - `OddsAdapterChannelFull`: Alert if channel drops > 100/minute
   - `OddsAdapterHighLatency`: Alert if p95 latency > 10 seconds
   - `OddsAdapterProviderDown`: Alert if no odds received in 5 minutes
   - `OddsAdapterKafkaPublishFailures`: Alert if failures > 10/minute

**Technical Notes**:
- Use prometheus client library: github.com/prometheus/client_golang
- Metrics should be updated in-line with minimal overhead
- Consider using histogram buckets: 0.001, 0.01, 0.1, 1, 10, 100 seconds

## Implementation Priority Summary

### P0 (Critical - Zero documentation)
1. **Create README** - Service has no documentation

### P1 (High - Production readiness)
2. **Implement Readiness Check** - Kubernetes deployment requirement
3. **Add Redis Persistence** - Performance and reliability
4. **Add Data Validation** - Data quality and security

### P2 (Medium - Operational excellence)
5. **Add Resilience Patterns** - Circuit breaker, retry, rate limiting
6. **Add Comprehensive Metrics** - Observability and alerting

## Dependencies Graph

```
tam-protos [COMPLETE]
    ↓
odds-adapter-service (ENG-66) [70% complete]
    ├─ [NEW] Create README (P0)
    ├─ [NEW] Implement Readiness Check (P1)
    ├─ [NEW] Add Redis Persistence (P1)
    ├─ [NEW] Add Data Validation (P1)
    ├─ [NEW] Add Resilience Patterns (P2)
    └─ [NEW] Add Comprehensive Metrics (P2)
    ↓
data-normalizer-service (ENG-70) [Consumes raw_odds Kafka topic]
odds-optimizer-service (ENG-74) [Consumes from data-normalizer]
```

## Notes

- **Data Pipeline Entry Point**: First service in the odds processing pipeline
- **High Throughput**: Must handle thousands of odds updates per second
- **Multiple Providers**: Trade360 (RabbitMQ), OptiOdds (HTTP) with pluggable architecture
- **Channel Buffering**: 10,000 capacity buffer prevents backpressure
- **Batching**: Improves Kafka throughput (100 events or 5s timeout)
- **Provider Partitioning**: Kafka messages partitioned by provider for ordered consumption
- **Graceful Shutdown**: Context cancellation ensures no data loss

## Testing Requirements

All new tickets must include:
1. Unit tests with >80% coverage
2. Integration tests with testcontainers (Kafka, RabbitMQ, Redis)
3. Load tests simulating high-throughput scenarios (10,000 odds/sec)
4. Failure recovery tests (provider disconnection, Kafka downtime)
5. Performance benchmarks for validation and transformation logic
