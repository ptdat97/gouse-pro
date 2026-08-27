package order_test

import (
	"context"
	"reflect"
	"testing"

	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	orderdom "github.com/fashion-commerce/platform/internal/modules/order/domain"
	orderpg "github.com/fashion-commerce/platform/internal/modules/order/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

// TestDocLaiDonKhongMatTruongNao — chặn CẢ LỚP lỗi, không chỉ hai trường.
//
// # Vì sao bài này tồn tại
//
// `withLines` dựng lại Order TỪ ĐẦU sau khi nạp dòng hàng, vì domain cố ý
// không có setter nào để gắn dòng vào đơn đã tạo. Cái giá là một danh sách
// phải chép tay, và trình biên dịch KHÔNG nhắc khi thiếu: một trường bỏ
// sót chỉ là giá trị rỗng hợp lệ.
//
// Đã hỏng hai lần:
//
//	quên Version           → mọi lần chuyển trạng thái TIẾP THEO thất bại
//	                         vì khóa lạc quan so với số 0
//	quên SourceCheckoutID  → bất biến "một phiên một đơn" mất chỗ dựa
//
// Bài này so SÁNH TỪNG TRƯỜNG giữa bản vừa ghi và bản đọc lại, bằng phản
// chiếu. Thêm trường mới vào Order mà quên `withLines` thì nó đỏ ngay,
// không cần ai nhớ ra.
func tien(n int64) money.Money { return money.MustNew(n, money.VND) }

func TestDocLaiDonKhongMatTruongNao(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	kho := orderpg.NewOrderStore(db.Pool())

	line, err := orderdom.NewLine(orderdom.NewLineParams{
		OfferID: ids.MustNew(ids.PrefixOffer), SKUID: ids.MustNew(ids.PrefixSKU),
		SellerID:    ids.MustNew(ids.PrefixSeller),
		ProductName: "Áo thử vòng đọc ghi", VariantDescription: "Trắng / M",
		UnitPrice: tien(199000), Quantity: 2, Now: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("dựng dòng hàng: %v", err)
	}

	goc, err := orderdom.NewOrder(orderdom.NewOrderParams{
		OrderNumber: "FC-TEST-" + ids.MustNew(ids.PrefixRequest).String()[22:],
		GuestEmail:  "vongdocghi@example.com",
		GuestPhone:  "0900999888",
		ShippingAddress: orderdom.Address{
			RecipientName: "Khách Thử", Phone: "0900999888",
			StreetAddress: "1 Đường Thử", Ward: "P1",
			District: "Q1", Province: "TP.HCM", CountryCode: "VN",
		},
		ShippingFee:      tien(25000),
		DiscountAmount:   tien(10000),
		TaxAmount:        tien(0),
		Lines:            []*orderdom.Line{line},
		IdempotencyKey:   "req_" + ids.MustNew(ids.PrefixRequest).String()[4:],
		SourceCheckoutID: ids.MustNew(ids.PrefixCheckout),
		Now:              time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("dựng đơn: %v", err)
	}

	if err := kho.Save(ctx, goc); err != nil {
		t.Fatalf("ghi đơn: %v", err)
	}

	doc, err := kho.FindByID(ctx, goc.ID())
	if err != nil {
		t.Fatalf("đọc lại đơn: %v", err)
	}

	// So từng phương thức đọc KHÔNG THAM SỐ trả về giá trị so sánh được.
	// Dùng phản chiếu để danh sách tự dài ra khi domain thêm trường —
	// đó là toàn bộ điểm của bài test này.
	tg := reflect.TypeOf(goc)
	var daSo int
	for i := 0; i < tg.NumMethod(); i++ {
		m := tg.Method(i)
		if m.Type.NumIn() != 1 || m.Type.NumOut() != 1 {
			continue
		}
		ten := m.Name
		// Bỏ qua getter trả lát cắt con trỏ: so địa chỉ là vô nghĩa, và
		// nội dung dòng hàng được kiểm riêng bên dưới.
		if m.Type.Out(0).Kind() == reflect.Slice {
			continue
		}

		a := m.Func.Call([]reflect.Value{reflect.ValueOf(goc)})[0].Interface()
		b := m.Func.Call([]reflect.Value{reflect.ValueOf(doc)})[0].Interface()

		// Dấu thời gian đọc từ PostgreSQL mang múi giờ của phiên, cùng
		// khoảnh khắc nhưng khác cách biểu diễn. So theo khoảnh khắc.
		if ta, ok := a.(time.Time); ok {
			tb := b.(time.Time)
			daSo++
			if !ta.Equal(tb) {
				t.Errorf("%s(): ghi %v, đọc lại %v", ten, ta, tb)
			}
			continue
		}

		daSo++
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s(): ghi %v, đọc lại %v — trường này biến mất khi đọc",
				ten, a, b)
		}
	}
	if daSo < 15 {
		t.Errorf("chỉ so được %d trường — phản chiếu không tìm thấy getter, "+
			"bài test mất tác dụng", daSo)
	}

	if len(doc.Lines()) != len(goc.Lines()) {
		t.Fatalf("đọc lại có %d dòng hàng, ghi %d",
			len(doc.Lines()), len(goc.Lines()))
	}

	// ---------------------------------------------------------------
	//
	// GIỚI HẠN của phép so bằng phản chiếu ở trên: nó chỉ bắt được trường
	// nào có giá trị KHÁC giá trị rỗng. `Version` của đơn mới bằng 0, nên
	// bỏ quên nó vẫn cho 0 = 0 và bài test vẫn xanh — đã kiểm bằng cách
	// phá, và nó THẬT SỰ không bắt được.
	//
	// Đó lại đúng là trường từng gây hỏng. Nên phải đẩy nó khác 0 rồi mới
	// so, bằng chính đường mà production đi.
	if err := doc.MarkPaid(time.Now().UTC()); err != nil {
		t.Fatalf("đánh dấu đã thanh toán: %v", err)
	}
	if err := kho.Update(ctx, doc); err != nil {
		t.Fatalf("cập nhật đơn: %v", err)
	}

	lai, err := kho.FindByID(ctx, goc.ID())
	if err != nil {
		t.Fatalf("đọc lại lần hai: %v", err)
	}
	if lai.Version() != 1 {
		t.Errorf("sau một lần cập nhật, Version() = %d, cần 1 — "+
			"khóa lạc quan mất tác dụng và mọi lần chuyển trạng thái "+
			"TIẾP THEO sẽ thất bại", lai.Version())
	}

	// Và lần cập nhật kế tiếp phải chạy được: đó chính là thứ hỏng lần
	// trước — lần một thành công, mọi lần sau thất bại im lặng.
	if err := lai.CancelWithReason("kiểm chuyển trạng thái lần hai", time.Now().UTC()); err != nil {
		t.Fatalf("hủy đơn: %v", err)
	}
	if err := kho.Update(ctx, lai); err != nil {
		t.Errorf("lần chuyển trạng thái THỨ HAI thất bại: %v", err)
	}
}
