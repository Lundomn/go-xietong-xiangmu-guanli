FROM scratch
WORKDIR /app
COPY project-project/target/project-project /app/project-project
COPY deploy/config/project-project.yaml /app/config/config.yaml
ENTRYPOINT ["/app/project-project"]
