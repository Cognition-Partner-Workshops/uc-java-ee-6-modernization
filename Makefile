# DayTrader 3 (Java EE 6) "before" estate: build, run on Liberty in a container, record and check parity.
# Usage: make up && make build && make run && make parity TARGET=http://localhost:9083/daytrader

LIBERTY_IMAGE ?= icr.io/appcafe/websphere-liberty:full-java8-ibmjava-ubi
CONTAINER     ?= daytrader3-liberty
PORT          ?= 9083
BASE_URL      ?= http://localhost:$(PORT)/daytrader
TARGET        ?= $(BASE_URL)
SEED          ?= 42
VENV          ?= .venv
PY            := $(VENV)/bin/python

# The POMs target Java 7 (net.wasdev.maven.parent:java7-parent). Build with JDK 8 when it is installed locally.
JDK8_HOME     ?= $(firstword $(wildcard /usr/lib/jvm/java-8-openjdk-amd64 /usr/lib/jvm/temurin-8-jdk-amd64))
MVN           := $(if $(JDK8_HOME),JAVA_HOME=$(JDK8_HOME) )mvn -B -DskipFVT=true

EAR           := $(CURDIR)/daytrader3-ee6/target/daytrader3-ee6-1.0-SNAPSHOT.ear
SERVER_XML    := $(CURDIR)/daytrader3-ee6-wlpcfg/servers/daytrader3_Sample/server.xml
DERBY_DIR     := $(CURDIR)/daytrader3-ee6-wlpcfg/shared/resources/Daytrader3_SampleDerbyLibs

.PHONY: up build test run seed record parity inventory down logs

up: ## Pull the Liberty image and install the parity harness's Python deps
	docker pull $(LIBERTY_IMAGE)
	test -x $(PY) || python3 -m venv $(VENV)
	$(PY) -m pip install -q -r parity/requirements.txt

build: ## Build the EAR (runs the JUnit 5 unit tests too)
	$(MVN) clean package

test: ## Unit tests only
	$(MVN) test -pl daytrader3-ee6-ejb

run: ## Start Liberty with the repo's server.xml, then create and populate the Derby DB
	-docker rm -f $(CONTAINER) >/dev/null 2>&1
	docker run -d --name $(CONTAINER) -p $(PORT):9083 \
	  -v $(SERVER_XML):/config/server.xml:ro \
	  -v $(EAR):/config/apps/daytrader3-ee6.ear:ro \
	  -v $(DERBY_DIR):/opt/ibm/wlp/usr/shared/resources/Daytrader3_SampleDerbyLibs:ro \
	  $(LIBERTY_IMAGE)
	$(PY) parity/seed.py --target $(BASE_URL)

seed: ## Re-create and re-populate the DB on a running server
	$(PY) parity/seed.py --target $(TARGET) --skip-wait

record: ## Re-record the reference transcripts from the legacy app (only from the Liberty baseline)
	$(PY) parity/record.py --target $(BASE_URL) --seed $(SEED)

parity: ## Replay all scenarios against TARGET and diff against parity/transcripts (non-zero exit on any diff)
	$(PY) parity/compare.py --target $(TARGET)

inventory: ## Regenerate docs/WEBSPHERE_INVENTORY.md
	python3 tools/ws_inventory.py

logs:
	docker logs --tail 200 $(CONTAINER)

down:
	-docker rm -f $(CONTAINER)

# --- Go + PostgreSQL target (daytrader-go/) -----------------------------------------------------
GO          ?= $(firstword $(shell command -v go) /usr/local/go/bin/go)
GO_PORT     ?= 8080
GO_DB_URL   ?= postgres://daytrader:daytrader@localhost:55432/daytrader?sslmode=disable

.PHONY: go-db go-build go-test go-seed go-run go-down

go-db: ## Start PostgreSQL for the Go target (port 55432)
	docker compose -f daytrader-go/compose.yaml up -d --wait

go-build: ## go vet + build daytrader-go/bin/daytrader
	cd daytrader-go && $(GO) vet ./... && CGO_ENABLED=0 $(GO) build -o bin/daytrader ./cmd/daytrader

go-test: ## go vet + go test
	cd daytrader-go && $(GO) vet ./... && $(GO) test ./...

go-seed: go-build go-db ## buildDBTables + buildDB on PostgreSQL
	DATABASE_URL='$(GO_DB_URL)' daytrader-go/bin/daytrader seed

go-run: go-seed ## Seed, then serve the Go target on :8080 in the background (log: work/daytrader-go.log)
	@mkdir -p work
	@-pkill -x daytrader 2>/dev/null; sleep 1
	DATABASE_URL='$(GO_DB_URL)' LISTEN_ADDR=:$(GO_PORT) nohup daytrader-go/bin/daytrader serve > work/daytrader-go.log 2>&1 &
	@sleep 2 && curl -fsS http://localhost:$(GO_PORT)/daytrader/health && echo

go-down: ## Stop the Go target and its PostgreSQL
	-pkill -x daytrader
	docker compose -f daytrader-go/compose.yaml down -v
