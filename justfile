set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

test:
    GOCACHE=${GOCACHE:-/tmp/project-datascape-go-cache} go test ./...

race:
    GOCACHE=${GOCACHE:-/tmp/project-datascape-go-cache} go test -race ./...

build:
    GOCACHE=${GOCACHE:-/tmp/project-datascape-go-cache} CGO_ENABLED=0 go build -trimpath -buildvcs=false -o bin/platformctl ./cmd/platformctl

validate-examples:
    GOCACHE=${GOCACHE:-/tmp/project-datascape-go-cache} go run ./cmd/platformctl validate --platform examples/change-stream/platform.yaml --profile profiles/local.yaml

generate-example:
    GOCACHE=${GOCACHE:-/tmp/project-datascape-go-cache} go run ./cmd/platformctl generate --platform examples/change-stream/platform.yaml --profile profiles/local.yaml --output dist/local

docs-check:
    test -s docs/index.md
    test -s docs/specification/reference.md
    test -s docs/migration/resource-model-v1alpha1.md

docs: build
    rm -rf dist/docs
    ./bin/platformctl docs build --source docs --output dist/docs
    test -s dist/docs/index.html
    test -s dist/docs/reference/api.html

docs-serve: docs
    ./bin/platformctl docs serve --directory dist/docs --listen 127.0.0.1:8000

reference-generate: build
    rm -rf dist/reference
    ./bin/platformctl generate --platform examples/reference-lakehouse/platform.yaml --profile profiles/reference.yaml --output dist/reference
    cp -R examples/reference-lakehouse/jobs dist/reference/jobs
    mkdir -p dist/reference/state/sqlite
    ./bin/platformctl secrets init --bundle dist/reference --development

reference-up: reference-generate
    cd dist/reference && docker compose --env-file .env --profile governance up -d --wait
    cd dist/reference && for attempt in $(seq 1 60); do if docker compose --env-file .env exec -T attendance-changes rpk topic describe attendance-changes --format json | grep -Eq '"high_watermark":[[:space:]]*[1-9]'; then exit 0; fi; sleep 2; done; echo "CDC did not publish attendance events within 120 seconds" >&2; exit 1
    cd dist/reference && docker compose --env-file .env --profile governance exec -T lakehouse-pipeline /opt/spark/bin/spark-submit --packages org.apache.spark:spark-sql-kafka-0-10_2.12:3.5.5 /opt/datascape/jobs/medallion.py
    just reference-verify

reference-verify:
    ./bin/platformctl verify --bundle dist/reference --runtime
    cd dist/reference && docker compose --env-file .env exec -T attendance-changes rpk cluster health -X brokers=localhost:9092 | grep -E 'Healthy:.+true'
    cd dist/reference && docker compose --env-file .env exec -T cdc-connector curl -fsS http://localhost:8083/connectors/attendance-source/status | grep -Eq '"state"[[:space:]]*:[[:space:]]*"RUNNING"'
    cd dist/reference && docker compose --env-file .env exec -T query-engine trino --output-format TSV --execute "SELECT count(*) FROM iceberg.education.school_daily_attendance_summary" | grep -Eq '^[1-9][0-9]*$'

reference-logs:
    cd dist/reference && docker compose --env-file .env --profile governance logs -f --tail=200

reference-down:
    cd dist/reference && docker compose --env-file .env --profile governance down

reference-reset:
    cd dist/reference && docker compose --env-file .env --profile governance down --volumes --remove-orphans
    rm -rf dist/reference

test-integration:
    GOCACHE=${GOCACHE:-/tmp/project-datascape-go-cache} go test -tags=integration ./...

terminology-check:
    ./scripts/terminology-check.sh

determinism-check:
    rm -rf /tmp/project-datascape-det-a /tmp/project-datascape-det-b
    GOCACHE=${GOCACHE:-/tmp/project-datascape-go-cache} go run ./cmd/platformctl generate --platform examples/change-stream/platform.yaml --profile profiles/local.yaml --output /tmp/project-datascape-det-a
    GOCACHE=${GOCACHE:-/tmp/project-datascape-go-cache} go run ./cmd/platformctl generate --platform examples/change-stream/platform.yaml --profile profiles/local.yaml --output /tmp/project-datascape-det-b
    diff -ru /tmp/project-datascape-det-a /tmp/project-datascape-det-b

conformance:
    GOCACHE=${GOCACHE:-/tmp/project-datascape-go-cache} go run ./cmd/platformctl conformance >/tmp/project-datascape-conformance.json
