FROM golang:1.23-alpine AS build
WORKDIR /src
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=$GOPROXY
RUN apk add --no-cache ca-certificates git
COPY go.mod ./
RUN go mod download
COPY . .
RUN go mod tidy && \
    CGO_ENABLED=0 go test ./... && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/fyderhythm-sync ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S -g 10001 app && \
    adduser -S -D -H -u 10001 -G app app
WORKDIR /app
COPY --from=build /out/fyderhythm-sync /usr/local/bin/fyderhythm-sync
RUN mkdir -p /data && chown -R 10001:10001 /data
USER 10001:10001
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/fyderhythm-sync"]
