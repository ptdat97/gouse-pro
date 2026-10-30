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
	@go test ./cmd/archcheck/

# Hợp đồng API và EVENT.
#
# # Vì sao mục tiêu này phải tồn tại
#
# `apicheck` và `eventcheck` được dựng để "chặn ở CI", và tài liệu ở ba chỗ
# nói đúng như thế (dependency-rules.md, api-first.md, backlog P3-56 ·
# P3-58 · P3-70 · P3-76). Nhưng CI thật chỉ gọi `make fmt-check`, `make
# vet`, `make arch`, `make test` — và `arch` chỉ chạy `archcheck`.
#
# Tức hai công cụ ấy CHƯA BAO GIỜ chạy trong CI. Phát hiện 25/09/2026, và
# đúng dạng lỗi hay gặp nhất của dự án này: một thứ được tin là đang chạy
# mà không chạy. Lần này thứ được tin là "hợp đồng của chúng ta có người
# gác".
#
# Chạy cả TEST của từng công cụ: một công cụ bỏ sót vi phạm còn nguy hiểm
# hơn không có công cụ.
.PHONY: hopdong
hopdong: ## Kiểm hợp đồng API và event (VI PHẠM = THẤT BẠI)
	@go run ./cmd/apicheck -v
	@go test ./cmd/apicheck/
	@go run ./cmd/eventcheck -v
	@go test ./cmd/eventcheck/

# Chặn BINARY lọt vào repo.
#
# # Vì sao mục tiêu này phải tồn tại
#
# `go build ./cmd/x` ghi ra `./x` ngay tại thư mục chạy lệnh, trùng tên
# với thư mục lệnh. Ba lần đã lọt vào lịch sử: taonhaban, taotaikhoan,
# rồi deadcol — mỗi file vài MB và không thể xóa khỏi lịch sử nữa.
#
# Hai .gitignore có một DANH SÁCH TÊN để chặn việc đó, và danh sách ấy
# thiếu đúng lệnh mới viết, mỗi lần. Danh sách không bao giờ đầy; phép
# ĐO thì đầy. Mục tiêu này đo: nó hỏi `file` xem từng tệp ĐANG ĐƯỢC
# THEO DÕI có phải mã máy hay không, nên nó bắt cả lệnh chưa ai đặt tên.
#
# Chạy trên toàn repo chứ không riêng `gouse/`: binary cũng rơi được ra
# gốc monorepo, nơi .gitignore của gouse/ không với tới.
#
# BẪY, đã cắn một lần ngay khi viết mục tiêu này: `file` ĐỆM TÊN TỆP bằng
# dấu cách khi nhận nhiều tệp, nên đầu ra là `tên:<nhiều dấu cách>kiểu`.
# Biểu thức đòi `: application/x-...` với ĐÚNG MỘT dấu cách khớp 0 dòng,
# và hàng rào báo "đạt" trong khi `gouse/deadcol` đang nằm trong repo.
#
# Hàng rào nào cũng phải được kiểm bằng cách PHÁ: cho nó một vi phạm
# thật và xem nó có đỏ không. Hàng rào này đã được kiểm như vậy.
.PHONY: nhiphan
nhiphan: ## Chặn mã máy lọt vào repo (VI PHẠM = THẤT BẠI)
	@root=$$(git rev-parse --show-toplevel); \
	bad=$$(cd "$$root" && git ls-files -z | xargs -0 file --mime-type \
		| grep -E '[[:space:]]application/x-(mach-binary|executable|pie-executable|sharedlib)$$' \
		|| true); \
	if [ -n "$$bad" ]; then \
		echo "$$bad"; \
		echo ""; \
		echo "Có MÃ MÁY trong repo. 'go build ./cmd/x' ghi ra ./x tại chỗ."; \
		echo "Xóa khỏi theo dõi: git rm --cached <tệp>"; \
		exit 1; \
	fi

.PHONY: deadcol
deadcol: ## Tìm cột KHÔNG AI GHI — cần DATABASE_URL có DỮ LIỆU THẬT
	@echo "deadcol đo trên DỮ LIỆU, nên nó KHÔNG ở trong 'make check':"
	@echo "một database vừa migrate thì mọi cột đều rỗng."
	@go run ./cmd/deadcol -v

.PHONY: api-lint
api-lint: ## Kiểm tra đặc tả OpenAPI
	npx --yes @redocly/cli@latest lint api/openapi.yaml

.PHONY: alerts
alerts: ## Kiểm tra luật cảnh báo Prometheus
	@command -v promtool >/dev/null 2>&1 || { \
		echo "cần promtool — tải từ https://prometheus.io/download/"; \
		echo "hoặc: brew install prometheus"; exit 1; }
	@promtool check rules deploy/prometheus/alerts.yml
	@promtool test rules deploy/prometheus/alerts_test.yml

.PHONY: check
check: fmt-check vet arch hopdong nhiphan test ## Chạy toàn bộ kiểm tra như CI
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
