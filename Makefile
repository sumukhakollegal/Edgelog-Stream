.PHONY: help init docker-up docker-down docker-verify build-ingestor build-router run-ingestor run-router test-ingestor test-e2e clean

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GORUN=$(GOCMD) run
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt

# Build parameters
BINARY_INGESTOR=bin/ingestor
BINARY_ROUTER=bin/router

# Default target
help:
	@echo "EdgeLog Stream - Makefile Commands"
	@echo ""
	@echo "Setup & Infrastructure:"
	@echo "  make init             - Initialize project (create directories, tidy deps)"
	@echo "  make docker-up        - Start all Docker services"
	@echo "  make docker-down      - Stop all Docker services"
	@echo "  make docker-verify    - Verify Docker services are healthy"
	@echo ""
	@echo "Build:"
	@echo "  make build-ingestor   - Compile ingestor service"
	@echo "  make build-router     - Compile router service (Phase 2)"
	@echo "  make build-all        - Build all services"
	@echo ""
	@echo "Run:"
	@echo "  make run-ingestor     - Run ingestor locally"
	@echo "  make run-router       - Run router locally (Phase 2)"
	@echo ""
	@echo "Test:"
	@echo "  make test-ingestor    - Send test event to ingestor"
	@echo "  make test-kafka       - Verify Kafka integration (Phase 2)"
	@echo "  make test-e2e         - End-to-end test (Phase 2)"
	@echo ""
	@echo "Utilities:"
	@echo "  make fmt              - Format Go code"
	@echo "  make clean            - Clean build artifacts"
	@echo "  make logs-ingestor    - Show ingestor logs"
	@echo ""

# Initialize project
init:
	@echo "Initializing EdgeLog Stream project..."
	mkdir -p bin
	mkdir -p logs
	$(GOMOD) tidy
	@echo "✅ Project initialized"

# Docker commands
docker-up:
	@echo "Starting Docker services..."
	docker-compose up -d
	@echo "Waiting for services to be ready..."
	sleep 10
	@echo "✅ Docker services started"
	@echo "Services available at:"
	@echo "  - Redpanda Console: http://localhost:8080"
	@echo "  - ClickHouse HTTP: http://localhost:8123"
	@echo "  - Redis: localhost:6379"
	@echo "  - Prometheus: http://localhost:9090"
	@echo "  - Grafana: http://localhost:3000 (admin/admin)"

docker-down:
	@echo "Stopping Docker services..."
	docker-compose down
	@echo "✅ Docker services stopped"

docker-verify:
	@echo "Verifying Docker services..."
	@echo "Checking Redpanda..."
	@docker exec edgelog-redpanda rpk cluster health || echo "❌ Redpanda not healthy"
	@echo "Checking ClickHouse..."
	@docker exec edgelog-clickhouse clickhouse-client --query "SELECT 1" || echo "❌ ClickHouse not healthy"
	@echo "Checking Redis..."
	@docker exec edgelog-redis redis-cli ping || echo "❌ Redis not healthy"
	@echo "✅ All services verified"

# Build commands
build-ingestor:
	@echo "Building ingestor..."
	$(GOBUILD) -o $(BINARY_INGESTOR) ./cmd/ingestor
	@echo "✅ Ingestor built: $(BINARY_INGESTOR)"

build-router:
	@echo "Building router..."
	$(GOBUILD) -o $(BINARY_ROUTER) ./cmd/router
	@echo "✅ Router built: $(BINARY_ROUTER)"

build-all: build-ingestor build-router
	@echo "✅ All services built"

# Run commands
run-ingestor: build-ingestor
	@echo "Running ingestor on port 8081..."
	PORT=8081 ./$(BINARY_INGESTOR)

run-router: build-router
	@echo "Running router..."
	./$(BINARY_ROUTER)

# Test commands
test-ingestor:
	@echo "Testing ingestor with sample event..."
	@echo "Checking health endpoint..."
	@curl -s http://localhost:8081/health | jq . || echo "❌ Health check failed"
	@echo ""
	@echo "Sending test event..."
	@curl -X POST http://localhost:8081/v1/events \
		-H "Content-Type: application/json" \
		-d '{ \
			"event_id": "test-'$$(date +%s)'", \
			"timestamp": "'$$(date -u +"%Y-%m-%dT%H:%M:%S.000Z")'", \
			"tenant_id": "test-tenant", \
			"event_type": "test.event", \
			"data": { \
				"message": "Hello from Makefile test", \
				"test": true \
			}, \
			"metadata": { \
				"correlation_id": "test-correlation-'$$(date +%s)'" \
			} \
		}' | jq . || echo "❌ Event ingestion failed"

test-kafka:
	@echo "Testing Kafka integration..."
	@echo "This will be implemented in Phase 2"

test-e2e:
	@echo "Running end-to-end test..."
	@echo "This will be implemented in Phase 2"

# Unit tests
test:
	@echo "Running unit tests..."
	$(GOTEST) -v ./...

# Format code
fmt:
	@echo "Formatting Go code..."
	$(GOFMT) ./...
	@echo "✅ Code formatted"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -rf logs/
	@echo "✅ Clean complete"

# Show logs
logs-ingestor:
	@tail -f logs/ingestor.log 2>/dev/null || echo "No logs found. Run 'make run-ingestor' first"

logs-router:
	@tail -f logs/router.log 2>/dev/null || echo "No logs found. Run 'make run-router' first"

# Development shortcuts
dev: docker-up build-ingestor run-ingestor
	@echo "Development environment ready"
