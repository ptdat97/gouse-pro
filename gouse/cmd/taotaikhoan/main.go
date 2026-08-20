// Tạo bộ tài khoản dùng để THỬ GIAO DIỆN ở môi trường phát triển.
//
// Đi qua API của module identity chứ không INSERT thẳng vào bảng: mật khẩu
// phải được băm đúng cách, và vai trò phải qua kiểm tra của domain. Ghi
// thẳng SQL là cách tạo ra tài khoản đăng nhập không được rồi mất thời
// gian tìm xem sai ở đâu.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/fashion-commerce/platform/internal/modules/identity"
	"github.com/fashion-commerce/platform/internal/platform/database"
	"github.com/fashion-commerce/platform/internal/platform/token"
)

type taiKhoan struct {
	email   string
	matKhau string
	vaiTro  []string
	moTa    string
}

func main() {
	// CHẶN Ở PRODUCTION, và chặn bằng cách từ chối chạy chứ không phải
	// bằng cách tin người gọi cẩn thận.
	//
	// Công cụ này tạo tài khoản ADMIN với mật khẩu nằm trong mã nguồn.
	// Chạy nhầm nó ở production là mở cửa hậu cho bất kỳ ai đọc được repo
	// — và "nhầm" ở đây chỉ cần một biến môi trường trỏ sai.
	if env := os.Getenv("APP_ENV"); env != "" && env != "development" {
		fmt.Fprintf(os.Stderr,
			"từ chối chạy với APP_ENV=%s — công cụ này CHỈ dùng khi phát triển\n",
			env)
		os.Exit(1)
	}

	ctx := context.Background()
	db, err := database.Open(ctx, database.Config{DSN: os.Getenv("DATABASE_URL")})
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// Issuer bắt buộc dù script này không dùng access token: module từ
	// chối khởi tạo mà không có nó, và đó là ràng buộc đúng — đăng nhập
	// thành công nhưng không cấp được token là hỏng theo kiểu khó thấy.
	//
	// Dùng CÙNG khóa với cmd/api để `Login` bên dưới kiểm chứng được đúng
	// thứ mà giao diện sẽ gặp.
	secret := os.Getenv("AUTH_JWT_SECRET")
	if secret == "" {
		secret = "development-only-jwt-secret-do-not-use-in-production"
	}
	issuer, err := token.NewIssuer(token.Config{Secret: secret})
	if err != nil {
		panic(err)
	}

	m, err := identity.New(identity.Config{
		Storage: "postgres", DB: db, Issuer: issuer,
	})
	if err != nil {
		panic(err)
	}

	const matKhau = "Gouse@Test2026"

	danhSach := []taiKhoan{
		{"khach@gouse.test", matKhau, nil, "khách hàng thường"},
		{"admin@gouse.test", matKhau, []string{"ADMIN"}, "quản trị"},
		{"vanhanh@gouse.test", matKhau,
			[]string{"OPS_MERCHANDISING", "OPS_FINANCE"}, "vận hành"},
	}

	for _, tk := range danhSach {
		v, err := m.Register(ctx, identity.RegisterRequest{
			Email:    tk.email,
			Password: tk.matKhau,
		})
		switch {
		case err == nil:
			fmt.Printf("đã tạo   %-22s %s\n", tk.email, tk.moTa)
		case errors.Is(err, identity.ErrDuplicateEmail):
			// Chạy lại lần hai không được làm hỏng gì: script này sẽ được
			// gọi lại mỗi khi nạp lại dữ liệu mẫu.
			//
			// Đăng nhập để lấy lại định danh — module không mở đường tra
			// theo email, và đăng nhập cũng đồng thời XÁC MINH mật khẩu
			// đúng như mong đợi. Tài khoản có thật mà mật khẩu khác thì
			// script này phải nói ra, không được im lặng.
			res, loginErr := m.Login(ctx, identity.LoginRequest{
				Email: tk.email, Password: tk.matKhau,
			})
			if loginErr != nil {
				fmt.Printf("CẢNH BÁO %-22s đã tồn tại nhưng mật khẩu KHÁC: %v\n",
					tk.email, loginErr)
				continue
			}
			v = identity.UserView{ID: res.User.ID}
			fmt.Printf("đã có    %-22s %s (mật khẩu đúng)\n", tk.email, tk.moTa)
		default:
			fmt.Printf("LỖI tạo %s: %v\n", tk.email, err)
			continue
		}

		for _, r := range tk.vaiTro {
			if err := m.GrantRole(ctx, v.ID, r, ""); err != nil {
				fmt.Printf("  LỖI gán %s: %v\n", r, err)
				continue
			}
			fmt.Printf("  + vai trò %s\n", r)
		}
	}
}
