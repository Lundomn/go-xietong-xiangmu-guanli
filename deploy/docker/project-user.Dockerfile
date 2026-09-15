FROM scratch
WORKDIR /app
COPY project-user/target/project-user /app/project-user
COPY deploy/config/project-user.yaml /app/config/config.yaml
ENTRYPOINT ["/app/project-user"]
