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
# Chạy SONG SONG được vì mỗi gói test có database riêng.
#
# Trước đây target này phải dùng `-p 1`: các gói dùng chung một database và
# TRUNCATE bảng khi dọn dẹp, nên gói này xóa dữ liệu gói kia đang chạy dở.
# Nay internal/platform/testdb cấp cho mỗi gói một database sao từ khuôn, và
# cờ đó không còn cần.
#
# Cần chạy `make test-db` MỘT LẦN trước. Thiếu TEST_DATABASE_URL thì test
# cần database sẽ tự bỏ qua chứ không chạy nhầm lên database phát triển.
test: ## Chạy toàn bộ test
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test ./...

.PHONY: test-v
test-v: ## Chạy test có chi tiết
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -v ./...

.PHONY: test-race
test-race: ## Chạy test với race detector
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -race ./...

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

# Database KHUÔN cho test. Mỗi gói test tự sao một bản riêng từ đây.
#
# CỐ TÌNH khác DATABASE_URL: dùng chung thì một lần `go test ./...` là xóa
# sạch dữ liệu phát triển, và không có gì báo cho tới khi mở giao diện thấy
# trống. internal/platform/testdb có hàng rào chặn hai biến trỏ cùng chỗ.
#
# Đổi máy chủ thì phải đổi CẢ HAI biến dưới: TEST_ADMIN_URL là nơi chạy lệnh
# CREATE DATABASE, nên nó phải trỏ cùng máy chủ với khuôn.
TEST_DATABASE_URL ?= postgres://postgres@127.0.0.1:5432/gouse_test?sslmode=disable
TEST_ADMIN_URL    ?= postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable

.PHONY: test-db
test-db: ## Tạo database KHUÔN cho test — chạy một lần, và sau mỗi migration mới
	@psql "$(TEST_ADMIN_URL)" -q \
		-c 'DROP DATABASE IF EXISTS gouse_test' \
		-c 'CREATE DATABASE gouse_test'
	@migrate -path migrations -database "$(TEST_DATABASE_URL)" up
	@echo "khuôn gouse_test đã sẵn sàng — chạy 'make test'"

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

# ---------------------------------------------------------------- Tiện ích

.PHONY: api-types
api-types: ## Sinh kiểu TypeScript từ đặc tả OpenAPI
	npx --yes openapi-typescript@latest api/openapi.yaml -o api/generated/types.ts

.PHONY: clean
clean: ## Xóa file build
	rm -rf bin coverage.out
