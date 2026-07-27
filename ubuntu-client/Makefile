.PHONY: fmt test vet build build-addon package clean

fmt:
	gofmt -w cmd internal

test:
	go test ./...

vet:
	go vet ./...

build:
	mkdir -p dist
	CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o dist/fyderhythm-sync ./cmd/fyderhythm-sync

build-addon:
	cmake -S fcitx-addon -B dist/fcitx-addon-build -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX=/usr
	cmake --build dist/fcitx-addon-build --parallel

package: build build-addon
	bash scripts/build-deb.sh

clean:
	rm -rf dist
