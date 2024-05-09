# ==============================================================================
# Tasks
# [ ] CLI Layer
# 		[ ] Safety checks
# 			[ ] localhost destination is required without explicit opt in
#			[ ] Confirmation prompts
# 		[ ] CLI frameworks
#        [ ] charmed/bubbletea for
# 			[ ] ardanlabs/conf/v3 for cli flag, env var and args handling if bubbletea does not handle it
# 		[ ] Support passing in dynamic values/params, like {erno}
# [ ] Main
# 		[ ] Main run function
# 		[ ] Param validation
#		[X] concurrency setup
#		[X] Task struct
#		[X] Defer constraints
#		[X] set up a task per table
#		[ ] Support table and schema exclusion
#		[ ] Support omitting sensitive data columns
#		
# [ ] Table Sync
# 		[X] truncate support
# 		[X] defer constraints support
# 		[ ] preserve existing data support
# 		[ ] 
# [X] Create Reader Datasource Interface
#     [X] Version
#     [X] Get Schemas
#     [X] Get Tables
# [X] Create ReadWriter Datasource Interface 
# 		[X] truncate	
#     [X] update sequences
#     [X] create temp table
#     [X] delete 
#     [X] insert w/ on conflict support
# [ ] Schema Sync
#		[ ] Figure out how to use pg_dump and pg_restore to accomplish this, or use Copy?
#	
# Future
# 		* Batch support for large tables

# ==============================================================================
# Variables

PSQL_SOURCE_CMD := docker compose exec source_db psql -h localhost -U source_user -d postgres
PSQL_DEST_CMD   := docker compose exec dest_db psql -h localhost -U dest_user -d postgres


# ==============================================================================
# Dev Commands

test:
	go test -count=1 ./...

reset-docker-databases:
	docker compose down
	docker volume rm pggosync_source-db-data
	docker volume rm pggosync_dest-db-data
	docker compose up -d --force-recreate


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