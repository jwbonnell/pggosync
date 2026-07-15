# ==============================================================================
# Tasks
# [ ] CLI Layer
# 		[ ] CLI frameworks
#        [ ] charmed/bubbletea for TUI
# 		[ ] Support passing in dynamic values/params, like {erno}
# [ ] Main
#		[ ] Support table and schema exclusion
#		[ ] Support omitting sensitive data columns
# 		[ ] Add support for configurable SSL/TLS support for PG connections
#		

# [ ] Bugs
# 		[ ] Test and fix table syncing + where clause
# 		[ ] Better handle excluded table detection, currently tables not in the list are getting picked up
# 		[ ] Possible remove 'sync' command and just make it the default
# 		[ ] no-safety flag is missing from flag list
# 		[ ] clean up flags, aliases and descriptions

# [ ] Prepared Statements
# [ ] Delete with filter support
# [ ] Schema Sync
#		[ ] Figure out how to use pg_dump and pg_restore to accomplish this, or use Copy?
# [ ] ERRORS
# 		[ ] Update goroutines to handle error appropriately using select or something - sync.go
# Future
# 		* Batch support for large tables

# ==============================================================================
# Variables

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo development)
PSQL_SOURCE_CMD := docker compose exec source_db psql -h localhost -U source_user -d postgres
PSQL_DEST_CMD   := docker compose exec dest_db psql -h localhost -U dest_user -d postgres


# ==============================================================================
# Dev Commands

up:
	docker compose up -d
	docker compose logs -f

down:
	docker compose down

docker-build:
	docker compose build

rebuild: down docker-build up

test:
	go test -count=1 ./... | sed ''/PASS/s//$(printf "\033[32mPASS\033[0m")/'' | sed ''/FAIL/s//$(printf "\033[31mFAIL\033[0m")/''

test-short:
	go test -count=1 -v ./... -short  | sed ''/PASS/s//$(printf "\033[32mPASS\033[0m")/'' | sed ''/FAIL/s//$(printf "\033[31mFAIL\033[0m")/''

reset-docker-databases:
	docker compose down
	docker volume rm pggosync_source-db-data
	docker volume rm pggosync_dest-db-data
	docker compose up -d --force-recreate

# ==============================================================================
# Build Commands
# ==============================================================================

build:
	GOARCH=amd64 go build -ldflags "-s -w -X main.build=$(VERSION)" -o ./bin/amd64/pggosync main.go
	GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w -X main.build=$(VERSION)" -o ./bin/arm64/pggosync main.go

# ==============================================================================
# CLI Commands
#
# dev-* targets are throwaway helpers for exercising pggosync against the local
# Docker databases. They all use the default "source"/"dest" connections and the
# checked-in sync configs (_configs/configs/default.ym, _configs/configs/manual.yml).

# Shared run invocation: default connections against the local Docker databases.
DEV_RUN := go run main.go run --source source --dest dest

dev-init:
	go run main.go conn init source
	go run main.go conn init dest

# Schema sync: copy the whole DDL (pg_dump | psql) into the destination — the "create
# the structure first" step before any data sync. Default create-missing path (existing
# objects are skipped). Add --dry-run to preview the DDL, or --clean to drop & recreate.
dev-schema-sync: dev-init
	go run main.go schema sync --source source --dest dest

# Truncate path: wipe destination tables, then COPY straight from source.
dev-truncate: dev-init
	$(DEV_RUN) --config ./_configs/configs/default.ym --group country_var_1:2 --truncate

# Upsert path (default): temp table + INSERT ... ON CONFLICT DO UPDATE.
dev-upsert: dev-init
	$(DEV_RUN) --config ./_configs/configs/default.ym --group country

# Preserve path: INSERT ... ON CONFLICT DO NOTHING (existing rows untouched).
dev-preserve: dev-init
	$(DEV_RUN) --config ./_configs/configs/default.ym --group country_preserve --preserve

# Dry run: resolve tasks and stream, but roll back instead of committing.
dev-dry-run: dev-init
	$(DEV_RUN) --config ./_configs/configs/default.ym --group country --dry-run

# Ad-hoc single table with an inline WHERE filter, no config needed.
dev-table: dev-init
	$(DEV_RUN) --config ./_configs/configs/default.ym --table public.city:country_id=10 --truncate

# Inline scrub rules applied to columns as SQL expressions on the source side.
dev-scrub: dev-init
	$(DEV_RUN) --config manual --table public.employee:'active = true':name=redact,role=null --truncate

# Multi-schema cross-schema FK chain, deferring constraints on the destination.
dev-defer: dev-init
	$(DEV_RUN) --config manual --group store_inventory:10 --defer-constraints --truncate

# Disable user triggers on the destination for the duration of the sync.
dev-triggers: dev-init
	$(DEV_RUN) --config manual --group employee_org --disable-triggers --truncate

# Self-referential FK / composite PK / JSONB coverage from the catalog schema.
dev-catalog: dev-init
	$(DEV_RUN) --config manual --group product_catalog:1

# Concurrent source pre-fetch across multiple groups.
dev-concurrency: dev-init
	$(DEV_RUN) --config manual --group product_catalog:1 --group product_reviews:5 --concurrency 4

# Diff which tables are shared / missing between source and destination.
dev-tables: dev-init
	go run main.go tables --source source --dest dest

# ==============================================================================
# Database commands

# Source
psql-source:
	$(PSQL_SOURCE_CMD)

psql-source-version:
	$(PSQL_SOURCE_CMD) -c 'SELECT VERSION();'

psql-source-city:
	$(PSQL_SOURCE_CMD) -c 'SELECT * FROM public.city;'

psql-source-summary:
	$(PSQL_SOURCE_CMD) -c 'SELECT * FROM summary_vw;'

# Destination
psql-dest:
	$(PSQL_DEST_CMD)

psql-dest-version:
	$(PSQL_DEST_CMD) -c 'SELECT VERSION();'

psql-dest-city:
	$(PSQL_DEST_CMD) -c 'SELECT * FROM public.city;'

psql-dest-summary:
	$(PSQL_DEST_CMD) -c 'SELECT * FROM summary_vw;'

psql-dest-ndc:
	$(PSQL_DEST_CMD) -c "SELECT table_schema AS schema, table_name AS table, constraint_name FROM information_schema.table_constraints WHERE constraint_type = 'FOREIGN KEY' AND is_deferrable = 'NO';"

psql-dest-triggers:
	$(PSQL_DEST_CMD) -c "SELECT tgname AS name, tgisinternal AS internal, tgenabled != 'D' AS enabled, tgconstraint != 0 AS integrity FROM pg_trigger;"