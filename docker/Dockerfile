# fs_api + the compiled fs_ui in one image. Build from the repo root (the
# context needs fs_ui/ and fs_api/; ignores live in Dockerfile.dockerignore):
#   docker build -f docker/Dockerfile -t cuttlefish-api .
# The config is not baked in: mount fs_config.yml at /app/fs_config.yml. With
# every secret written as ${VAR} and supplied via env it can be a ConfigMap;
# a config holding literal secrets must be mode 0440 or stricter.

FROM node:24-alpine AS ui
WORKDIR /src
COPY fs_ui/package.json fs_ui/package-lock.json ./
RUN npm ci
COPY fs_ui/ ./
RUN npm run build

FROM golang:1.27 AS api
WORKDIR /src
COPY fs_api/go.mod fs_api/go.sum ./
RUN go mod download
COPY fs_api/ ./
# Swagger docs are committed, so no swag step here.
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/fs_api .

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=api /out/fs_api /app/fs_api
COPY --from=ui /src/build /app/ui/build
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/fs_api"]
