.PHONY: fmt test vet build package

fmt:
	gofmt -w cmd internal

test:
	go test ./...

vet:
	go vet ./...

build:
	mkdir -p dist
	CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o dist/fyderhythm-sync ./cmd/fyderhythm-sync

package: build
	bash scripts/build-deb.sh
