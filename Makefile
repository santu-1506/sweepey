VERSION ?= 0.1.0
LDFLAGS := -s -w -X main.version=$(VERSION)
PKG := ./cmd/sweepr

.PHONY: build test vet dist clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/sweepr $(PKG)

test:
	go test ./...

vet:
	go vet ./...
	GOOS=windows go vet ./...
	GOOS=linux go vet ./...

# Cross-compile release binaries for every supported platform into dist/.
dist:
	GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/sweepr-darwin-arm64 $(PKG)
	GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/sweepr-darwin-amd64 $(PKG)
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/sweepr-windows-amd64.exe $(PKG)
	GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/sweepr-windows-arm64.exe $(PKG)
	GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/sweepr-linux-amd64 $(PKG)

clean:
	rm -rf bin dist
