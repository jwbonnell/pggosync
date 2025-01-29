# ==============================================================================
# Tasks
# [ ] CLI Layer
# 		[ ] Safety checks
# 			[ ] localhost destination is required without explicit opt in
#			[ ] Confirmation prompts
# 		[ ] CLI frameworks
#        [ ] charmed/bubbletea for TUI
# 		[ ] Support passing in dynamic values/params, like {erno}
# [ ] Main
#		[ ] Support table and schema exclusion
#		[ ] Support omitting sensitive data columns
# 		[X] Disable triggers support, maybe needed for the AH scenario where set_modified_by was preventing upserts from succeeding
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

VERSION=0.1.0-prerelease
PSQL_SOURCE_CMD := docker compose exec source_db psql -h localhost -U source_user -d postgres
PSQL_DEST_CMD   := docker compose exec dest_db psql -h localhost -U dest_user -d postgres


# ==============================================================================
# Dev Commands

test:
	go test -count=1 -v ./... | sed ''/PASS/s//$(printf "\033[32mPASS\033[0m")/'' | sed ''/FAIL/s//$(printf "\033[31mFAIL\033[0m")/''

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
	go build -ldflags "-s -w -X main.build=$(VERSION)" -o ./bin/pggosync main.go

# ==============================================================================
# CLI Commands

pggosync_truncate:
	go run main.go sync --group country_var_1:2 --truncate

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