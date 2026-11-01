.PHONY: build test run run-ch ch-up ch-down ch-sql docker

build:
	go build -o bin/screener ./cmd/screener
	go build -o bin/backfill ./cmd/backfill
	go build -o bin/backtest ./cmd/backtest

test:
	go vet ./... && go test ./...

run:
	go run ./cmd/screener -log debug

run-ch:
	go run ./cmd/screener -clickhouse clickhouse://default:screener@localhost:9000/screener -ch-optional

ch-up:
	docker compose up -d --wait clickhouse

ch-down:
	docker compose down

ch-sql:
	docker compose exec clickhouse clickhouse-client --database screener

docker:
	docker build -t screener:local .

backfill:
	go run ./cmd/backfill -clickhouse clickhouse://default:screener@localhost:9000/screener -top 150 -days 180

backtest:
	go run ./cmd/backtest -clickhouse clickhouse://default:screener@localhost:9000/screener -out backtest-report.md

widget:
	cd widget/sandbox && npm install && npm run dev
