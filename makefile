# ==============================================================================
# Tasks
# [ ] CLI Layer
# 		[ ] Safety checks
# 			[ ] localhost destination is required without explicit opt in
#			[ ] Confirmation prompts
# 		[ ] CLI frameworks
#        [ ] charmed/bubbletea for
# 			[ ] ardanlabs/conf/v3 for cli flag, env var and args handling if bubbletea does not handle it
# 		[ ] 
# [ ] Table Sync
# 		[ ] truncate support
# 		[ ] defer constraints support
# 		[ ] preserve existing data support
# 		[ ] 
# [ ] Create Reader Datasource Interface
#     [ ] Query
#     [X] Version
#     [X] Get Schemas
#     [X] Get Tables
# [ ] Create ReadWriter Datasource Interface 
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

reset-docker-databases:
	docker compose down
	docker volume rm pggosync_source-db-data
	docker volume rm pggosync_dest-db-data
	docker compose up -d --force-recreate


# ==============================================================================
# Database commands

psql-source:
	$(PSQL_SOURCE_CMD)

psql-source-version:
	$(PSQL_SOURCE_CMD) -c 'SELECT VERSION();'

psql-source-city:
	$(PSQL_SOURCE_CMD) -c 'SELECT * FROM public.city;'

psql-dest:
	$(PSQL_DEST_CMD)

psql-dest-version:
	$(PSQL_DEST_CMD) -c 'SELECT VERSION();'

psql-dest-city:
	$(PSQL_DEST_CMD) -c 'SELECT * FROM public.city;'