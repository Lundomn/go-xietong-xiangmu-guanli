FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY project-api/target/project-api /app/project-api
COPY deploy/config/project-api.yaml /app/config/config.yaml
COPY deploy/upload/.keep /app/upload/.keep
ENTRYPOINT ["/app/project-api"]
