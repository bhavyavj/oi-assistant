FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -ldflags="-s -w" -o /oi-assistant ./cmd/server

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /oi-assistant .
COPY configs/ configs/
EXPOSE 8080
ENTRYPOINT ["/app/oi-assistant"]
