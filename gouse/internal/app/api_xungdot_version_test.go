package app

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// mienTruXungDot là những module CỐ Ý không ánh xạ ErrVersionConflict ở
// tầng HTTP, kèm lý do.
//
// Danh sách này phải NGẮN và mỗi dòng phải giải thích được. Một danh sách
// miễn trừ dài là dấu hiệu quy tắc sai, không phải dấu hiệu nhiều ngoại lệ.
var mienTruXungDot = map[string]string{
	"payment": "ErrVersionConflict của payment chỉ phát sinh khi ghi đợt " +
		"đối soát, mà đường đó do worker chạy chứ không có endpoint HTTP " +
		"nào chạm tới (payment chỉ có 3 route đọc + ledger adjustment). " +
		"Xung đột ở đường worker được outbox giao lại — đúng cơ chế cho " +
		"tầng đó. Thêm ánh xạ HTTP sẽ là mã chết.",
}

// TestMoiModuleAnhXaXungDotVersionThanh409 — PH-6, phần "chuẩn hóa xử lý
// thất bại".
//
// # Vì sao 500 là câu trả lời sai
//
// `apierror.From` biến mọi lỗi chưa ánh xạ thành `INTERNAL_ERROR` (500).
// Với xung đột phiên bản, đó là sai theo HAI hướng cùng lúc:
//
//   - người gọi tưởng hệ thống hỏng nên KHÔNG thử lại, trong khi thử lại
//     chính là việc đúng cần làm
//   - giám sát kêu báo động cho một tình huống bình thường dưới tải, và
//     một cảnh báo kêu sai vài lần là cảnh báo không ai đọc nữa
//
// Xung đột phiên bản là 409: dữ liệu vừa đổi ở đường khác, tải lại rồi
// thử lại.
//
// # Vì sao quét mã nguồn chứ không gọi từng endpoint
//
// Gây ra xung đột THẬT ở mọi endpoint đòi dựng một cuộc đua riêng cho từng
// đường — nhiều công, dễ vỡ, và vẫn bỏ sót đường mới thêm. Kiểu hỏng cần
// chặn ở đây rất hẹp và rất dễ nhận: QUÊN ánh xạ. Quét mã bắt đúng nó.
//
// Bài `TestSuaHoSoSongSongKhongTra500` bên dưới đo hành vi thật trên một
// đường cụ thể, để bài quét này không chỉ là kiểm tra chuỗi ký tự.
func TestMoiModuleAnhXaXungDotVersionThanh409(t *testing.T) {
	goc := filepath.Join("..", "modules")
	ds, err := os.ReadDir(goc)
	if err != nil {
		t.Fatalf("đọc thư mục module: %v", err)
	}

	var thieu, daKiem []string
	for _, e := range ds {
		if !e.IsDir() {
			continue
		}
		mod := e.Name()
		thuMuc := filepath.Join(goc, mod)

		if !dinhNghiaXungDot(t, thuMuc) {
			continue
		}
		if !coRouteGhi(t, filepath.Join(thuMuc, "interfaces", "http")) {
			continue
		}
		if lyDo, mienTru := mienTruXungDot[mod]; mienTru {
			t.Logf("miễn trừ %s: %s", mod, lyDo)
			continue
		}
		daKiem = append(daKiem, mod)
		if !anhXaXungDot(t, filepath.Join(thuMuc, "interfaces", "http")) {
			thieu = append(thieu, mod)
		}
	}

	// In ra phạm vi phủ. Một bài quét không nói nó quét được gì thì không
	// phân biệt được "mọi module đều đúng" với "không module nào lọt lưới
	// vì bộ lọc hỏng".
	t.Logf("đã kiểm %d module: %s", len(daKiem), strings.Join(daKiem, ", "))
	if len(daKiem) < 3 {
		t.Errorf("chỉ kiểm được %d module — bộ lọc nhiều khả năng đang hỏng",
			len(daKiem))
	}

	if len(thieu) > 0 {
		t.Errorf("%d module có đường HTTP GHI và có khóa lạc quan nhưng "+
			"KHÔNG ánh xạ ErrVersionConflict — xung đột sẽ ra 500:\n  %s",
			len(thieu), strings.Join(thieu, "\n  "))
	}
}

// khaiBaoXungDot khớp khai báo ErrVersionConflict.
//
// `\s+` chứ không phải một dấu cách: gofmt CĂN CỘT các khai báo trong cùng
// khối var, nên chuỗi thật là `ErrVersionConflict   = errors.New`. Bản đầu
// của bài này khớp đúng một dấu cách và vì thế bỏ qua gần hết số module —
// nó xanh trong khi phủ ít hơn hẳn điều nó tuyên bố.
var khaiBaoXungDot = regexp.MustCompile(`ErrVersionConflict\s+=\s+errors\.New`)

func dinhNghiaXungDot(t *testing.T, thuMuc string) bool {
	t.Helper()
	return quetTep(t, filepath.Join(thuMuc, "domain"), func(s string) bool {
		return khaiBaoXungDot.MatchString(s)
	})
}

var routeGhi = regexp.MustCompile(`mux\.Handle(?:Func)?\("(POST|PUT|PATCH|DELETE) `)

func coRouteGhi(t *testing.T, thuMuc string) bool {
	t.Helper()
	return quetTep(t, thuMuc, func(s string) bool {
		return routeGhi.MatchString(s)
	})
}

func anhXaXungDot(t *testing.T, thuMuc string) bool {
	t.Helper()
	return quetTep(t, thuMuc, func(s string) bool {
		return strings.Contains(s, "ErrVersionConflict")
	})
}

// quetTep đọc mọi tệp .go KHÔNG phải test trong thư mục và hỏi `hop`.
func quetTep(t *testing.T, thuMuc string, hop func(string) bool) bool {
	t.Helper()
	ds, err := os.ReadDir(thuMuc)
	if err != nil {
		return false
	}
	for _, e := range ds {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") ||
			strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(thuMuc, e.Name()))
		if err != nil {
			t.Fatalf("đọc %s: %v", e.Name(), err)
		}
		if hop(string(b)) {
			return true
		}
	}
	return false
}

// TestSuaHoSoSongSongKhongTra500 đo HÀNH VI THẬT trên một đường cụ thể.
//
// Nhiều request sửa hồ sơ chạy song song trên cùng một khách: bên thắng
// ghi được, bên thua gặp xung đột phiên bản. Bên thua phải nhận 4xx —
// không bao giờ 5xx. Đây là chuyện xảy ra thật khi khách mở hai tab, hoặc
// bấm Lưu hai lần vì lần đầu chậm.
//
// # Hai cách bài này đã từng xanh mà không kiểm gì
//
// Cả hai đều đáng ghi lại, vì cùng một kiểu: test chạy, test xanh, test
// không chạm tới thứ nó tuyên bố đo.
//
//  1. Dùng CHUNG một khóa idempotency cho mọi request — lớp chống lặp gộp
//     tất cả thành một lượt ghi, không có xung đột nào xảy ra.
//  2. Dùng helper song song sẵn có, nhưng helper đó không nhận header nên
//     không mang token: cả 8 request trả 401 và không request nào tới
//     được handler.
//
// Vì thế bài này tự dựng vòng song song (token + khóa riêng cho từng
// request) và KHẲNG ĐỊNH có ít nhất một request đi lọt qua xác thực.
func TestSuaHoSoSongSongKhongTra500(t *testing.T) {
	a := newAPITest(t)
	tok := a.dangKyVaDangNhap(emailMoi("songsong"))

	// Dùng helper song song CHỤP ẢNH cookie, không gọi `a.call` trong
	// goroutine.
	//
	// Bản đầu tự dựng vòng lặp gọi thẳng `a.call`, và đó là ĐUA DỮ LIỆU
	// thật: `a.call` ghi vào hũ cookie dùng chung của apiTest. Nó chỉ lộ
	// ra khi chạy `-race` trên CẢ gói, không phải khi chạy riêng bài này.
	ketQua := a.goiSongSongCoHeader(8, http.MethodPatch, "/api/v1/me",
		map[string]any{"name": "Tên Mới", "phone": "0900777888"},
		map[string]string{"Authorization": "Bearer " + tok})

	var quaDuocXacThuc int
	for i, r := range ketQua {
		if r.code != http.StatusUnauthorized {
			quaDuocXacThuc++
		}
		if r.code >= 500 {
			t.Errorf("request %d nhận HTTP %d — xung đột phiên bản KHÔNG "+
				"phải lỗi hệ thống, và 500 khiến người gọi không thử lại "+
				"trong khi thử lại mới là việc đúng: %s", i, r.code, r.raw)
		}
	}

	// Chống xanh rỗng: nếu mọi request dừng ở cửa xác thực thì bài này
	// không đo được gì về xung đột phiên bản.
	if quaDuocXacThuc == 0 {
		t.Fatalf("không request nào qua được xác thực — bài test không "+
			"chạm tới handler, và khẳng định \"không 500\" là vô nghĩa: %s",
			ketQua[0].raw)
	}
}
