package main

import (
	"context"
	"errors"
	"testing"
)

// TestVetOutboxRutHetKhiKhoeManh: lô đầy thì làm tiếp ngay, không chờ nhịp.
//
// Không có vòng vét thì tốc độ rút bị khóa ở `lô / nhịp` — 100/5s = 20
// event/giây — bất kể hệ thống rảnh đến đâu. Đo thật cho ra đúng con số
// đó, phẳng tuyệt đối, trong khi một lô chỉ tốn 182ms.
func TestVetOutboxRutHetKhiKhoeManh(t *testing.T) {
	con := 450
	goi := 0

	tong, err := vetOutbox(context.Background(), 100, 50,
		func(_ context.Context, n int) (int, error) {
			goi++
			if con < n {
				n = con
			}
			con -= n
			return n, nil
		})
	if err != nil {
		t.Fatalf("vét: %v", err)
	}

	if tong != 450 {
		t.Errorf("rút được %d, cần 450", tong)
	}
	if goi != 5 {
		t.Errorf("gọi %d lượt, cần 5 (4 lô đầy + 1 lô 50) — "+
			"số lượt khác nghĩa là vòng vét dừng sai chỗ", goi)
	}
}

// TestVetOutboxDungNgayKhiCoLoi là hàng rào QUAN TRỌNG NHẤT của tệp này.
//
// `maxAttempts` đếm theo LƯỢT THỬ, không theo thời gian. Nếu vòng vét chạy
// tiếp khi có event hỏng, một sự cố thoáng qua một giây sẽ đốt hết năm
// lượt thử trong vài trăm mili giây và đẩy CẢ HÀNG ĐỢI vào dead letter —
// trong khi chỉ cần chờ vài giây là bên nhận sống lại.
//
// Dead letter nghĩa là có sự thật nghiệp vụ không bao giờ tới bên nhận:
// đơn đã đặt mà tồn kho không chuyển sang Committed. Biến một sự cố tạm
// thời thành mất mát vĩnh viễn là cái giá quá đắt cho việc rút nhanh hơn
// một nhịp.
func TestVetOutboxDungNgayKhiCoLoi(t *testing.T) {
	goi := 0

	// Mỗi lô lấy đủ 100 nhưng CHỈ 99 phát được: một event hỏng.
	tong, err := vetOutbox(context.Background(), 100, 50,
		func(_ context.Context, _ int) (int, error) {
			goi++
			return 99, nil
		})
	if err != nil {
		t.Fatalf("vét: %v", err)
	}

	if goi != 1 {
		t.Errorf("gọi %d lượt khi lô có event hỏng, cần đúng 1 — "+
			"vét tiếp sẽ đốt hết %d lượt thử trong một nhịp và đẩy cả "+
			"hàng đợi vào dead letter", goi, 5)
	}
	if tong != 99 {
		t.Errorf("rút được %d, cần 99", tong)
	}
}

// TestVetOutboxKhongVuotTran: một lượt job phải kết thúc trong thời gian
// hữu hạn, nếu không thì tín hiệu dừng không tới được và triển khai lại
// phải chờ.
func TestVetOutboxKhongVuotTran(t *testing.T) {
	goi := 0
	tong, err := vetOutbox(context.Background(), 100, 7,
		func(_ context.Context, n int) (int, error) {
			goi++
			return n, nil // luôn đầy: hàng đợi vô tận
		})
	if err != nil {
		t.Fatalf("vét: %v", err)
	}
	if goi != 7 {
		t.Errorf("gọi %d lượt, trần là 7", goi)
	}
	if tong != 700 {
		t.Errorf("rút được %d, cần 700", tong)
	}
}

// TestVetOutboxDungKhiHuyNguCanh: hàng đợi vô tận không được giữ tiến
// trình lại lúc đang dừng.
func TestVetOutboxDungKhiHuyNguCanh(t *testing.T) {
	ctx, huy := context.WithCancel(context.Background())
	goi := 0

	_, err := vetOutbox(ctx, 100, 50, func(_ context.Context, n int) (int, error) {
		goi++
		if goi == 3 {
			huy()
		}
		return n, nil
	})
	if err != nil {
		t.Fatalf("vét: %v", err)
	}
	if goi != 3 {
		t.Errorf("gọi %d lượt sau khi hủy ngữ cảnh, cần dừng ở 3", goi)
	}
}

// TestVetOutboxTraLoiNgay: lỗi hạ tầng phải nổi lên, không bị nuốt.
func TestVetOutboxTraLoiNgay(t *testing.T) {
	hong := errors.New("database mất kết nối")
	goi := 0

	tong, err := vetOutbox(context.Background(), 100, 50,
		func(_ context.Context, n int) (int, error) {
			goi++
			if goi == 2 {
				return 0, hong
			}
			return n, nil
		})
	if !errors.Is(err, hong) {
		t.Errorf("lỗi trả về %v, cần %v", err, hong)
	}
	if tong != 100 {
		t.Errorf("rút được %d trước khi lỗi, cần 100 — "+
			"số đã rút không được mất khi báo lỗi", tong)
	}
}
