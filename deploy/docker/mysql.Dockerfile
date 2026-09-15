FROM mysql:8.0.42

COPY deploy/mysql/init/001_schema.sql /docker-entrypoint-initdb.d/001_schema.sql
