FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags="-s -w -X main.version=${VERSION}" -o /out/tatnet-mcp ./cmd/tatnet-mcp

FROM debian:bookworm-slim
# ca-certificates — сервер ходит в api.tatnet.ru по TLS.
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/tatnet-mcp /usr/local/bin/tatnet-mcp
EXPOSE 8080
CMD ["/usr/local/bin/tatnet-mcp"]
