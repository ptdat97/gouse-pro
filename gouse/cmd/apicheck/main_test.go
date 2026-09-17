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
func duAn(t *testing.T, openapi string, paths map[string]string, goSrc string) string {
	t.Helper()
	goc := t.TempDir()

	must := func(duong, noiDung string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(duong), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(duong, []byte(noiDung), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	must(filepath.Join(goc, "api", "openapi.yaml"), openapi)
	for ten, noiDung := range paths {
		must(filepath.Join(goc, "api", "paths", ten), noiDung)
	}
	must(filepath.Join(goc, "internal", "app", "routes.go"), goSrc)

	// Mọi dự án giả cần một cors.go, vì `run` luôn đọc nó. Bài nào muốn
	// kiểm tầng header thì ghi đè file này.
	must(filepath.Join(goc, "internal", "platform", "httpserver", "cors.go"),
		corsGo("X-Thing-Id"))
	return goc
}

// corsGo dựng một cors.go tối thiểu có đúng `var HeaderChoPhep`.
func corsGo(header ...string) string {
	var b strings.Builder
	b.WriteString("package httpserver\n\nvar HeaderChoPhep = []string{\n")
	for _, h := range header {
		fmt.Fprintf(&b, "\t%q,\n", h)
	}
	b.WriteString("}\n")
	return b.String()
}

// ghiDe ghi đè một file trong dự án giả đã dựng.
func ghiDe(t *testing.T, goc, duong, noiDung string) {
	t.Helper()
	d := filepath.Join(goc, filepath.FromSlash(duong))
	if err := os.MkdirAll(filepath.Dir(d), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d, []byte(noiDung), 0o644); err != nil {
		t.Fatal(err)
	}
}

const openapiMotTuyen = `openapi: 3.1.0
paths:
  /api/v1/things:
    $ref: './paths/things.yaml#/things'
`

const pathsMotTuyen = `things:
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

const goMotTuyen = `package app

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
func chay(t *testing.T, goc string, so ...map[string]string) *checker {
	t.Helper()
	c := &checker{
		root:                     goc,
		chuaCai:                  map[string]string{},
		ngoaiHopDong:             map[string]string{},
		headerNgoaiDacTa:         map[string]string{},
		headerKhongQuaTrinhDuyet: map[string]string{},
	}
	if len(so) > 0 && so[0] != nil {
		c.chuaCai = so[0]
	}
	if len(so) > 1 && so[1] != nil {
		c.ngoaiHopDong = so[1]
	}
	if err := c.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	return c
}

func TestKhopThiKhongBaoViPham(t *testing.T) {
	goc := duAn(t, openapiMotTuyen,
		map[string]string{"things.yaml": pathsMotTuyen}, goMotTuyen)

	if c := chay(t, goc); len(c.viPham) != 0 {
		t.Fatalf("dự án khớp mà báo %d vi phạm: %v", len(c.viPham), c.viPham)
	}
}

// TUYẾN SỐNG NGOÀI HỢP ĐỒNG — lỗi công cụ này sinh ra để bắt.
func TestBatTuyenKhongCoTrongDacTa(t *testing.T) {
	goSrc := goMotTuyen + `
func Them(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/things/{id}/approve", nil)
}
`
	goc := duAn(t, openapiMotTuyen,
		map[string]string{"things.yaml": pathsMotTuyen}, goSrc)

	c := chay(t, goc)
	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
	if !strings.Contains(c.viPham[0], "POST /api/v1/things/{id}/approve") {
		t.Errorf("vi phạm không nêu đúng tuyến: %s", c.viPham[0])
	}
}

// THAO TÁC ĐẶC TẢ KHÔNG AI CÀI, và không ai khai lý do.
func TestBatThaoTacChuaCaiMaKhongKhai(t *testing.T) {
	openapi := openapiMotTuyen + `  /api/v1/widgets:
    $ref: './paths/widgets.yaml#/widgets'
`
	widgets := `widgets:
  get:
    operationId: listWidgets
    responses:
      '200':
        description: ok
`
	goc := duAn(t, openapi, map[string]string{
		"things.yaml": pathsMotTuyen, "widgets.yaml": widgets,
	}, goMotTuyen)

	c := chay(t, goc)
	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
	if !strings.Contains(c.viPham[0], "GET /api/v1/widgets") {
		t.Errorf("vi phạm không nêu đúng thao tác: %s", c.viPham[0])
	}
}

// CHUỖI TRONG MÔ TẢ KHÔNG PHẢI MỘT TUYẾN.
//
// Bài này giữ lý do dùng AST thay vì grep: chuỗi "POST /api/v1/events"
// thật sự xuất hiện trong mô tả của một tham số vận hành, và một phép
// grep sẽ đếm nó là tuyến rồi báo vi phạm giả. Cảnh báo giả là thứ làm
// người ta tắt hẳn phép kiểm.
func TestChuoiTrongMoTaKhongPhaiTuyen(t *testing.T) {
	goSrc := goMotTuyen + `
const moTa = "Số sự kiện tối đa mỗi phút qua POST /api/v1/events. 0 = TẮT."

func Dung() string { return moTa }
`
	goc := duAn(t, openapiMotTuyen,
		map[string]string{"things.yaml": pathsMotTuyen}, goSrc)

	if c := chay(t, goc); len(c.viPham) != 0 {
		t.Fatalf("chuỗi trong mô tả bị đếm là tuyến: %v", c.viPham)
	}
}

// BỘ ĐỌC ĐẶC TẢ KHÔNG ĐƯỢC DỄ DÃI.
//
// Gặp cách khai nó không hiểu thì phải BÁO LỖI. Âm thầm bỏ qua sẽ dựng
// lại đúng vấn đề công cụ này sinh ra để sửa: một dấu xanh cho một phép
// kiểm không kiểm gì.
func TestDacTaKhaiLaThiBaoLoiChuKhongBoQua(t *testing.T) {
	openapi := `openapi: 3.1.0
paths:
  /api/v1/things:
    get:
      operationId: listThings
`
	goc := duAn(t, openapi,
		map[string]string{"things.yaml": pathsMotTuyen}, goMotTuyen)

	c := &checker{
		root:                     goc,
		chuaCai:                  map[string]string{},
		ngoaiHopDong:             map[string]string{},
		headerNgoaiDacTa:         map[string]string{},
		headerKhongQuaTrinhDuyet: map[string]string{},
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
// Cài xong mà quên xóa dòng trong `chuaCai` thì danh sách hoãn dần thành
// danh sách không ai đọc — và khi ấy nó che mất thao tác thật sự bị bỏ
// quên.
func TestBatDongHoanDaCaiXong(t *testing.T) {
	goc := duAn(t, openapiMotTuyen,
		map[string]string{"things.yaml": pathsMotTuyen}, goMotTuyen)

	c := chay(t, goc, map[string]string{"GET /api/v1/things": "giả vờ hoãn"})
	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
	if !strings.Contains(c.viPham[0], "xóa dòng") {
		t.Errorf("vi phạm không nói phải làm gì: %s", c.viPham[0])
	}
}

// MIỄN TRỪ CHO THỨ KHÔNG TỒN TẠI cũng phải bị bắt.
func TestBatMienTruThua(t *testing.T) {
	goc := duAn(t, openapiMotTuyen,
		map[string]string{"things.yaml": pathsMotTuyen}, goMotTuyen)

	c := chay(t, goc, nil, map[string]string{"GET /khong-ton-tai": "giả vờ"})
	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
}

// DỰ ÁN THẬT phải sạch — bài này chạy công cụ lên chính repo.
//
// Hai bài kia dùng dự án giả nên chúng chứng minh công cụ ĐÚNG; bài này
// chứng minh dự án đang KHỚP. Thiếu nó thì công cụ có thể đúng mà không ai
// chạy nó lên thứ đáng chạy.
func TestDuAnThatKhop(t *testing.T) {
	c := &checker{
		root: "../..", chuaCai: chuaCai, ngoaiHopDong: ngoaiHopDong,
		headerNgoaiDacTa:         headerNgoaiDacTa,
		headerKhongQuaTrinhDuyet: headerKhongQuaTrinhDuyet,
	}
	if err := c.run(); err != nil {
		t.Fatalf("run trên repo thật: %v", err)
	}
	if len(c.viPham) != 0 {
		t.Fatalf("repo có %d lệch giữa đặc tả và tuyến:\n%s",
			len(c.viPham), strings.Join(c.viPham, "\n"))
	}
}

// ------------------------------------------------------------- Tầng HEADER

// chayHeader chạy công cụ với hai sổ header truyền vào.
func chayHeader(
	t *testing.T, goc string, ngoaiDacTa, khongQuaTrinhDuyet map[string]string,
) *checker {
	t.Helper()
	if ngoaiDacTa == nil {
		ngoaiDacTa = map[string]string{}
	}
	if khongQuaTrinhDuyet == nil {
		khongQuaTrinhDuyet = map[string]string{}
	}
	c := &checker{
		root:                     goc,
		chuaCai:                  map[string]string{},
		ngoaiHopDong:             map[string]string{},
		headerNgoaiDacTa:         ngoaiDacTa,
		headerKhongQuaTrinhDuyet: khongQuaTrinhDuyet,
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
func TestBatHeaderDacTaCoMaCORSKhongChoPhep(t *testing.T) {
	goc := duAn(t, openapiMotTuyen,
		map[string]string{"things.yaml": pathsMotTuyen}, goMotTuyen)
	// CORS quên mất X-Thing-Id mà đặc tả khai.
	ghiDe(t, goc, "internal/platform/httpserver/cors.go", corsGo("Authorization"))

	c := chayHeader(t, goc, map[string]string{"Authorization": "chuẩn"}, nil)
	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
	if !strings.Contains(c.viPham[0], "X-Thing-Id") {
		t.Errorf("vi phạm không nêu đúng header: %s", c.viPham[0])
	}
	// Thông điệp phải nói rõ vì sao KHÔNG tìm thấy dấu vết ở máy chủ —
	// thiếu câu đó thì người sửa đi tìm nhầm chỗ, đúng như lần đầu.
	if !strings.Contains(c.viPham[0], "SẠCH") {
		t.Errorf("vi phạm không cảnh báo về log máy chủ sạch: %s", c.viPham[0])
	}
}

// Hướng ngược lại: CORS rộng hơn đặc tả.
//
// Không vô hại. Mỗi header thừa là một lời mời gửi kèm từ trình duyệt, và
// không ai rà soát thứ không nằm trong hợp đồng.
func TestBatHeaderCORSChoPhepMaDacTaKhongKhai(t *testing.T) {
	goc := duAn(t, openapiMotTuyen,
		map[string]string{"things.yaml": pathsMotTuyen}, goMotTuyen)
	ghiDe(t, goc, "internal/platform/httpserver/cors.go",
		corsGo("X-Thing-Id", "X-Khong-Ai-Khai"))

	c := chayHeader(t, goc, nil, nil)
	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
	if !strings.Contains(c.viPham[0], "X-Khong-Ai-Khai") {
		t.Errorf("vi phạm không nêu đúng header: %s", c.viPham[0])
	}
}

// Header SERVER-TO-SERVER phải khai lý do, và khai rồi thì hết vi phạm.
//
// `X-Signature` của webhook là ca thật: đặc tả khai nó, mà cho nó vào CORS
// sẽ là nói với mọi trang web rằng chữ ký webhook gửi được từ trình duyệt.
func TestHeaderServerToServerKhaiLyDoThiHetViPham(t *testing.T) {
	goc := duAn(t, openapiMotTuyen,
		map[string]string{"things.yaml": pathsMotTuyen}, goMotTuyen)
	ghiDe(t, goc, "internal/platform/httpserver/cors.go", corsGo("Authorization"))

	c := chayHeader(t, goc,
		map[string]string{"Authorization": "chuẩn"},
		map[string]string{"X-Thing-Id": "webhook, không qua trình duyệt"})
	if len(c.viPham) != 0 {
		t.Fatalf("khai lý do rồi mà vẫn báo: %v", c.viPham)
	}
}

// Dòng miễn trừ CHẾT cũng phải bị bắt — ở cả hai sổ.
func TestBatDongMienTruHeaderDaChet(t *testing.T) {
	goc := duAn(t, openapiMotTuyen,
		map[string]string{"things.yaml": pathsMotTuyen}, goMotTuyen)

	c := chayHeader(t, goc,
		map[string]string{"X-Da-Bo": "CORS không còn cho phép"},
		map[string]string{"X-Cung-Da-Bo": "đặc tả không còn khai"})
	if len(c.viPham) != 2 {
		t.Fatalf("mong 2 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
	got := strings.Join(c.viPham, "\n")
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
func TestHeaderKhongPhanBietHoaThuong(t *testing.T) {
	goc := duAn(t, openapiMotTuyen,
		map[string]string{"things.yaml": pathsMotTuyen}, goMotTuyen)
	ghiDe(t, goc, "internal/platform/httpserver/cors.go", corsGo("X-THING-ID"))

	if c := chayHeader(t, goc, nil, nil); len(c.viPham) != 0 {
		t.Fatalf("khác hoa thường bị coi là khác header: %v", c.viPham)
	}
}

// `in: header` KHÔNG tìm được tên thì BÁO LỖI, không bỏ qua.
//
// Cùng nguyên tắc với bộ đọc đặc tả: bỏ qua âm thầm là dựng lại đúng vấn
// đề công cụ này sinh ra để sửa.
func TestHeaderKhongTimDuocTenThiBaoLoi(t *testing.T) {
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
	goc := duAn(t, openapiMotTuyen, map[string]string{"things.yaml": paths}, goMotTuyen)

	c := &checker{
		root:                     goc,
		chuaCai:                  map[string]string{},
		ngoaiHopDong:             map[string]string{},
		headerNgoaiDacTa:         map[string]string{},
		headerKhongQuaTrinhDuyet: map[string]string{},
	}
	err := c.run()
	if err == nil {
		t.Fatal("khối `in: header` không có tên mà công cụ vẫn báo đọc xong")
	}
	if !strings.Contains(err.Error(), "name:") {
		t.Errorf("lỗi không nói rõ thiếu gì: %v", err)
	}
}
