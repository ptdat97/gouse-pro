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

	// …và một schemas.yaml, vì tầng DTO luôn đọc nó.
	//
	// Dựng ở đây chứ không bắt từng bài tự thêm: thêm tầng thứ năm sau này
	// sẽ lại làm mọi bài cũ vỡ vì cùng một lý do, và lần nào cũng phải sửa
	// hàng chục chỗ. Đã mắc đúng lỗi này với `archcheck` cùng ngày.
	must(filepath.Join(goc, "api", "components", "schemas.yaml"), "")
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
		capEnum:                  capEnumDaKiem,
		enumKhongGhep:            enumKhongGhep,
		nguongEnumChuaGac:        soEnumChuaGac,
		capDTO:                   capDTODaKiem,
		nguongDTOChuaGac:         soDTOChuaGac,
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

// --------------------------------------------------------------- Tầng ENUM

// chayEnum chạy công cụ với sổ ghép enum truyền vào.
//
// `nguong` phải truyền tay từng bài: chốt của dự án thật (63) sẽ biến mọi
// bài ở đây thành "thiếu 63 enum chưa gác" và không bài nào đo được thứ nó
// muốn đo.
func chayEnum(
	t *testing.T, goc string, cap []capEnum, khongGhep map[string]string, nguong int,
) *checker {
	t.Helper()
	if khongGhep == nil {
		khongGhep = map[string]string{}
	}
	c := &checker{
		root:                     goc,
		chuaCai:                  map[string]string{},
		ngoaiHopDong:             map[string]string{},
		headerNgoaiDacTa:         map[string]string{},
		headerKhongQuaTrinhDuyet: map[string]string{},
		capEnum:                  cap,
		enumKhongGhep:            khongGhep,
		nguongEnumChuaGac:        nguong,
	}
	if err := c.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	return c
}

// duongMau là đường dẫn mà cả hai cách viết YAML dưới đây phải cho ra.
const duongMau = "components/mau.yaml#/schemas/Color/properties/color_family"

// enumNhieuDong là dạng danh sách — cách đặc tả thật viết.
const enumNhieuDong = `schemas:
  Color:
    properties:
      color_family:
        type: string
        enum:
          - WHITE
          - GREY
`

// enumMotDong là dạng gọn `[A, B]` — YAML hợp lệ như nhau.
const enumMotDong = `schemas:
  Color:
    properties:
      color_family:
        type: string
        enum: [WHITE, GREY]
`

const goMau = `package domain

type NhomMau string

const (
	MauTrang NhomMau = "WHITE"
	MauXam   NhomMau = "GREY"
)
`

// capMau là sổ ghép trỏ đúng vào cặp trên.
func capMau(tapCon bool) []capEnum {
	return []capEnum{{duongMau, "thing/domain", "NhomMau", tapCon, "nhóm màu"}}
}

// duAnMau dựng dự án giả có một enum trong đặc tả và một kiểu trong Go.
func duAnMau(t *testing.T, yaml, goSrc string) string {
	t.Helper()
	goc := duAn(t, openapiMotTuyen,
		map[string]string{"things.yaml": pathsMotTuyen}, goMotTuyen)
	ghiDe(t, goc, "api/components/mau.yaml", yaml)
	ghiDe(t, goc, "internal/thing/domain/mau.go", goSrc)
	return goc
}

func TestEnumKhopThiKhongBaoViPham(t *testing.T) {
	goc := duAnMau(t, enumNhieuDong, goMau)

	if c := chayEnum(t, goc, capMau(false), nil, 0); len(c.viPham) != 0 {
		t.Fatalf("mong 0 vi phạm, nhận: %v", c.viPham)
	}
}

// HAI cách viết YAML của cùng một thứ phải cho CÙNG một đường dẫn.
//
// Nếu không, sổ ghép phải biết đặc tả viết kiểu nào — một chi tiết không ai
// nhớ, và đổi cách viết sẽ lặng lẽ tháo hàng rào ra khỏi enum đó.
func TestEnumVietMotDongCungDuongDanVoiNhieuDong(t *testing.T) {
	goc := duAnMau(t, enumMotDong, goMau)

	if c := chayEnum(t, goc, capMau(false), nil, 0); len(c.viPham) != 0 {
		t.Fatalf("dạng một dòng phải khớp cùng sổ, nhận: %v", c.viPham)
	}
}

// ĐÂY LÀ LỖI THẬT: đặc tả nói `GRAY`, máy chủ lưu `GREY`.
//
// Bộ lọc theo màu im lặng trả rỗng — không lỗi, không log, không ai biết.
// Một chữ cái, và cả hai chiều đều sai cùng lúc.
func TestBatLechMotChuCai(t *testing.T) {
	goc := duAnMau(t, strings.Replace(enumNhieuDong, "GREY", "GRAY", 1), goMau)

	c := chayEnum(t, goc, capMau(false), nil, 0)
	if len(c.viPham) != 2 {
		t.Fatalf("mong 2 vi phạm (mỗi chiều một), nhận %d: %v", len(c.viPham), c.viPham)
	}
	gop := strings.Join(c.viPham, "\n")
	if !strings.Contains(gop, "GRAY") || !strings.Contains(gop, "GREY") {
		t.Fatalf("phải gọi tên CẢ HAI giá trị lệch: %v", c.viPham)
	}
}

// Đặc tả khai giá trị Go không sinh ra: client viết một nhánh không bao giờ
// chạy tới. KHÔNG BAO GIỜ hợp lệ — kể cả khi cặp cho phép tập con.
func TestBatDacTaKhaiGiaTriGoKhongCo(t *testing.T) {
	yaml := strings.Replace(enumNhieuDong, "          - GREY",
		"          - GREY\n          - MULTI", 1)
	goc := duAnMau(t, yaml, goMau)

	for _, tapCon := range []bool{false, true} {
		c := chayEnum(t, goc, capMau(tapCon), nil, 0)
		if len(c.viPham) != 1 {
			t.Fatalf("tậpCon=%v: mong 1 vi phạm, nhận %d: %v",
				tapCon, len(c.viPham), c.viPham)
		}
		if !strings.Contains(c.viPham[0], "MULTI") {
			t.Fatalf("tậpCon=%v: phải gọi tên MULTI: %v", tapCon, c.viPham[0])
		}
	}
}

// ĐÂY LÀ LỖI THẬT: `account_type` khai 9 tài khoản, Go có 13.
//
// Client sinh kiểu từ đặc tả thu hẹp kiểu sai, rồi vỡ khi gặp response thật.
func TestBatGoCoMaDacTaThieu(t *testing.T) {
	goSrc := strings.Replace(goMau, `	MauXam   NhomMau = "GREY"`,
		`	MauXam   NhomMau = "GREY"`+"\n"+`	MauBac   NhomMau = "SILVER"`, 1)
	goc := duAnMau(t, enumNhieuDong, goSrc)

	c := chayEnum(t, goc, capMau(false), nil, 0)
	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
	if !strings.Contains(c.viPham[0], "SILVER") {
		t.Fatalf("phải gọi tên SILVER: %v", c.viPham[0])
	}

	// …trừ khi cặp KHAI rằng đặc tả hẹp hơn là đúng — như `tier` của `/me`
	// không bao giờ trả `ANONYMIZED`.
	if c := chayEnum(t, goc, capMau(true), nil, 0); len(c.viPham) != 0 {
		t.Fatalf("khai ChoPhepTapCon rồi thì phải im: %v", c.viPham)
	}
}

// Đọc AST chứ không grep.
//
// Một giá trị nằm trong bình luận hay trong thông điệp lỗi KHÔNG phải một
// giá trị hợp lệ của kiểu. Nếu công cụ grep, nó sẽ đòi đặc tả khai thêm
// `BLACK` — một cảnh báo giả, và cảnh báo giả làm người ta tắt phép kiểm.
func TestGiaTriTrongBinhLuanVaChuoiKhongTinh(t *testing.T) {
	goSrc := goMau + `
// MauDen NhomMau = "BLACK" — bỏ đi vì kho không còn phân loại theo đen.

const ThongBaoLoi = "nhóm màu hợp lệ: WHITE, GREY, BLACK"
`
	goc := duAnMau(t, enumNhieuDong, goSrc)

	if c := chayEnum(t, goc, capMau(false), nil, 0); len(c.viPham) != 0 {
		t.Fatalf("BLACK trong bình luận/chuỗi không phải giá trị: %v", c.viPham)
	}
}

// Sổ ghép trỏ tới đường dẫn KHÔNG CÒN là một dòng nói dối.
//
// Nó im lặng ngừng gác enum ấy trong khi vẫn trông như đang gác — đúng kiểu
// hỏng mà `types:check` đã mắc.
func TestBatSoGhepTroToiDuongDanChet(t *testing.T) {
	goc := duAnMau(t, enumNhieuDong, goMau)
	chet := []capEnum{{
		"components/mau.yaml#/schemas/ColorInfo/properties/color_family",
		"thing/domain", "NhomMau", false, "nhóm màu",
	}}

	// Enum thật lúc này KHÔNG được gác, nên chốt để 1 để đo đúng một thứ.
	c := chayEnum(t, goc, chet, nil, 1)
	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
	if !strings.Contains(c.viPham[0], "ColorInfo") {
		t.Fatalf("phải gọi tên đường dẫn chết: %v", c.viPham[0])
	}
}

// Sổ miễn trừ cũng phải chết theo enum nó miễn trừ.
func TestBatDongKhongGhepDaChet(t *testing.T) {
	goc := duAnMau(t, enumNhieuDong, goMau)
	c := chayEnum(t, goc, capMau(false),
		map[string]string{"components/mau.yaml#/schemas/DaXoa/properties/x": "lý do cũ"}, 0)

	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
	if !strings.Contains(c.viPham[0], "DaXoa") {
		t.Fatalf("phải gọi tên dòng chết: %v", c.viPham[0])
	}
}

// Chốt CHỈ ĐƯỢC GIẢM.
//
// Một chốt cao hơn thực tế là chỗ trống để lặng lẽ thêm enum không ai gác —
// nó không đỏ, nên không ai biết hàng rào đã nới ra.
func TestChotEnumChuaGacChiDuocGiam(t *testing.T) {
	goc := duAnMau(t, enumNhieuDong, goMau)

	c := chayEnum(t, goc, capMau(false), nil, 5)
	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
	if !strings.Contains(c.viPham[0], "Hạ chốt") {
		t.Fatalf("phải bảo hạ chốt: %v", c.viPham[0])
	}

	// Thêm enum không gác mà chốt vẫn thấp thì cũng đỏ, ở chiều kia.
	if c := chayEnum(t, goc, nil, nil, 0); len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm ở chiều vượt chốt, nhận %d: %v",
			len(c.viPham), c.viPham)
	}
}

// Đổi tên kiểu Go mà quên sửa sổ: công cụ phải KÊU, không được im.
func TestBatKieuGoDoiTen(t *testing.T) {
	goc := duAnMau(t, enumNhieuDong, strings.ReplaceAll(goMau, "NhomMau", "HoMau"))

	c := chayEnum(t, goc, capMau(false), nil, 0)
	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
	if !strings.Contains(c.viPham[0], "NhomMau") {
		t.Fatalf("phải gọi tên kiểu không tìm thấy: %v", c.viPham[0])
	}
}

// ---------------------------------------------------------------- Tầng DTO

// chayDTO chạy công cụ với sổ ghép DTO truyền vào.
//
// `nguong` truyền tay từng bài: chốt của dự án thật (20) sẽ biến mọi bài ở
// đây thành "thiếu 20 schema chưa gác".
func chayDTO(t *testing.T, goc string, cap []capDTO, nguong int) *checker {
	t.Helper()
	c := &checker{
		root:                     goc,
		chuaCai:                  map[string]string{},
		ngoaiHopDong:             map[string]string{},
		headerNgoaiDacTa:         map[string]string{},
		headerKhongQuaTrinhDuyet: map[string]string{},
		capDTO:                   cap,
		nguongDTOChuaGac:         nguong,
	}
	if err := c.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	return c
}

// schemaHang là một schema có ba thuộc tính, dạng thật của đặc tả.
const schemaHang = `Hang:
  type: object
  required: [id, ten]
  description: |
    Một mô tả nhiều dòng, để chắc rằng phép đọc không nhầm nó thành
    thuộc tính.
  properties:
    id:
      $ref: './common.yaml#/schemas/Id'
    ten:
      type: string
    nhom:
      type: object
      description: Object LỒNG — thuộc tính của nó KHÔNG phải của Hang.
      properties:
        ma:
          type: string
        nhan:
          type: string
`

// dtoHang dựng struct Go phục vụ schema trên.
func dtoHang(truong ...string) string {
	s := "package http\n\ntype hangJSON struct {\n"
	for _, t := range truong {
		s += "\tX" + t + " string `json:\"" + t + "\"`\n"
	}
	return s + "}\n"
}

// duAnDTO dựng dự án giả có schemas.yaml và một gói DTO.
func duAnDTO(t *testing.T, schema, dto string) string {
	t.Helper()
	goc := duAn(t, openapiMotTuyen,
		map[string]string{"things.yaml": pathsMotTuyen}, goMotTuyen)
	ghiDe(t, goc, "api/components/schemas.yaml", schema)
	ghiDe(t, goc, "internal/modules/hang/interfaces/http/dto.go", dto)
	return goc
}

func capHang(choPhepThieu map[string]string) []capDTO {
	return []capDTO{{
		Schema: "Hang", GoiGo: "modules/hang/interfaces/http",
		KieuGo: "hangJSON", ChoPhepThieu: choPhepThieu, LyDo: "hàng thử",
	}}
}

func TestDTOKhopThiKhongBaoViPham(t *testing.T) {
	goc := duAnDTO(t, schemaHang, dtoHang("id", "ten", "nhom"))

	if c := chayDTO(t, goc, capHang(nil), 0); len(c.viPham) != 0 {
		t.Fatalf("mong 0 vi phạm, nhận: %v", c.viPham)
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
func TestDTOThuocTinhLongKhongTinh(t *testing.T) {
	goc := duAnDTO(t, schemaHang, dtoHang("id", "ten", "nhom"))

	c := chayDTO(t, goc, capHang(nil), 0)
	for _, v := range c.viPham {
		if strings.Contains(v, "ma") || strings.Contains(v, "nhan") {
			t.Fatalf("thuộc tính lồng bị tính vào schema ngoài: %s", v)
		}
	}
}

// ĐÂY LÀ DẠNG LỖI đã xảy ra năm lần: đặc tả có, DTO không có.
func TestDTOBatTruongDacTaCoMaGoThieu(t *testing.T) {
	goc := duAnDTO(t, schemaHang, dtoHang("id", "ten")) // thiếu `nhom`

	c := chayDTO(t, goc, capHang(nil), 0)
	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
	if !strings.Contains(c.viPham[0], "nhom") {
		t.Fatalf("phải gọi tên trường thiếu: %s", c.viPham[0])
	}
}

// Chiều ngược: Go trả trường mà hợp đồng không nhắc.
//
// Không ai biết nó tồn tại để dùng, và xóa nó đi là thay đổi PHÁ VỠ không
// ai nhận ra.
func TestDTOBatTruongGoTraNgoaiHopDong(t *testing.T) {
	goc := duAnDTO(t, schemaHang, dtoHang("id", "ten", "nhom", "bi_mat"))

	c := chayDTO(t, goc, capHang(nil), 0)
	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
	if !strings.Contains(c.viPham[0], "bi_mat") {
		t.Fatalf("phải gọi tên trường thừa: %s", c.viPham[0])
	}
}

// `ChoPhepThieu` tắt được một trường, và CHỈ trường ấy.
func TestDTOChoPhepThieuChiTatMotTruong(t *testing.T) {
	goc := duAnDTO(t, schemaHang, dtoHang("id")) // thiếu `ten` và `nhom`

	c := chayDTO(t, goc, capHang(map[string]string{"nhom": "chưa làm"}), 0)
	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm (còn `ten`), nhận %d: %v",
			len(c.viPham), c.viPham)
	}
	if !strings.Contains(c.viPham[0], "ten") ||
		strings.Contains(c.viPham[0], "nhom") {
		t.Fatalf("phải còn kêu `ten` và im về `nhom`: %s", c.viPham[0])
	}
}

// Hai cách một dòng `ChoPhepThieu` thành NÓI DỐI.
func TestDTOChoPhepThieuNoiDoiThiBiBat(t *testing.T) {
	t.Run("trường không còn trong đặc tả", func(t *testing.T) {
		goc := duAnDTO(t, schemaHang, dtoHang("id", "ten", "nhom"))
		c := chayDTO(t, goc, capHang(map[string]string{"da_xoa": "lý do cũ"}), 0)
		if len(c.viPham) != 1 || !strings.Contains(c.viPham[0], "da_xoa") {
			t.Fatalf("mong 1 vi phạm về `da_xoa`, nhận: %v", c.viPham)
		}
	})

	t.Run("Go ĐÃ trả trường ấy", func(t *testing.T) {
		goc := duAnDTO(t, schemaHang, dtoHang("id", "ten", "nhom"))
		c := chayDTO(t, goc, capHang(map[string]string{"ten": "nói là chưa trả"}), 0)
		if len(c.viPham) != 1 || !strings.Contains(c.viPham[0], "ĐÃ trả") {
			t.Fatalf("mong 1 vi phạm \"Go ĐÃ trả\", nhận: %v", c.viPham)
		}
	})
}

// Sổ ghép trỏ tới schema hoặc struct KHÔNG CÒN phải bị bắt.
func TestDTOSoGhepTroSaiThiBiBat(t *testing.T) {
	t.Run("schema không tồn tại", func(t *testing.T) {
		goc := duAnDTO(t, schemaHang, dtoHang("id", "ten", "nhom"))
		c := chayDTO(t, goc, []capDTO{{
			Schema: "KhongCo", GoiGo: "modules/hang/interfaces/http",
			KieuGo: "hangJSON", LyDo: "thử",
		}}, 1)
		if len(c.viPham) != 1 || !strings.Contains(c.viPham[0], "KhongCo") {
			t.Fatalf("mong 1 vi phạm về schema chết, nhận: %v", c.viPham)
		}
	})

	t.Run("struct không tồn tại", func(t *testing.T) {
		goc := duAnDTO(t, schemaHang, dtoHang("id", "ten", "nhom"))
		c := chayDTO(t, goc, []capDTO{{
			Schema: "Hang", GoiGo: "modules/hang/interfaces/http",
			KieuGo: "khongCoJSON", LyDo: "thử",
		}}, 0)
		if len(c.viPham) != 1 || !strings.Contains(c.viPham[0], "khongCoJSON") {
			t.Fatalf("mong 1 vi phạm về struct chết, nhận: %v", c.viPham)
		}
	})
}

// Chốt schema chưa gác CHỈ ĐƯỢC GIẢM.
func TestDTOChotChuaGacChiDuocGiam(t *testing.T) {
	goc := duAnDTO(t, schemaHang+`
HangKhac:
  type: object
  properties:
    x:
      type: string
`, dtoHang("id", "ten", "nhom"))

	// `HangKhac` chưa gác → chốt phải là 1.
	if c := chayDTO(t, goc, capHang(nil), 1); len(c.viPham) != 0 {
		t.Fatalf("chốt đúng thì phải im: %v", c.viPham)
	}
	if c := chayDTO(t, goc, capHang(nil), 0); len(c.viPham) != 1 {
		t.Fatalf("vượt chốt phải kêu: %v", c.viPham)
	}
	if c := chayDTO(t, goc, capHang(nil), 5); len(c.viPham) != 1 ||
		!strings.Contains(c.viPham[0], "Hạ chốt") {
		t.Fatalf("chốt cao hơn thực tế phải bảo hạ: %v", c.viPham)
	}
}
