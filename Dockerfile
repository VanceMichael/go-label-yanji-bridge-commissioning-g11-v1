FROM golang:1.25.0-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
RUN go build -trimpath -ldflags="-s -w" -o /out/bridgewatch ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/* && useradd --system --uid 10001 --create-home bridgewatch && mkdir -p /data && chown bridgewatch:bridgewatch /data
COPY --from=build /out/bridgewatch /usr/local/bin/bridgewatch
USER bridgewatch
ENV BRIDGEWATCH_ADDR=:8080 BRIDGEWATCH_DATABASE=/data/bridgewatch.db
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/bridgewatch"]
