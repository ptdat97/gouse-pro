package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Bộ bài này kiểm CHÍNH CÔNG CỤ, không kiểm dự án.
//
// Một công cụ bỏ sót vi phạm còn nguy hiểm hơn không có công cụ: nó tạo
// cảm giác an toàn giả, và cảm giác ấy khiến người ta thôi nhìn. Đó đúng
// là chuyện đã xảy ra với `types:check` — nó xanh suốt trong lúc sáu
// endpoint sống ngoài hợp đồng.
//
// Nên mỗi bài dưới đây dựng một dự án GIẢ có đúng một lỗi, rồi đòi công cụ
// tìm ra nó.

// duAn dựng một cây thư mục tối thiểu mà apicheck đọc được.
func project(t *testing.T, openapi string, paths map[string]string, goSrc string) string {
	t.Helper()
	root2 := t.TempDir()

	must := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	must(filepath.Join(root2, "api", "openapi.yaml"), openapi)
	for label, content := range paths {
		must(filepath.Join(root2, "api", "paths", label), content)
	}
	must(filepath.Join(root2, "internal", "app", "routes.go"), goSrc)

	// Mọi dự án giả cần một cors.go, vì `run` luôn đọc nó. Bài nào muốn
	// kiểm tầng header thì ghi đè file này.
	must(filepath.Join(root2, "internal", "platform", "httpserver", "cors.go"),
		corsGo("X-Thing-Id"))

	// …và một schemas.yaml, vì tầng DTO luôn đọc nó.
	//
	// Dựng ở đây chứ không bắt từng bài tự thêm: thêm tầng thứ năm sau này
	// sẽ lại làm mọi bài cũ vỡ vì cùng một lý do, và lần nào cũng phải sửa
	// hàng chục chỗ. Đã mắc đúng lỗi này với `archcheck` cùng ngày.
	must(filepath.Join(root2, "api", "components", "schemas.yaml"), "")
	return root2
}

// corsGo dựng một cors.go tối thiểu có đúng `var AllowedHeaders`.
func corsGo(header ...string) string {
	var b strings.Builder
	b.WriteString("package httpserver\n\nvar AllowedHeaders = []string{\n")
	for _, h := range header {
		fmt.Fprintf(&b, "\t%q,\n", h)
	}
	b.WriteString("}\n")
	return b.String()
}

// override ghi đè một file trong dự án giả đã dựng.
func override(t *testing.T, root2, path, content string) {
	t.Helper()
	d := filepath.Join(root2, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(d), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const oneRouteOpenAPI = `openapi: 3.1.0
paths:
  /api/v1/things:
    $ref: './paths/things.yaml#/things'
`

const oneRoutePaths = `things:
  get:
    operationId: listThings
    parameters:
      - name: X-Thing-Id
        in: header
        schema:
          type: string
    responses:
      '200':
        description: ok
`

const oneRouteGo = `package app

import "net/http"

func Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/things", nil)
}
`

// chay chạy công cụ trên dự án giả với sổ RỖNG.
//
// Sổ rỗng có chủ ý: bài test đo phép đối chiếu, không đo nội dung sổ của
// dự án thật. Dùng sổ thật thì mỗi bài kèm theo mười chín vi phạm không
// liên quan và không bài nào đo được thứ nó muốn đo.
func runCheck(t *testing.T, root2 string, overrides ...map[string]string) *checker {
	t.Helper()
	c := &checker{
		root:                 root2,
		notImplemented:       map[string]string{},
		outsideContract:      map[string]string{},
		headersOutsideSpec:   map[string]string{},
		headersNotViaBrowser: map[string]string{},
	}
	if len(overrides) > 0 && overrides[0] != nil {
		c.notImplemented = overrides[0]
	}
	if len(overrides) > 1 && overrides[1] != nil {
		c.outsideContract = overrides[1]
	}
	if err := c.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	return c
}

func TestAMatchingRouteReportsNoViolation(t *testing.T) {
	root2 := project(t, oneRouteOpenAPI,
		map[string]string{"things.yaml": oneRoutePaths}, oneRouteGo)

	if c := runCheck(t, root2); len(c.violations) != 0 {
		t.Fatalf("dự án khớp mà báo %d vi phạm: %v", len(c.violations), c.violations)
	}
}

// TUYẾN SỐNG NGOÀI HỢP ĐỒNG — lỗi công cụ này sinh ra để bắt.
func TestCatchesARouteMissingFromTheSpec(t *testing.T) {
	goSrc := oneRouteGo + `
func Register(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/things/{id}/approve", nil)
}
`
	root2 := project(t, oneRouteOpenAPI,
		map[string]string{"things.yaml": oneRoutePaths}, goSrc)

	c := runCheck(t, root2)
	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
	if !strings.Contains(c.violations[0], "POST /api/v1/things/{id}/approve") {
		t.Errorf("vi phạm không nêu đúng tuyến: %s", c.violations[0])
	}
}

// THAO TÁC ĐẶC TẢ KHÔNG AI CÀI, và không ai khai lý do.
func TestCatchesAnUnimplementedOperationNotDeclared(t *testing.T) {
	openapi := oneRouteOpenAPI + `  /api/v1/widgets:
    $ref: './paths/widgets.yaml#/widgets'
`
	widgets := `widgets:
  get:
    operationId: listWidgets
    responses:
      '200':
        description: ok
`
	root2 := project(t, openapi, map[string]string{
		"things.yaml": oneRoutePaths, "widgets.yaml": widgets,
	}, oneRouteGo)

	c := runCheck(t, root2)
	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
	if !strings.Contains(c.violations[0], "GET /api/v1/widgets") {
		t.Errorf("vi phạm không nêu đúng thao tác: %s", c.violations[0])
	}
}

// CHUỖI TRONG MÔ TẢ KHÔNG PHẢI MỘT TUYẾN.
//
// Bài này giữ lý do dùng AST thay vì grep: chuỗi "POST /api/v1/events"
// thật sự xuất hiện trong mô tả của một tham số vận hành, và một phép
// grep sẽ đếm nó là tuyến rồi báo vi phạm giả. Cảnh báo giả là thứ làm
// người ta tắt hẳn phép kiểm.
func TestAStringInADescriptionIsNotARoute(t *testing.T) {
	goSrc := oneRouteGo + `
const description = "Số sự kiện tối đa mỗi phút qua POST /api/v1/events. 0 = TẮT."

func Use() string { return description }
`
	root2 := project(t, oneRouteOpenAPI,
		map[string]string{"things.yaml": oneRoutePaths}, goSrc)

	if c := runCheck(t, root2); len(c.violations) != 0 {
		t.Fatalf("chuỗi trong mô tả bị đếm là tuyến: %v", c.violations)
	}
}

// BỘ ĐỌC ĐẶC TẢ KHÔNG ĐƯỢC DỄ DÃI.
//
// Gặp cách khai nó không hiểu thì phải BÁO LỖI. Âm thầm bỏ qua sẽ dựng
// lại đúng vấn đề công cụ này sinh ra để sửa: một dấu xanh cho một phép
// kiểm không kiểm gì.
func TestAnUnknownSpecDeclarationErrorsInsteadOfBeingSkipped(t *testing.T) {
	openapi := `openapi: 3.1.0
paths:
  /api/v1/things:
    get:
      operationId: listThings
`
	root2 := project(t, openapi,
		map[string]string{"things.yaml": oneRoutePaths}, oneRouteGo)

	c := &checker{
		root:                 root2,
		notImplemented:       map[string]string{},
		outsideContract:      map[string]string{},
		headersOutsideSpec:   map[string]string{},
		headersNotViaBrowser: map[string]string{},
	}
	err := c.run()
	if err == nil {
		t.Fatal("đặc tả khai theo cách lạ mà công cụ vẫn báo đọc xong")
	}
	if !strings.Contains(err.Error(), "$ref") {
		t.Errorf("lỗi không nói rõ vì sao: %v", err)
	}
}

// DANH SÁCH HOÃN PHẢI ĐƯỢC DỌN.
//
// Cài xong mà quên xóa dòng trong `notImplemented` thì danh sách hoãn dần thành
// danh sách không ai đọc — và khi ấy nó che mất thao tác thật sự bị bỏ
// quên.
func TestCatchesADeferRowAlreadyImplemented(t *testing.T) {
	root2 := project(t, oneRouteOpenAPI,
		map[string]string{"things.yaml": oneRoutePaths}, oneRouteGo)

	c := runCheck(t, root2, map[string]string{"GET /api/v1/things": "giả vờ hoãn"})
	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
	if !strings.Contains(c.violations[0], "xóa dòng") {
		t.Errorf("vi phạm không nói phải làm gì: %s", c.violations[0])
	}
}

// MIỄN TRỪ CHO THỨ KHÔNG TỒN TẠI cũng phải bị bắt.
func TestCatchesAnExtraExemption(t *testing.T) {
	root2 := project(t, oneRouteOpenAPI,
		map[string]string{"things.yaml": oneRoutePaths}, oneRouteGo)

	c := runCheck(t, root2, nil, map[string]string{"GET /khong-ton-tai": "giả vờ"})
	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
}

// DỰ ÁN THẬT phải sạch — bài này chạy công cụ lên chính repo.
//
// Hai bài kia dùng dự án giả nên chúng chứng minh công cụ ĐÚNG; bài này
// chứng minh dự án đang KHỚP. Thiếu nó thì công cụ có thể đúng mà không ai
// chạy nó lên thứ đáng chạy.
func TestTheRealProjectMatches(t *testing.T) {
	c := &checker{
		root: "../..", notImplemented: notImplemented, outsideContract: outsideContract,
		headersOutsideSpec:   headersOutsideSpec,
		headersNotViaBrowser: headersNotViaBrowser,
		enumPair:             checkedEnumPairs,
		unpairedEnums:        unpairedEnums,
		ungatedEnumLedger:    ungatedEnums,
		dtoPair:              checkedDTOPairs,
		ungatedDTOLedger:     ungatedDTOs,
	}
	if err := c.run(); err != nil {
		t.Fatalf("run trên repo thật: %v", err)
	}
	if len(c.violations) != 0 {
		t.Fatalf("repo có %d lệch giữa đặc tả và tuyến:\n%s",
			len(c.violations), strings.Join(c.violations, "\n"))
	}
}

// ------------------------------------------------------------- Tầng HEADER

// runHeader chạy công cụ với hai sổ header truyền vào.
func runHeaderCheck(
	t *testing.T, root2 string, outsideSpec, notViaBrowser map[string]string,
) *checker {
	t.Helper()
	if outsideSpec == nil {
		outsideSpec = map[string]string{}
	}
	if notViaBrowser == nil {
		notViaBrowser = map[string]string{}
	}
	c := &checker{
		root:                 root2,
		notImplemented:       map[string]string{},
		outsideContract:      map[string]string{},
		headersOutsideSpec:   outsideSpec,
		headersNotViaBrowser: notViaBrowser,
	}
	if err := c.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	return c
}

// ĐÂY LÀ LỖI THẬT ngày 17/09/2026, dựng lại ở quy mô nhỏ.
//
// `X-Visit-Id` vào api-client mà không vào CORS: preflight từ chối, request
// thật không rời máy khách, log máy chủ sạch trơn, và CẢ cửa hàng ngừng
// tải dữ liệu. Không phép kiểm nào lúc đó nhìn vào chỗ này.
func TestCatchesASpecHeaderCORSDoesNotAllow(t *testing.T) {
	root2 := project(t, oneRouteOpenAPI,
		map[string]string{"things.yaml": oneRoutePaths}, oneRouteGo)
	// CORS quên mất X-Thing-Id mà đặc tả khai.
	override(t, root2, "internal/platform/httpserver/cors.go", corsGo("Authorization"))

	c := runHeaderCheck(t, root2, map[string]string{"Authorization": "chuẩn"}, nil)
	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
	if !strings.Contains(c.violations[0], "X-Thing-Id") {
		t.Errorf("vi phạm không nêu đúng header: %s", c.violations[0])
	}
	// Thông điệp phải nói rõ vì sao KHÔNG tìm thấy dấu vết ở máy chủ —
	// thiếu câu đó thì người sửa đi tìm nhầm chỗ, đúng như lần đầu.
	if !strings.Contains(c.violations[0], "SẠCH") {
		t.Errorf("vi phạm không cảnh báo về log máy chủ sạch: %s", c.violations[0])
	}
}

// Hướng ngược lại: CORS rộng hơn đặc tả.
//
// Không vô hại. Mỗi header thừa là một lời mời gửi kèm từ trình duyệt, và
// không ai rà soát thứ không nằm trong hợp đồng.
func TestCatchesACORSHeaderTheSpecDoesNotDeclare(t *testing.T) {
	root2 := project(t, oneRouteOpenAPI,
		map[string]string{"things.yaml": oneRoutePaths}, oneRouteGo)
	override(t, root2, "internal/platform/httpserver/cors.go",
		corsGo("X-Thing-Id", "X-Khong-Ai-Khai"))

	c := runHeaderCheck(t, root2, nil, nil)
	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
	if !strings.Contains(c.violations[0], "X-Khong-Ai-Khai") {
		t.Errorf("vi phạm không nêu đúng header: %s", c.violations[0])
	}
}

// Header SERVER-TO-SERVER phải khai lý do, và khai rồi thì hết vi phạm.
//
// `X-Signature` của webhook là ca thật: đặc tả khai nó, mà cho nó vào CORS
// sẽ là nói với mọi trang web rằng chữ ký webhook gửi được từ trình duyệt.
func TestAServerToServerHeaderWithAReasonClearsTheViolation(t *testing.T) {
	root2 := project(t, oneRouteOpenAPI,
		map[string]string{"things.yaml": oneRoutePaths}, oneRouteGo)
	override(t, root2, "internal/platform/httpserver/cors.go", corsGo("Authorization"))

	c := runHeaderCheck(t, root2,
		map[string]string{"Authorization": "chuẩn"},
		map[string]string{"X-Thing-Id": "webhook, không qua trình duyệt"})
	if len(c.violations) != 0 {
		t.Fatalf("khai lý do rồi mà vẫn báo: %v", c.violations)
	}
}

// Dòng miễn trừ CHẾT cũng phải bị bắt — ở cả hai sổ.
func TestCatchesADeadHeaderExemptionRow(t *testing.T) {
	root2 := project(t, oneRouteOpenAPI,
		map[string]string{"things.yaml": oneRoutePaths}, oneRouteGo)

	c := runHeaderCheck(t, root2,
		map[string]string{"X-Da-Bo": "CORS không còn cho phép"},
		map[string]string{"X-Cung-Da-Bo": "đặc tả không còn khai"})
	if len(c.violations) != 2 {
		t.Fatalf("mong 2 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
	got := strings.Join(c.violations, "\n")
	for _, can := range []string{"X-Da-Bo", "X-Cung-Da-Bo", "xóa dòng"} {
		if !strings.Contains(got, can) {
			t.Errorf("thiếu %q trong: %s", can, got)
		}
	}
}

// HOA THƯỜNG không được tạo cảnh báo giả.
//
// Chuẩn HTTP nói tên header không phân biệt hoa thường, và trình duyệt gửi
// `x-visit-id` viết thường. Một phép so phân biệt hoa thường sẽ báo lỗi cho
// `X-Request-ID` đấu với `X-Request-Id` — hai cách viết của MỘT header.
// Cảnh báo giả là thứ làm người ta tắt hẳn phép kiểm.
func TestHeadersAreCaseInsensitive(t *testing.T) {
	root2 := project(t, oneRouteOpenAPI,
		map[string]string{"things.yaml": oneRoutePaths}, oneRouteGo)
	override(t, root2, "internal/platform/httpserver/cors.go", corsGo("X-THING-ID"))

	if c := runHeaderCheck(t, root2, nil, nil); len(c.violations) != 0 {
		t.Fatalf("khác hoa thường bị coi là khác header: %v", c.violations)
	}
}

// `in: header` KHÔNG tìm được tên thì BÁO LỖI, không bỏ qua.
//
// Cùng nguyên tắc với bộ đọc đặc tả: bỏ qua âm thầm là dựng lại đúng vấn
// đề công cụ này sinh ra để sửa.
func TestANameLessHeaderErrors(t *testing.T) {
	paths := `things:
  get:
    operationId: listThings
    parameters:
      - in: header
        schema:
          type: string
    responses:
      '200':
        description: ok
`
	root2 := project(t, oneRouteOpenAPI, map[string]string{"things.yaml": paths}, oneRouteGo)

	c := &checker{
		root:                 root2,
		notImplemented:       map[string]string{},
		outsideContract:      map[string]string{},
		headersOutsideSpec:   map[string]string{},
		headersNotViaBrowser: map[string]string{},
	}
	err := c.run()
	if err == nil {
		t.Fatal("khối `in: header` không có tên mà công cụ vẫn báo đọc xong")
	}
	if !strings.Contains(err.Error(), "name:") {
		t.Errorf("lỗi không nói rõ thiếu gì: %v", err)
	}
}

// --------------------------------------------------------------- Tầng ENUM

// runEnum chạy công cụ với sổ ghép enum truyền vào.
//
// `nguong` phải truyền tay từng bài: chốt của dự án thật (63) sẽ biến mọi
// bài ở đây thành "thiếu 63 enum chưa gác" và không bài nào đo được thứ nó
// muốn đo.
func runEnum(
	t *testing.T, root2 string, pair []enumPair, unpaired map[string]string, ledger int,
) *checker {
	t.Helper()
	if unpaired == nil {
		unpaired = map[string]string{}
	}
	c := &checker{
		root:                 root2,
		notImplemented:       map[string]string{},
		outsideContract:      map[string]string{},
		headersOutsideSpec:   map[string]string{},
		headersNotViaBrowser: map[string]string{},
		enumPair:             pair,
		unpairedEnums:        unpaired,
		ungatedEnumLedger:    ledger,
	}
	if err := c.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	return c
}

// samplePath là đường dẫn mà cả hai cách viết YAML dưới đây phải cho ra.
const samplePath = "components/mau.yaml#/schemas/Color/properties/color_family"

// multiLineEnum là dạng danh sách — cách đặc tả thật viết.
const multiLineEnum = `schemas:
  Color:
    properties:
      color_family:
        type: string
        enum:
          - WHITE
          - GREY
`

// oneLineEnum là dạng gọn `[A, B]` — YAML hợp lệ như nhau.
const oneLineEnum = `schemas:
  Color:
    properties:
      color_family:
        type: string
        enum: [WHITE, GREY]
`

const sampleGo = `package domain

type ColorFamily string

const (
	ColorWhite ColorFamily = "WHITE"
	ColorGrey   ColorFamily = "GREY"
)
`

// samplePairs là sổ ghép trỏ đúng vào cặp trên.
func samplePairs(subset bool) []enumPair {
	return []enumPair{{samplePath, "thing/domain", "ColorFamily", subset, "nhóm màu"}}
}

// sampleProject dựng dự án giả có một enum trong đặc tả và một kiểu trong Go.
func sampleProject(t *testing.T, yaml, goSrc string) string {
	t.Helper()
	root2 := project(t, oneRouteOpenAPI,
		map[string]string{"things.yaml": oneRoutePaths}, oneRouteGo)
	override(t, root2, "api/components/mau.yaml", yaml)
	override(t, root2, "internal/thing/domain/mau.go", goSrc)
	return root2
}

func TestAMatchingEnumReportsNoViolation(t *testing.T) {
	root2 := sampleProject(t, multiLineEnum, sampleGo)

	if c := runEnum(t, root2, samplePairs(false), nil, 0); len(c.violations) != 0 {
		t.Fatalf("mong 0 vi phạm, nhận: %v", c.violations)
	}
}

// HAI cách viết YAML của cùng một thứ phải cho CÙNG một đường dẫn.
//
// Nếu không, sổ ghép phải biết đặc tả viết kiểu nào — một chi tiết không ai
// nhớ, và đổi cách viết sẽ lặng lẽ tháo hàng rào ra khỏi enum đó.
func TestOneLineEnumsTakeTheSamePathAsMultiLine(t *testing.T) {
	root2 := sampleProject(t, oneLineEnum, sampleGo)

	if c := runEnum(t, root2, samplePairs(false), nil, 0); len(c.violations) != 0 {
		t.Fatalf("dạng một dòng phải khớp cùng sổ, nhận: %v", c.violations)
	}
}

// ĐÂY LÀ LỖI THẬT: đặc tả nói `GRAY`, máy chủ lưu `GREY`.
//
// Bộ lọc theo màu im lặng trả rỗng — không lỗi, không log, không ai biết.
// Một chữ cái, và cả hai chiều đều sai cùng lúc.
func TestCatchesASingleLetterDrift(t *testing.T) {
	root2 := sampleProject(t, strings.Replace(multiLineEnum, "GREY", "GRAY", 1), sampleGo)

	c := runEnum(t, root2, samplePairs(false), nil, 0)
	if len(c.violations) != 2 {
		t.Fatalf("mong 2 vi phạm (mỗi chiều một), nhận %d: %v", len(c.violations), c.violations)
	}
	merge := strings.Join(c.violations, "\n")
	if !strings.Contains(merge, "GRAY") || !strings.Contains(merge, "GREY") {
		t.Fatalf("phải gọi tên CẢ HAI giá trị lệch: %v", c.violations)
	}
}

// Đặc tả khai giá trị Go không sinh ra: client viết một nhánh không bao giờ
// chạy tới. KHÔNG BAO GIỜ hợp lệ — kể cả khi cặp cho phép tập con.
func TestCatchesASpecValueMissingFromGo(t *testing.T) {
	yaml := strings.Replace(multiLineEnum, "          - GREY",
		"          - GREY\n          - MULTI", 1)
	root2 := sampleProject(t, yaml, sampleGo)

	for _, subset := range []bool{false, true} {
		c := runEnum(t, root2, samplePairs(subset), nil, 0)
		if len(c.violations) != 1 {
			t.Fatalf("tậpCon=%v: mong 1 vi phạm, nhận %d: %v",
				subset, len(c.violations), c.violations)
		}
		if !strings.Contains(c.violations[0], "MULTI") {
			t.Fatalf("tậpCon=%v: phải gọi tên MULTI: %v", subset, c.violations[0])
		}
	}
}

// ĐÂY LÀ LỖI THẬT: `account_type` khai 9 tài khoản, Go có 13.
//
// Client sinh kiểu từ đặc tả thu hẹp kiểu sai, rồi vỡ khi gặp response thật.
func TestCatchesAGoValueMissingFromTheSpec(t *testing.T) {
	goSrc := strings.Replace(sampleGo, `	ColorGrey   ColorFamily = "GREY"`,
		`	ColorGrey   ColorFamily = "GREY"`+"\n"+`	ColorSilver   ColorFamily = "SILVER"`, 1)
	root2 := sampleProject(t, multiLineEnum, goSrc)

	c := runEnum(t, root2, samplePairs(false), nil, 0)
	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
	if !strings.Contains(c.violations[0], "SILVER") {
		t.Fatalf("phải gọi tên SILVER: %v", c.violations[0])
	}

	// …trừ khi cặp KHAI rằng đặc tả hẹp hơn là đúng — như `tier` của `/me`
	// không bao giờ trả `ANONYMIZED`.
	if c := runEnum(t, root2, samplePairs(true), nil, 0); len(c.violations) != 0 {
		t.Fatalf("khai AllowSubset rồi thì phải im: %v", c.violations)
	}
}

// Đọc AST chứ không grep.
//
// Một giá trị nằm trong bình luận hay trong thông điệp lỗi KHÔNG phải một
// giá trị hợp lệ của kiểu. Nếu công cụ grep, nó sẽ đòi đặc tả khai thêm
// `BLACK` — một cảnh báo giả, và cảnh báo giả làm người ta tắt phép kiểm.
func TestValuesInCommentsAndStringsDoNotCount(t *testing.T) {
	goSrc := sampleGo + `
// ColorBlack ColorFamily = "BLACK" — bỏ đi vì kho không còn phân loại theo đen.

const ErrMessage = "nhóm màu hợp lệ: WHITE, GREY, BLACK"
`
	root2 := sampleProject(t, multiLineEnum, goSrc)

	if c := runEnum(t, root2, samplePairs(false), nil, 0); len(c.violations) != 0 {
		t.Fatalf("BLACK trong bình luận/chuỗi không phải giá trị: %v", c.violations)
	}
}

// Sổ ghép trỏ tới đường dẫn KHÔNG CÒN là một dòng nói dối.
//
// Nó im lặng ngừng gác enum ấy trong khi vẫn trông như đang gác — đúng kiểu
// hỏng mà `types:check` đã mắc.
func TestCatchesALedgerEntryPointingAtADeadPath(t *testing.T) {
	root2 := sampleProject(t, multiLineEnum, sampleGo)
	deadPairs := []enumPair{{
		"components/mau.yaml#/schemas/ColorInfo/properties/color_family",
		"thing/domain", "ColorFamily", false, "nhóm màu",
	}}

	// Enum thật lúc này KHÔNG được gác, nên chốt để 1 để đo đúng một thứ.
	c := runEnum(t, root2, deadPairs, nil, 1)
	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
	if !strings.Contains(c.violations[0], "ColorInfo") {
		t.Fatalf("phải gọi tên đường dẫn chết: %v", c.violations[0])
	}
}

// Sổ miễn trừ cũng phải chết theo enum nó miễn trừ.
func TestCatchesADeadUnpairedRow(t *testing.T) {
	root2 := sampleProject(t, multiLineEnum, sampleGo)
	c := runEnum(t, root2, samplePairs(false),
		map[string]string{"components/mau.yaml#/schemas/DaXoa/properties/x": "lý do cũ"}, 0)

	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
	if !strings.Contains(c.violations[0], "DaXoa") {
		t.Fatalf("phải gọi tên dòng chết: %v", c.violations[0])
	}
}

// Chốt CHỈ ĐƯỢC GIẢM.
//
// Một chốt cao hơn thực tế là chỗ trống để lặng lẽ thêm enum không ai gác —
// nó không đỏ, nên không ai biết hàng rào đã nới ra.
func TestTheUngatedEnumRatchetOnlyGoesDown(t *testing.T) {
	root2 := sampleProject(t, multiLineEnum, sampleGo)

	c := runEnum(t, root2, samplePairs(false), nil, 5)
	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
	if !strings.Contains(c.violations[0], "Hạ chốt") {
		t.Fatalf("phải bảo hạ chốt: %v", c.violations[0])
	}

	// Thêm enum không gác mà chốt vẫn thấp thì cũng đỏ, ở chiều kia.
	if c := runEnum(t, root2, nil, nil, 0); len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm ở chiều vượt chốt, nhận %d: %v",
			len(c.violations), c.violations)
	}
}

// Đổi tên kiểu Go mà quên sửa sổ: công cụ phải KÊU, không được im.
func TestCatchesARenamedGoType(t *testing.T) {
	root2 := sampleProject(t, multiLineEnum, strings.ReplaceAll(sampleGo, "ColorFamily", "ColourFamily"))

	c := runEnum(t, root2, samplePairs(false), nil, 0)
	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
	if !strings.Contains(c.violations[0], "ColorFamily") {
		t.Fatalf("phải gọi tên kiểu không tìm thấy: %v", c.violations[0])
	}
}

// ---------------------------------------------------------------- Tầng DTO

// runDTO chạy công cụ với sổ ghép DTO truyền vào.
//
// `nguong` truyền tay từng bài: chốt của dự án thật (20) sẽ biến mọi bài ở
// đây thành "thiếu 20 schema chưa gác".
func runDTOCheck(t *testing.T, root2 string, pair []dtoPair, ledger int) *checker {
	t.Helper()
	c := &checker{
		root:                 root2,
		notImplemented:       map[string]string{},
		outsideContract:      map[string]string{},
		headersOutsideSpec:   map[string]string{},
		headersNotViaBrowser: map[string]string{},
		dtoPair:              pair,
		ungatedDTOLedger:     ledger,
	}
	if err := c.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	return c
}

// rowSchema là một schema có ba thuộc tính, dạng thật của đặc tả.
const rowSchema = `Row:
  type: object
  required: [id, name]
  description: |
    Một mô tả nhiều dòng, để chắc rằng phép đọc không nhầm nó thành
    thuộc tính.
  properties:
    id:
      $ref: './common.yaml#/schemas/Id'
    name:
      type: string
    group:
      type: object
      description: Object LỒNG — thuộc tính của nó KHÔNG phải của Row.
      properties:
        code:
          type: string
        label:
          type: string
`

// rowDTO dựng struct Go phục vụ schema trên.
func rowDTO(fields ...string) string {
	s := "package http\n\ntype rowJSON struct {\n"
	for _, t := range fields {
		s += "\tX" + t + " string `json:\"" + t + "\"`\n"
	}
	return s + "}\n"
}

// dtoProject dựng dự án giả có schemas.yaml và một gói DTO.
func dtoProject(t *testing.T, schema, dto string) string {
	t.Helper()
	root2 := project(t, oneRouteOpenAPI,
		map[string]string{"things.yaml": oneRoutePaths}, oneRouteGo)
	override(t, root2, "api/components/schemas.yaml", schema)
	override(t, root2, "internal/modules/row/interfaces/http/dto.go", dto)
	return root2
}

func rowPairs(allowMissing map[string]string) []dtoPair {
	return []dtoPair{{
		Schema: "Row", GoPkg: "modules/row/interfaces/http",
		GoType: "rowJSON", AllowMissing: allowMissing, Reason: "hàng thử",
	}}
}

func TestAMatchingDTOReportsNoViolation(t *testing.T) {
	root2 := dtoProject(t, rowSchema, rowDTO("id", "name", "group"))

	if c := runDTOCheck(t, root2, rowPairs(nil), 0); len(c.violations) != 0 {
		t.Fatalf("mong 0 vi phạm, nhận: %v", c.violations)
	}
}

// Thuộc tính của object LỒNG không phải thuộc tính của schema.
//
// Nhận nhầm chúng sẽ đòi struct Go có `ma` và `nhan` ở cấp ngoài cùng —
// một cảnh báo giả, và cảnh báo giả làm người ta tắt hẳn phép kiểm.
//
// Bản đầu của phép đọc mắc đúng lỗi này theo chiều NGƯỢC LẠI: nó dùng một
// máy trạng thái "đã vào object lồng" và không bao giờ ra, nên `Checkout`
// chỉ đọc được 7 trong 12 thuộc tính.
func TestDTONestedPropertiesDoNotCount(t *testing.T) {
	root2 := dtoProject(t, rowSchema, rowDTO("id", "name", "group"))

	c := runDTOCheck(t, root2, rowPairs(nil), 0)
	for _, v := range c.violations {
		if strings.Contains(v, "ma") || strings.Contains(v, "nhan") {
			t.Fatalf("thuộc tính lồng bị tính vào schema ngoài: %s", v)
		}
	}
}

// ĐÂY LÀ DẠNG LỖI đã xảy ra năm lần: đặc tả có, DTO không có.
func TestDTOCatchesASpecFieldMissingFromGo(t *testing.T) {
	root2 := dtoProject(t, rowSchema, rowDTO("id", "name")) // thiếu `group`

	c := runDTOCheck(t, root2, rowPairs(nil), 0)
	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
	if !strings.Contains(c.violations[0], "group") {
		t.Fatalf("phải gọi tên trường thiếu: %s", c.violations[0])
	}
}

// Chiều ngược: Go trả trường mà hợp đồng không nhắc.
//
// Không ai biết nó tồn tại để dùng, và xóa nó đi là thay đổi PHÁ VỠ không
// ai nhận ra.
func TestDTOCatchesAGoFieldOutsideTheContract(t *testing.T) {
	root2 := dtoProject(t, rowSchema, rowDTO("id", "name", "group", "secret"))

	c := runDTOCheck(t, root2, rowPairs(nil), 0)
	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
	if !strings.Contains(c.violations[0], "secret") {
		t.Fatalf("phải gọi tên trường thừa: %s", c.violations[0])
	}
}

// `AllowMissing` tắt được một trường, và CHỈ trường ấy.
func TestDTOAllowMissingSilencesExactlyOneField(t *testing.T) {
	root2 := dtoProject(t, rowSchema, rowDTO("id")) // thiếu `name` và `group`

	c := runDTOCheck(t, root2, rowPairs(map[string]string{"group": "chưa làm"}), 0)
	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm (còn `name`), nhận %d: %v",
			len(c.violations), c.violations)
	}
	if !strings.Contains(c.violations[0], "name") ||
		strings.Contains(c.violations[0], "group") {
		t.Fatalf("phải còn kêu `name` và im về `group`: %s", c.violations[0])
	}
}

// Hai cách một dòng `AllowMissing` thành NÓI DỐI.
func TestDTOAllowMissingThatLiesIsCaught(t *testing.T) {
	t.Run("trường không còn trong đặc tả", func(t *testing.T) {
		root2 := dtoProject(t, rowSchema, rowDTO("id", "name", "group"))
		c := runDTOCheck(t, root2, rowPairs(map[string]string{"da_xoa": "lý do cũ"}), 0)
		if len(c.violations) != 1 || !strings.Contains(c.violations[0], "da_xoa") {
			t.Fatalf("mong 1 vi phạm về `da_xoa`, nhận: %v", c.violations)
		}
	})

	t.Run("Go ĐÃ trả trường ấy", func(t *testing.T) {
		root2 := dtoProject(t, rowSchema, rowDTO("id", "name", "group"))
		c := runDTOCheck(t, root2, rowPairs(map[string]string{"name": "nói là chưa trả"}), 0)
		if len(c.violations) != 1 || !strings.Contains(c.violations[0], "ĐÃ trả") {
			t.Fatalf("mong 1 vi phạm \"Go ĐÃ trả\", nhận: %v", c.violations)
		}
	})
}

// Sổ ghép trỏ tới schema hoặc struct KHÔNG CÒN phải bị bắt.
func TestDTOCatchesALedgerEntryPointingWrong(t *testing.T) {
	t.Run("schema không tồn tại", func(t *testing.T) {
		root2 := dtoProject(t, rowSchema, rowDTO("id", "name", "group"))
		c := runDTOCheck(t, root2, []dtoPair{{
			Schema: "Missing", GoPkg: "modules/row/interfaces/http",
			GoType: "rowJSON", Reason: "thử",
		}}, 1)
		if len(c.violations) != 1 || !strings.Contains(c.violations[0], "Missing") {
			t.Fatalf("mong 1 vi phạm về schema chết, nhận: %v", c.violations)
		}
	})

	t.Run("struct không tồn tại", func(t *testing.T) {
		root2 := dtoProject(t, rowSchema, rowDTO("id", "name", "group"))
		c := runDTOCheck(t, root2, []dtoPair{{
			Schema: "Row", GoPkg: "modules/row/interfaces/http",
			GoType: "khongCoJSON", Reason: "thử",
		}}, 0)
		if len(c.violations) != 1 || !strings.Contains(c.violations[0], "khongCoJSON") {
			t.Fatalf("mong 1 vi phạm về struct chết, nhận: %v", c.violations)
		}
	})
}

// Chốt schema chưa gác CHỈ ĐƯỢC GIẢM.
func TestTheUngatedDTORatchetOnlyGoesDown(t *testing.T) {
	root2 := dtoProject(t, rowSchema+`
OtherRow:
  type: object
  properties:
    x:
      type: string
`, rowDTO("id", "name", "group"))

	// `OtherRow` chưa gác → chốt phải là 1.
	if c := runDTOCheck(t, root2, rowPairs(nil), 1); len(c.violations) != 0 {
		t.Fatalf("chốt đúng thì phải im: %v", c.violations)
	}
	if c := runDTOCheck(t, root2, rowPairs(nil), 0); len(c.violations) != 1 {
		t.Fatalf("vượt chốt phải kêu: %v", c.violations)
	}
	if c := runDTOCheck(t, root2, rowPairs(nil), 5); len(c.violations) != 1 ||
		!strings.Contains(c.violations[0], "Hạ chốt") {
		t.Fatalf("chốt cao hơn thực tế phải bảo hạ: %v", c.violations)
	}
}
