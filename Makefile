GO ?= go
.PHONY: build web test linux demo clean
web:
	cd web && npm ci && npm run build
build: web
	$(GO) build -trimpath -o bin/anker ./cmd/anker
linux: web
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -o bin/anker-linux-amd64 ./cmd/anker
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -o bin/anker-linux-arm64 ./cmd/anker
test:
	$(GO) test -race ./...
	python3 -m unittest discover -s host -p '*test*.py'
	cd web && npm run build && npm test
demo: build
	./bin/anker --data ./var/demo demo
clean:
	rm -rf bin web/test-results web/playwright-report
