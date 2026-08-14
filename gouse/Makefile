.DEFAULT_GOAL := help
SHELL := /bin/bash

MODULE  := github.com/fashion-commerce/platform
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

GREEN := \033[0;32m
RESET := \033[0m

.PHONY: help
help: ## Hiển thị danh sách lệnh
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# ---------------------------------------------------------------- Phát triển

.PHONY: run
run: ## Chạy API server
	go run -ldflags "$(LDFLAGS)" ./cmd/api

.PHONY: build
build: ## Build binary vào bin/
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/api ./cmd/api
	go build -ldflags "$(LDFLAGS)" -o bin/worker ./cmd/worker 2>/dev/null || true
	go build -o bin/archcheck ./cmd/archcheck
	@echo -e "$(GREEN)✓ Đã build vào bin/$(RESET)"

.PHONY: fmt
fmt: ## Định dạng code
	gofmt -w .

.PHONY: tidy
tidy: ## Dọn go.mod
	go mod tidy

# ---------------------------------------------------------------- Kiểm tra

.PHONY: test
# -p 1 là BẮT BUỘC, không phải tùy chọn.
#
# Các gói test tích hợp dùng CHUNG một database và TRUNCATE bảng ở bước
# dọn dẹp. Chạy song song thì gói này xóa dữ liệu của gói kia đang chạy,
# và test đỏ ngẫu nhiên theo thứ tự lịch trình — loại lỗi tốn nhiều giờ
# nhất để tìm ra.
#
# Cách bỏ được -p 1: mỗi gói test dùng schema hoặc database riêng. Chưa
# làm vì chi phí lớn hơn lợi ích ở quy mô hiện tại.
test: ## Chạy toàn bộ test
	go test -p 1 ./...

.PHONY: test-v
test-v: ## Chạy test có chi tiết
	go test -p 1 -v ./...

.PHONY: test-race
test-race: ## Chạy test với race detector
	go test -race ./...

.PHONY: cover
cover: ## Báo cáo độ phủ test
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1
	@echo "Xem chi tiết: go tool cover -html=coverage.out"

.PHONY: vet
vet: ## Phân tích tĩnh của Go
	go vet ./...

.PHONY: fmt-check
fmt-check: ## Kiểm tra định dạng (dùng trong CI)
	@out=$$(gofmt -l . | grep -v '^$$' || true); \
	if [ -n "$$out" ]; then \
		echo "Các file chưa định dạng:"; echo "$$out"; \
		echo "Chạy: make fmt"; exit 1; \
	fi
	@echo -e "$(GREEN)✓ Định dạng đúng$(RESET)"

.PHONY: arch
arch: ## Kiểm tra ranh giới module (VI PHẠM = THẤT BẠI)
	@go run ./cmd/archcheck

.PHONY: api-lint
api-lint: ## Kiểm tra đặc tả OpenAPI
	npx --yes @redocly/cli@latest lint api/openapi.yaml

.PHONY: check
check: fmt-check vet arch test ## Chạy toàn bộ kiểm tra như CI
	@echo -e "$(GREEN)✓ Mọi kiểm tra đều đạt$(RESET)"

# ---------------------------------------------------------------- Database

# DSN mặc định trỏ tới database phát triển cục bộ.
# Ghi đè bằng: make migrate-up DATABASE_URL=postgres://...
DATABASE_URL ?= postgres://postgres@127.0.0.1:5432/gouse?sslmode=disable

.PHONY: migrate-up
migrate-up: ## Áp dụng toàn bộ migration
	migrate -path migrations -database "$(DATABASE_URL)" up

.PHONY: migrate-down
migrate-down: ## Lùi MỘT migration
	migrate -path migrations -database "$(DATABASE_URL)" down 1

.PHONY: migrate-reset
migrate-reset: ## XÓA TOÀN BỘ bảng rồi tạo lại (chỉ dùng khi phát triển)
	migrate -path migrations -database "$(DATABASE_URL)" down -all
	migrate -path migrations -database "$(DATABASE_URL)" up

.PHONY: migrate-version
migrate-version: ## Xem phiên bản migration hiện tại
	@migrate -path migrations -database "$(DATABASE_URL)" version

.PHONY: migrate-new
migrate-new: ## Tạo migration mới: make migrate-new NAME=ten_migration
	@test -n "$(NAME)" || { echo "Thiếu NAME. Ví dụ: make migrate-new NAME=inventory"; exit 1; }
	migrate create -ext sql -dir migrations -seq $(NAME)

.PHONY: test-db
test-db: ## Chạy test có dùng database thật
	DATABASE_URL="$(DATABASE_URL)" go test ./... -count=1

# ---------------------------------------------------------------- Tiện ích

.PHONY: api-types
api-types: ## Sinh kiểu TypeScript từ đặc tả OpenAPI
	npx --yes openapi-typescript@latest api/openapi.yaml -o api/generated/types.ts

.PHONY: clean
clean: ## Xóa file build
	rm -rf bin coverage.out
