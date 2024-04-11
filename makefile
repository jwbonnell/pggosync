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