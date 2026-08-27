# syntax=docker/dockerfile:1.7
ARG GO_VERSION=1.25
ARG ALPINE_VERSION=3.21

FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS build
WORKDIR /src

RUN apk add --no-cache \
    build-base=0.5-r3 \
    sqlite-dev=3.48.0-r4

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
ENV CGO_ENABLED=1
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/opencode-status ./cmd/opencode-status

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/opencode-status /usr/local/bin/opencode-status
USER 1000:1000
WORKDIR /data
VOLUME ["/data"]
ENV OPENCODE_STATUS_DB=/data/history.db
ENV OPENCODE_STATUS_WEB=:8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/opencode-status"]
CMD ["--no-tui", "--db", "/data/history.db", "--web", ":8080"]
