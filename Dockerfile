FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git gcc musl-dev sqlite-dev

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o court-data-fetcher cmd/server/main.go

FROM alpine:latest

RUN apk --no-cache add ca-certificates chromium chromium-chromedriver

RUN addgroup -g 1000 -S appuser && \
    adduser -u 1000 -S appuser -G appuser

WORKDIR /app

COPY --from=builder /app/court-data-fetcher .

COPY --from=builder /app/web ./web

RUN mkdir -p data && chown -R appuser:appuser /app

USER appuser

EXPOSE 8080

ENV ROD_BROWSER_PATH=/usr/bin/chromium-browser

CMD ["./court-data-fetcher"]