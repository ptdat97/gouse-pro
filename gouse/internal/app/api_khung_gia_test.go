package app

import (
	"context"
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	marketapp "github.com/fashion-commerce/platform/internal/modules/marketplace/application"
	marketdom "github.com/fashion-commerce/platform/internal/modules/marketplace/domain"
	pricingapp "github.com/fashion-commerce/platform/internal/modules/pricing/application"
)

// Khung giá phải CHẶN offer giá ngoài khung, ngay lúc GHI.
//
// # P3-79 — một bảng có mục đích mà không ai đọc
//
// `price_constraint` có từ migration 000003 và mang mục đích ghi rõ trong tài
// liệu: *"khung giá ràng buộc seller"*. `pricing.ValidateSellerPrice` cũng đã
// cài đủ, có test ở hai tầng, và chú thích của nó nói đúng điều cần làm:
// *"Module marketplace gọi TRƯỚC khi cho seller lưu offer — đây là hàng rào
// chống bán phá giá VÀ chống lỗi nhập liệu (gõ thiếu một số 0)"*.
//
// Tới 01/10/2026 nó **chưa bao giờ có bên gọi**. Nhà bán đặt giá ngoài khung
// vẫn qua, và không gì báo.
//
// # Vì sao chặn lúc GHI, không phát hiện sau (ADR-0023)
//
// Giá sai lên sàn nghĩa là khách có thể đã mua ở giá đó. Lúc ấy không có
// đường sửa nào ngoài hủy đơn hoặc chịu lỗ — PH-41 đã rút ra quy tắc ấy cho
// dữ liệu tiền: *"phát hiện xong thì SỬA được không?"*, trả lời "không, chỉ
// đảo được" thì việc cần làm là chặn.
//
// # Vì sao bài này gọi service, không qua HTTP
//
// `CreateOffer` của nhà bán chưa có route. Gọi service là đường production
// thật hiện có — và nó vẫn đi qua đúng chỗ cần kiểm: adapter
// marketplace → pricing.
func TestGiaNgoaiKhungBiChanLucGhi(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	B := dungNhaBan(t, a, "khunggia"+ids.MustNew(ids.PrefixRequest).String()[24:])
	skuID, khung := a.skuThuongHieuMoCoKhungGia(t)

	taoOffer := func(gia int64) error {
		_, err := a.mods.marketplace.Service().CreateOffer(ctx,
			marketapp.CreateOfferInput{
				SKUID:             ids.ID(skuID),
				SellerID:          ids.ID(B.sellerID),
				Price:             money.MustNew(gia, money.VND),
				Condition:         marketdom.ConditionNew,
				HandlingTimeHours: 24,
				MinOrderQuantity:  1,
			})
		return err
	}

	// ---- Dưới sàn: CHẶN ----
	//
	// Đây là trường hợp bán phá giá, và cũng là trường hợp gõ thiếu một số 0.
	duoiSan := khung.min / 10
	err := taoOffer(duoiSan)
	var ngoaiKhung *marketapp.GiaNgoaiKhungError
	if !errors.As(err, &ngoaiKhung) {
		t.Fatalf("giá %d (khung %d–%d) phải bị CHẶN, nhận err=%v — "+
			"price_constraint lại không ai đọc",
			duoiSan, khung.min, khung.max, err)
	}
	if ngoaiKhung.Ma == "" {
		t.Error("lỗi không mang MÃ máy đọc được — giao diện không biết chỉ " +
			"vào đâu để nhà bán sửa")
	}

	// ---- Trên trần: CHẶN ----
	tranCao := khung.max * 10
	if err := taoOffer(tranCao); !errors.As(err, &ngoaiKhung) {
		t.Fatalf("giá %d (khung %d–%d) phải bị CHẶN, nhận err=%v",
			tranCao, khung.min, khung.max, err)
	}

	// ---- Trong khung: ĐI QUA ----
	//
	// Khẳng định này quan trọng ngang hai cái trên: một hàng rào chặn mọi
	// thứ cũng "xanh" ở hai bài đầu, và nó làm nhà bán không đăng được gì.
	trongKhung := (khung.min + khung.max) / 2
	if err := taoOffer(trongKhung); err != nil {
		if errors.As(err, &ngoaiKhung) {
			t.Fatalf("giá %d NẰM TRONG khung %d–%d mà bị chặn — hàng rào "+
				"chặn cả giá hợp lệ", trongKhung, khung.min, khung.max)
		}
		t.Fatalf("tạo offer giá hợp lệ: %v", err)
	}
}

// Đổi giá cũng phải qua khung, không chỉ lúc tạo.
//
// `UpdatePrice` là đường đi vào vùng ngoài khung DỄ NHẤT: tạo offer giá hợp
// lệ rồi sửa xuống. Kiểm ở một chỗ mà bỏ chỗ kia là một hàng rào có cửa sau.
func TestDoiGiaCungPhaiQuaKhungGia(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	B := dungNhaBan(t, a, "doigia"+ids.MustNew(ids.PrefixRequest).String()[24:])
	skuID, khung := a.skuThuongHieuMoCoKhungGia(t)

	o, err := a.mods.marketplace.Service().CreateOffer(ctx,
		marketapp.CreateOfferInput{
			SKUID:             ids.ID(skuID),
			SellerID:          ids.ID(B.sellerID),
			Price:             money.MustNew((khung.min+khung.max)/2, money.VND),
			Condition:         marketdom.ConditionNew,
			HandlingTimeHours: 24,
			MinOrderQuantity:  1,
		})
	if err != nil {
		t.Fatalf("tạo offer giá hợp lệ: %v", err)
	}

	_, err = a.mods.marketplace.Service().UpdatePrice(ctx, o.ID(),
		money.MustNew(khung.min/10, money.VND), money.Money{},
		ids.ID(B.sellerID))

	var ngoaiKhung *marketapp.GiaNgoaiKhungError
	if !errors.As(err, &ngoaiKhung) {
		t.Fatalf("SỬA giá xuống %d (khung %d–%d) phải bị chặn, nhận err=%v — "+
			"kiểm lúc tạo mà bỏ lúc sửa là hàng rào có cửa sau",
			khung.min/10, khung.min, khung.max, err)
	}

	// Và giá cũ phải GIỮ NGUYÊN: một lời gọi bị từ chối không được để lại
	// trạng thái nửa chừng.
	sau, err := a.mods.marketplace.Service().GetOffer(ctx, o.ID())
	if err != nil {
		t.Fatalf("đọc lại offer: %v", err)
	}
	if sau.Price().Amount() != o.Price().Amount() {
		t.Errorf("giá đổi thành %d dù lời gọi bị từ chối, cần giữ %d",
			sau.Price().Amount(), o.Price().Amount())
	}
}

type khungGia struct{ min, max int64 }

// skuThuongHieuMoCoKhungGia dựng một SKU thương hiệu MỞ kèm khung giá.
//
// # Vì sao phải dựng thay vì dùng SKU mẫu có sẵn
//
// SKU mẫu duy nhất có khung giá thuộc một thương hiệu HẠN CHẾ
// (`RESTRICTED_TO_DESIGNATED_SELLER`), nên hàng rào chống hàng giả — bước 3
// của `CreateOffer` — chặn trước khi tới bước khung giá. Bài test sẽ đỏ vì
// một lý do KHÁC, và đó là cách một phép thử nói về chính nó chứ không về
// thứ nó định kiểm.
//
// Thứ tự ấy ĐÚNG và không đổi: chống hàng giả là rủi ro sống còn, nó phải
// chặn trước. Nên bài test tự dựng dữ liệu đi tới được bước 4.
func (a *apiTest) skuThuongHieuMoCoKhungGia(t *testing.T) (string, khungGia) {
	t.Helper()
	ctx := context.Background()

	if a.mods.pricing == nil {
		t.Skip("module pricing cần kho lưu trữ postgres")
	}

	skuID := dungSkuThuongHieuMo(t, a)
	k := khungGia{min: 300_000, max: 800_000}

	if _, err := a.mods.pricing.Service().SetConstraint(ctx,
		pricingapp.SetConstraintInput{
			SKUID:          ids.ID(skuID),
			MinPrice:       money.MustNew(k.min, money.VND),
			MaxPrice:       money.MustNew(k.max, money.VND),
			ReferencePrice: money.MustNew((k.min+k.max)/2, money.VND),
		}); err != nil {
		t.Fatalf("đặt khung giá: %v", err)
	}
	return skuID, k
}
