# Build context is the repository root (the image also ships /game-configs).
FROM golang:1.26-alpine AS build
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
COPY game-configs /game-configs
ENV GAME_CONFIG_DIR=/game-configs HTTP_ADDR=:8080
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/api"]
