FROM golang:1.23-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go test ./... && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/fyderhythm-sync ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=build /out/fyderhythm-sync /usr/local/bin/fyderhythm-sync
RUN mkdir -p /data && chown -R app:app /data
USER app
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/fyderhythm-sync"]
