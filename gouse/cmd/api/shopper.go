package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/fashion-commerce/platform/internal/modules/customer"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
)

// customerResolver đổi định danh TÀI KHOẢN lấy định danh KHÁCH HÀNG.
//
// # Vì sao cầu nối này phải nằm ở đây
//
// httpserver.CustomerResolver do platform khai báo, module customer cài
// đặt — nhưng platform KHÔNG được import module nghiệp vụ nào (quy tắc R3
// của archcheck), nên hai đầu không tự gặp nhau được.
//
// cmd/api là điểm nối duy nhất biết cả hai. Cùng mẫu với TokenVerifier.
type customerResolver struct{ api customer.API }

var _ httpserver.CustomerResolver = (*customerResolver)(nil)

// CustomerIDForUser trả hồ sơ khách hàng của một tài khoản.
//
// KHÔNG có hồ sơ là trạng thái HỢP LỆ, không phải lỗi: nhân viên vận hành
// có tài khoản nhưng chưa bao giờ mua gì. Trả chuỗi rỗng thì họ được coi
// như khách vãng lai và vẫn mua hàng được.
//
// Biến lỗi này thành lỗi thật nghĩa là mọi nhân viên đăng nhập đều không
// thêm được món nào vào giỏ.
func (r *customerResolver) CustomerIDForUser(
	ctx context.Context, userID string,
) (string, error) {
	view, err := r.api.GetCustomerByUserID(ctx, userID)
	if errors.Is(err, customer.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return view.ID, nil
}

// registerShoppingRoutes nối giỏ hàng và phiên thanh toán.
//
// # Vì sao hai module này dùng chung một chuỗi middleware
//
// Cả hai trả lời cùng một câu hỏi trước mọi việc khác: "ai đang mua?".
// Chuỗi ở đây trả lời câu đó một lần:
//
//	OptionalAuth   → nhận diện nếu có token, KHÔNG chặn nếu không
//	ResolveShopper → đổi user_id lấy customer_id, cấp phiên cho khách vãng lai
//
// OptionalAuth chứ không phải Auth là điểm quyết định: khách VÃNG LAI phải
// mua được (mvp.md mục 4). Dùng Auth ở đây là chặn mọi khách chưa đăng ký
// khỏi việc thêm hàng vào giỏ.
func registerShoppingRoutes(mux *http.ServeMux, log *slog.Logger, m modules) {
	if m.cart == nil && m.checkout == nil && m.order == nil && m.customer == nil {
		return
	}

	// resolver nil là chấp nhận được: khi đó khách đăng nhập bị coi như
	// vãng lai và giỏ gắn với cookie phiên. Mất tính liên tục giữa các
	// thiết bị, nhưng không ai bị chặn khỏi việc mua hàng.
	var resolver httpserver.CustomerResolver
	if m.customer != nil {
		resolver = &customerResolver{api: m.customer}
	}

	shopper := func(inner *http.ServeMux, extra ...httpserver.Middleware) http.Handler {
		chain := []httpserver.Middleware{httpserver.ResolveShopper(resolver)}
		if m.identity != nil {
			// OptionalAuth phải đứng TRƯỚC ResolveShopper: ResolveShopper
			// đọc AuthContext do nó đặt vào. Đảo thứ tự thì mọi khách đăng
			// nhập lặng lẽ bị coi là vãng lai — hỏng mà không báo lỗi.
			chain = append([]httpserver.Middleware{
				httpserver.OptionalAuth(m.identity),
			}, chain...)
		}
		return httpserver.Chain(inner, append(chain, extra...)...)
	}

	if m.cart != nil {
		cartMux := http.NewServeMux()
		m.cart.RegisterRoutes(cartMux, log)

		// GET không cần Idempotency-Key và middleware cũng chỉ áp cho
		// POST/PATCH, nên một chuỗi phục vụ được cả bốn đường.
		h := shopper(cartMux, httpserver.RequireIdempotencyKey())
		mux.Handle("GET /api/v1/cart", h)
		mux.Handle("POST /api/v1/cart/items", h)
		mux.Handle("PATCH /api/v1/cart/items/{cart_item_id}", h)
		mux.Handle("DELETE /api/v1/cart/items/{cart_item_id}", h)
	}

	if m.checkout != nil {
		checkoutMux := http.NewServeMux()
		m.checkout.RegisterRoutes(checkoutMux, log)

		// Idempotency-Key BẮT BUỘC cho mọi đường ghi ở đây. Đường quan
		// trọng nhất là `complete`: bấm hai lần không được tạo hai đơn.
		h := shopper(checkoutMux, httpserver.RequireIdempotencyKey())
		mux.Handle("POST /api/v1/checkout", h)
		mux.Handle("GET /api/v1/checkout/{checkout_id}", h)
		mux.Handle("PATCH /api/v1/checkout/{checkout_id}/shipping-address", h)
		mux.Handle("PATCH /api/v1/checkout/{checkout_id}/shipping-method", h)
		mux.Handle("POST /api/v1/checkout/{checkout_id}/coupon", h)
		mux.Handle("POST /api/v1/checkout/{checkout_id}/complete", h)

		// `POST /api/v1/orders` do module CHECKOUT phục vụ, không phải
		// module order: nó phải đọc phiên thanh toán, mà order không được
		// gọi checkout (ADR-0007). Xem chú thích ở checkout Register.
		mux.Handle("POST /api/v1/orders", h)
	}

	if m.customer != nil {
		accountMux := http.NewServeMux()
		m.customer.RegisterRoutes(accountMux, log)

		h := shopper(accountMux, httpserver.RequireIdempotencyKey())
		mux.Handle("GET /api/v1/me", h)
		mux.Handle("PATCH /api/v1/me", h)
		mux.Handle("GET /api/v1/me/addresses", h)
		mux.Handle("POST /api/v1/me/addresses", h)
		mux.Handle("GET /api/v1/me/wishlist", h)
		mux.Handle("POST /api/v1/me/wishlist", h)
	}

	if m.order != nil {
		orderMux := http.NewServeMux()
		m.order.RegisterCustomerRoutes(orderMux, log)

		// GET không cần khóa; middleware chỉ áp cho POST/PATCH nên một
		// chuỗi phục vụ được cả ba đường.
		h := shopper(orderMux, httpserver.RequireIdempotencyKey())
		mux.Handle("GET /api/v1/orders", h)
		mux.Handle("GET /api/v1/orders/{order_id}", h)
		mux.Handle("POST /api/v1/orders/{order_id}/cancel", h)
	}
}
