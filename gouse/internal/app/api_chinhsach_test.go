package app

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
	"github.com/fashion-commerce/platform/internal/modules/identity"
	"github.com/fashion-commerce/platform/internal/modules/seller"
	"github.com/fashion-commerce/platform/internal/platform/opsconfig"
)

// Bộ kiểm cho các tham số CHÍNH SÁCH KINH DOANH vừa đưa lên giao diện.
//
// # Vì sao mỗi tham số cần một bài đi HẾT ĐƯỜNG
//
// Khai vào sổ đăng ký, dựng adapter, nối vào module — cả ba bước đều có
// thể đúng mà tham số vẫn không có tác dụng. `fulfillment.carrier_cost_*`
// đã khai đủ, nối đủ, và im lặng suốt vì một trường KHÁC rỗng; hai lý do
// độc lập cùng dẫn tới giá 0, và không chú thích nào nói ra.
//
// Nên mỗi bài dưới đây ĐỔI tham số qua đúng API quản trị mà người thật
// dùng, rồi đo ở chỗ NGƯỜI DÙNG NHÌN THẤY. Không bài nào gọi thẳng adapter.
//
// Xem docs/09-operations/cau-hinh-nghiep-vu.md.

const lyDoThu = "Kiem chung tham so chinh sach co tac dung that hay khong"

// phiCuaPhien mở một phiên thanh toán rồi trả về phí vận chuyển của nó.
func (a *apiTest) phiCuaPhien(t *testing.T, email, sdt, phuongThuc string) int64 {
	t.Helper()

	maOffer := a.timOfferBanDuoc()
	if maOffer == "" {
		t.Skip("không có offer nào bán được")
	}
	res := a.call(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": maOffer, "quantity": 1}, khoaIdem())
	gio, _ := res.body["cart"].(map[string]any)
	maGio, _ := gio["id"].(string)

	res = a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id": maGio, "guest_email": email, "guest_phone": sdt,
	}, khoaIdem())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	maPhien, _ := res.body["id"].(string)

	a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Phí", "phone": sdt,
			"street_address": "1 Đường Thử", "ward": "P1",
			"district": "Q1", "province": "TP.HCM", "country_code": "VN",
		}, khoaIdem())
	res = a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-method",
		map[string]any{"shipping_method": phuongThuc}, khoaIdem())
	if res.code != http.StatusOK {
		t.Fatalf("đặt phương thức giao: HTTP %d — %s", res.code, res.raw)
	}

	var phi int64
	if err := a.db.Pool().QueryRow(context.Background(),
		`SELECT shipping_fee FROM checkout WHERE id = $1`, maPhien).
		Scan(&phi); err != nil {
		t.Fatalf("đọc phí của phiên: %v", err)
	}
	return phi
}

// TestPhiVanChuyenDoiTheoCauHinh — mục 3 của bản kiểm kê.
//
// Phí này là con số HIỆN TRÊN MÀN HÌNH THANH TOÁN. Đổi nó là việc chạy
// khuyến mãi hoặc đàm phán lại với hãng — không việc nào nên chờ một lần
// triển khai.
func TestPhiVanChuyenDoiTheoCauHinh(t *testing.T) {
	a := newAPITest(t)
	tok := a.taoTaiKhoanVaiTro(t, identity.RoleAdmin)

	truoc := a.phiCuaPhien(t, "phi1@example.com", "0900010001", "STANDARD")
	if truoc != 30_000 {
		t.Fatalf("phí mặc định = %d, cần 30000 — mặc định phải đúng bằng "+
			"giá trị đang chạy TRƯỚC khi có tham số", truoc)
	}

	if res := a.datCauHinh(t, tok, opsconfig.KeyPhiGiaoTieuChuan, 25_000,
		lyDoThu); res.code != http.StatusOK {
		t.Fatalf("đặt phí: HTTP %d — %s", res.code, res.raw)
	}

	sau := a.phiCuaPhien(t, "phi2@example.com", "0900010002", "STANDARD")
	if sau != 25_000 {
		t.Errorf("sau khi đổi, phí thu của khách = %d, cần 25000 — tham số "+
			"không tới được màn hình thanh toán", sau)
	}
}

// TestSoNgayGiaoDoiTheoCauHinh — mục 3, nửa còn lại.
//
// Số ngày là LỜI HỨA với khách: nó thành ngày giao dự kiến trên trang theo
// dõi đơn, tính từ lúc bàn giao cho hãng.
func TestSoNgayGiaoDoiTheoCauHinh(t *testing.T) {
	a := newAPITest(t)
	tok := a.taoTaiKhoanVaiTro(t, identity.RoleAdmin)

	if res := a.datCauHinh(t, tok, opsconfig.KeyNgayGiaoTieuChuan, 5,
		lyDoThu); res.code != http.StatusOK {
		t.Fatalf("đặt số ngày: HTTP %d — %s", res.code, res.raw)
	}

	maDon := a.datDonCOD(t, "ngaygiao")
	a.phatEvent(t)
	foID, sellerID := a.donThucHienCuaDon(t, maDon)
	if foID == "" {
		t.Skip("không tạo được đơn thực hiện")
	}

	ctx := context.Background()
	for _, b := range []func() error{
		func() error { return a.mods.fulfillment.ConfirmFulfillment(ctx, sellerID, foID) },
		func() error { return a.mods.fulfillment.MarkPicking(ctx, sellerID, foID) },
		func() error { return a.mods.fulfillment.MarkPacked(ctx, sellerID, foID) },
	} {
		if err := b(); err != nil {
			t.Fatalf("chuyển trạng thái: %v", err)
		}
	}

	truocKhiBanGiao := time.Now().UTC()
	if err := a.mods.fulfillment.HandOverToCarrier(ctx,
		fulfillment.HandOverRequest{
			SellerID: sellerID, FulfillmentID: foID,
			Provider: "GHN", TrackingNumber: "GHN-NG-" + foID[4:14],
		}); err != nil {
		t.Fatalf("bàn giao: %v", err)
	}

	var ngayDuKien *time.Time
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT estimated_delivery_date FROM fulfillment_order WHERE id = $1`,
		foID).Scan(&ngayDuKien); err != nil {
		t.Fatalf("đọc ngày giao dự kiến: %v", err)
	}
	if ngayDuKien == nil {
		t.Fatal("không có ngày giao dự kiến sau khi bàn giao")
	}

	can := truocKhiBanGiao.AddDate(0, 0, 5).Format("2006-01-02")
	got := ngayDuKien.Format("2006-01-02")
	if got != can {
		t.Errorf("hứa giao ngày %s, cần %s (bàn giao + 5 ngày) — lời hứa "+
			"giao hàng không đổi theo cấu hình", got, can)
	}
}

// TestSanHoaHongChanDuyetDuoiSan — mục 5.
//
// Không có sàn thì duyệt nhầm một nhà bán ở 0% là nền tảng không thu được
// đồng nào trên MỌI đơn của họ, và không gì báo.
func TestSanHoaHongChanDuyetDuoiSan(t *testing.T) {
	a := newAPITest(t)
	tok := a.taoTaiKhoanVaiTro(t, identity.RoleAdmin)

	duyet := func(nhan string, tyLe int) reply {
		t.Helper()
		maNB := a.nhaBanChoDuyet(t, nhan, tok)
		h := khoaIdem()
		h["Authorization"] = "Bearer " + tok
		return a.call(http.MethodPost,
			"/api/v1/admin/sellers/"+maNB+"/approve",
			map[string]any{"commission_rate_bp": tyLe, "notes": "thử sàn"}, h)
	}

	// Sàn mặc định 0: duyệt ở 3% phải QUA — đúng bằng hành vi trước khi
	// có tham số này.
	if res := duyet("aduoi", 300); res.code != http.StatusOK {
		t.Fatalf("sàn mặc định 0 mà vẫn chặn duyệt ở 3%%: HTTP %d — %s",
			res.code, res.raw)
	}

	if res := a.datCauHinh(t, tok, opsconfig.KeyHoaHongSan, 500,
		lyDoThu); res.code != http.StatusOK {
		t.Fatalf("đặt sàn: HTTP %d — %s", res.code, res.raw)
	}

	if res := duyet("bduoi", 300); res.code == http.StatusOK {
		t.Error("sàn 5% mà vẫn duyệt được nhà bán ở 3% — nền tảng mất " +
			"hoa hồng trên mọi đơn của họ, và không gì báo")
	}

	// Sàn phải chặn ĐÚNG cái cần chặn, không phải chặn tất cả.
	if res := duyet("ctren", 800); res.code != http.StatusOK {
		t.Errorf("sàn 5%% mà chặn cả nhà bán ở 8%%: HTTP %d — %s",
			res.code, res.raw)
	}
}

// nhaBanChoDuyet NỘP một hồ sơ nhà bán mới và trả mã của nó.
//
// Nộp mới mỗi lần chứ không tìm hồ sơ có sẵn: bài kiểm duyệt ba lần với ba
// tỷ lệ khác nhau, và một hồ sơ chỉ duyệt được MỘT lần. Dùng lại hồ sơ cũ
// thì lần thứ hai hỏng vì sai trạng thái — một lý do chẳng liên quan gì
// tới sàn hoa hồng.
func (a *apiTest) nhaBanChoDuyet(t *testing.T, nhan, tokAdmin string) string {
	t.Helper()

	sel, err := a.mods.seller.ApplyAsSeller(context.Background(),
		seller.ApplicationRequest{
			Name:       "Xưởng " + nhan,
			Slug:       "xuong-" + nhan,
			SellerType: "BUSINESS",
			LegalName:  "Công ty TNHH " + nhan,
			TaxCode:    "0" + nhan[:1] + "12345678",
			Email:      nhan + "@example.com",
			Phone:      "0900" + nhan[:1] + "00000",
			BankAccount: seller.BankAccountInput{
				BankCode: "VCB", AccountNumber: "1234567890",
				AccountHolder: "CONG TY " + nhan,
			},
		})
	if err != nil {
		t.Fatalf("nộp hồ sơ nhà bán: %v", err)
	}

	// Đưa hồ sơ vào HÀNG ĐỢI RÀ SOÁT qua đúng tuyến quản trị.
	//
	// Trước đây đoạn này phải đi vòng bằng SQL: `SubmitForReview` có đủ ở
	// domain và application mà KHÔNG cửa nào ở production gọi tới, nên hồ
	// sơ nộp thật không bao giờ duyệt được.
	h := khoaIdem()
	h["Authorization"] = "Bearer " + tokAdmin
	if res := a.call(http.MethodPost,
		"/api/v1/admin/sellers/"+sel.ID+"/submit-review", nil, h); res.code != http.StatusOK {
		t.Fatalf("đưa hồ sơ vào hàng đợi rà soát: HTTP %d — %s", res.code, res.raw)
	}
	return sel.ID
}
