#!/bin/bash
set -e

echo "Applying TimescaleDB performance tuning..."

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    ALTER SYSTEM SET work_mem = '8MB';
    ALTER SYSTEM SET maintenance_work_mem = '128MB';
    ALTER SYSTEM SET max_wal_size = '2GB';
    ALTER SYSTEM SET min_wal_size = '80MB';
    ALTER SYSTEM SET checkpoint_completion_target = '0.9';
    ALTER SYSTEM SET wal_buffers = '16MB';
    ALTER SYSTEM SET default_statistics_target = '100';
    ALTER SYSTEM SET random_page_cost = '1.1';
    ALTER SYSTEM SET effective_io_concurrency = '200';
    ALTER SYSTEM SET max_parallel_workers_per_gather = '2';
    ALTER SYSTEM SET max_parallel_maintenance_workers = '2';
    ALTER SYSTEM SET max_parallel_workers = '4';
    ALTER SYSTEM SET parallel_setup_cost = '1000';
    ALTER SYSTEM SET parallel_tuple_cost = '0.1';
    ALTER SYSTEM SET timescaledb.max_background_workers = '8';

    SELECT pg_reload_conf();
EOSQL

echo "TimescaleDB tuning applied successfully."
