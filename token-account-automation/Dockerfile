FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /token-account-automation ./cmd/server

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /token-account-automation /app/token-account-automation
EXPOSE 8091
ENTRYPOINT ["/app/token-account-automation"]
