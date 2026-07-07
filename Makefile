# Yata — Linux/macOS build
.PHONY: build web backend run dev docker clean

build: web backend

web:
	cd web && npm install && npm run build

backend:
	go build -o yata ./cmd/yata

run: build
	./yata

# Hot-reload development: Go backend on :8420, Vite dev server on :5173
dev:
	@echo "Run these in two terminals:"
	@echo "  go run ./cmd/yata"
	@echo "  cd web && npm run dev"

docker:
	docker build -t yata .

clean:
	rm -f yata yata.exe static/dashboard.js
