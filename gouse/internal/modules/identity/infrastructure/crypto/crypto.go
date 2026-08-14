// Package crypto cài đặt băm mật khẩu và sinh token.
//
// TÁCH KHỎI DOMAIN có chủ ý: tầng domain định nghĩa interface
// `PasswordHasher` và `TokenGenerator`, package này cài đặt. Đổi thuật
// toán băm là viết một cài đặt mới, không sửa module.
package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/fashion-commerce/platform/internal/modules/identity/domain"
	"github.com/fashion-commerce/platform/internal/platform/privacy"
)

// BcryptHasher băm mật khẩu bằng bcrypt.
//
// VÌ SAO BCRYPT: nó CHẬM CÓ CHỦ Ý. Một hàm băm nhanh như SHA-256 cho phép
// thử hàng tỷ mật khẩu mỗi giây trên GPU; bcrypt với chi phí mặc định chỉ
// cho phép vài nghìn. Muối nằm sẵn trong chuỗi kết quả, nên không có
// chuyện quên thêm muối.
//
// Argon2id chống GPU tốt hơn, nhưng phải tự quản lý muối và tham số rồi
// tự mã hóa chuỗi lưu trữ — nhiều chỗ sai hơn. Xem
// docs/11-oss/dependency-registry.md.
type BcryptHasher struct {
	cost int
}

// NewBcryptHasher tạo bộ băm.
//
// cost bằng 0 thì dùng mặc định của thư viện (hiện là 10). Tăng cost làm
// việc dò mật khẩu chậm hơn theo cấp số nhân, nhưng cũng làm mỗi lần đăng
// nhập chậm hơn — cần đo trên phần cứng thật trước khi tăng.
func NewBcryptHasher(cost int) *BcryptHasher {
	if cost <= 0 {
		cost = bcrypt.DefaultCost
	}
	return &BcryptHasher{cost: cost}
}

var _ domain.PasswordHasher = (*BcryptHasher)(nil)

func (h *BcryptHasher) Hash(password string) (string, error) {
	// bcrypt CHỈ dùng 72 byte đầu và âm thầm bỏ phần còn lại.
	//
	// Với mật khẩu dài hơn, hai chuỗi khác nhau ở byte thứ 73 trở đi sẽ
	// băm ra cùng kết quả. Báo lỗi thay vì âm thầm cắt: người dùng nghĩ
	// mật khẩu 100 ký tự của họ an toàn hơn thực tế.
	if len(password) > 72 {
		return "", fmt.Errorf("identity: mật khẩu vượt quá 72 byte — " +
			"bcrypt bỏ qua phần dư và làm mật khẩu yếu hơn người dùng tưởng")
	}

	b, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", fmt.Errorf("identity: băm mật khẩu: %w", err)
	}
	return string(b), nil
}

// Verify kiểm tra mật khẩu khớp với hash.
//
// Trả về false chứ không phải lỗi khi không khớp: sai mật khẩu là đường đi
// BÌNH THƯỜNG, không phải sự cố. Trả lỗi sẽ khiến bên gọi ghi cảnh báo cho
// mỗi lần người dùng gõ nhầm.
//
// bcrypt.CompareHashAndPassword so sánh theo THỜI GIAN HẰNG ĐỊNH — không
// tự viết vòng lặp so sánh, vì thời gian so sánh khác nhau để lộ bao nhiêu
// ký tự đầu đã đúng.
func (h *BcryptHasher) Verify(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// RandomTokenGenerator sinh token ngẫu nhiên bằng crypto/rand.
//
// DÙNG crypto/rand, KHÔNG dùng math/rand: math/rand đoán được nếu biết
// hạt giống, và token đoán được nghĩa là đăng nhập được vào tài khoản
// người khác.
type RandomTokenGenerator struct{}

func NewTokenGenerator() *RandomTokenGenerator { return &RandomTokenGenerator{} }

var _ domain.TokenGenerator = (*RandomTokenGenerator)(nil)

// tokenBytes là độ dài token thô.
//
// 32 byte (256 bit) là mức không thể dò được bằng vét cạn trong thực tế.
const tokenBytes = 32

// NewToken sinh token và trả về cả bản nguyên văn lẫn bản băm.
//
// Nguyên văn trả cho client MỘT LẦN duy nhất; bản băm lưu vào database.
// Nhờ vậy rò rỉ database không cho phép kẻ tấn công đăng nhập.
func (g *RandomTokenGenerator) NewToken() (plain, hashed string, err error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("identity: sinh token: %w", err)
	}

	plain = base64.RawURLEncoding.EncodeToString(buf)
	return plain, g.HashToken(plain), nil
}

// HashToken băm token để tra cứu.
//
// DÙNG SHA-256 chứ không bcrypt, và đây là khác biệt có chủ ý so với mật
// khẩu:
//
//	Mật khẩu  do NGƯỜI chọn → entropy thấp → cần hàm CHẬM chống vét cạn
//	Token     do MÁY sinh   → 256 bit      → vét cạn bất khả thi
//
// Dùng bcrypt cho token sẽ làm mỗi lần làm mới phiên tốn hàng chục
// mili-giây mà không tăng an toàn chút nào.
func (g *RandomTokenGenerator) HashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// HashIP băm địa chỉ IP để lưu nhật ký.
//
// Ủy quyền cho platform/privacy: module `customer` cần đúng việc này, và
// hai bản sao chép sẽ trôi xa nhau. Khi đó cùng một địa chỉ băm ra hai giá
// trị khác nhau — đủ để phá mọi truy vấn "nhiều lần thử từ cùng một nguồn".
func HashIP(ip string) string { return privacy.HashIP(ip) }
