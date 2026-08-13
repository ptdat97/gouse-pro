// Package ids sinh và kiểm tra định danh ULID có tiền tố.
//
// Vì sao ULID thay vì số tự tăng:
//   - Số tự tăng lộ quy mô kinh doanh (đơn #1547 = nền tảng mới có 1547 đơn)
//   - Khó tách service: hai service sinh id trùng
//   - Dễ dò: đoán được id của bản ghi khác
//
// Vì sao ULID thay vì UUID ngẫu nhiên:
//   - Sắp xếp được theo thời gian tạo → chèn tuần tự, ít phân mảnh chỉ mục
//   - Vẫn tương thích định dạng 128-bit
//
// Tiền tố giúp gỡ lỗi nhanh: nhìn off_... biết ngay là offer, không phải order.
//
// Xem docs/05-data/data-model.md mục 11.
package ids

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Bảng chữ Crockford base32: bỏ I, L, O, U để tránh nhầm lẫn khi đọc/gõ.
const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// encodedLen là độ dài phần ULID sau khi mã hóa (48 bit thời gian + 80 bit ngẫu nhiên).
const encodedLen = 26

var (
	ErrInvalidFormat = errors.New("ids: định dạng không hợp lệ")
	ErrWrongPrefix   = errors.New("ids: sai tiền tố")
	ErrEmptyPrefix   = errors.New("ids: tiền tố rỗng")
)

// Prefix là tiền tố loại thực thể.
type Prefix string

// Tiền tố của các thực thể. Danh sách này khớp với đặc tả OpenAPI.
const (
	PrefixOrder             Prefix = "ord"
	PrefixOrderLine         Prefix = "oln"
	PrefixFulfillmentOrder  Prefix = "ful"
	PrefixAdjustment        Prefix = "adj"
	PrefixCart              Prefix = "crt"
	PrefixCartItem          Prefix = "cit"
	PrefixCheckout          Prefix = "chk"
	PrefixProduct           Prefix = "prd"
	PrefixVariant           Prefix = "var"
	PrefixSKU               Prefix = "sku"
	PrefixOffer             Prefix = "off"
	PrefixPrice             Prefix = "prc"
	PrefixPriceConstraint   Prefix = "pcs"
	PrefixBrand             Prefix = "brd"
	PrefixAuthorization     Prefix = "aut"
	PrefixCollection        Prefix = "col"
	PrefixCategory          Prefix = "cat"
	PrefixSizeChart         Prefix = "szc"
	PrefixSeller            Prefix = "sel"
	PrefixCustomer          Prefix = "cus"
	PrefixCreator           Prefix = "cre"
	PrefixContent           Prefix = "cnt"
	PrefixOutfit            Prefix = "otf"
	PrefixAffiliateLink     Prefix = "afl"
	PrefixCampaign          Prefix = "cmp"
	PrefixUser              Prefix = "usr"
	PrefixInventoryItem     Prefix = "inv"
	PrefixInventoryMovement Prefix = "imv"
	PrefixReservation       Prefix = "rsv"
	PrefixStockLocation     Prefix = "loc"
	PrefixLedgerEntry       Prefix = "led"
	PrefixSettlement        Prefix = "stl"
	PrefixPayout            Prefix = "pay"
	PrefixSupplier          Prefix = "sup"
	PrefixProductionOrder   Prefix = "pro"
	PrefixProductionBatch   Prefix = "pbt"
	PrefixRequest           Prefix = "req"
	PrefixEvent             Prefix = "evt"
)

// ID là định danh có tiền tố, ví dụ "ord_01J9XABC123DEF456GHJKMNPQR".
type ID string

func (id ID) String() string { return string(id) }
func (id ID) IsZero() bool   { return id == "" }

// Prefix trả về phần tiền tố.
func (id ID) Prefix() Prefix {
	i := strings.IndexByte(string(id), '_')
	if i < 0 {
		return ""
	}
	return Prefix(id[:i])
}

// generator giữ trạng thái để đảm bảo tính đơn điệu trong cùng mili-giây.
type generator struct {
	mu       sync.Mutex
	lastMS   uint64
	lastRand [10]byte
}

var defaultGen = &generator{}

// New sinh ID mới với tiền tố cho trước.
//
// Trả lỗi nếu tiền tố rỗng. Lỗi từ nguồn ngẫu nhiên của hệ điều hành gây
// panic — đó là lỗi không thể phục hồi, và tiếp tục với entropy yếu còn
// nguy hiểm hơn dừng lại.
func New(p Prefix) (ID, error) {
	if p == "" {
		return "", ErrEmptyPrefix
	}
	return ID(string(p) + "_" + defaultGen.next()), nil
}

// MustNew như New nhưng panic khi lỗi. Dùng khi tiền tố là hằng số.
func MustNew(p Prefix) ID {
	id, err := New(p)
	if err != nil {
		panic(err)
	}
	return id
}

// next sinh phần ULID 26 ký tự.
func (g *generator) next() string {
	ms := uint64(time.Now().UnixMilli())

	g.mu.Lock()
	var entropy [10]byte
	if ms == g.lastMS {
		// Cùng mili-giây: tăng phần ngẫu nhiên để giữ tính đơn điệu,
		// đảm bảo id sinh sau luôn lớn hơn id sinh trước.
		entropy = g.lastRand
		for i := len(entropy) - 1; i >= 0; i-- {
			entropy[i]++
			if entropy[i] != 0 {
				break
			}
		}
	} else {
		if _, err := rand.Read(entropy[:]); err != nil {
			g.mu.Unlock()
			panic(fmt.Sprintf("ids: không đọc được nguồn ngẫu nhiên: %v", err))
		}
		g.lastMS = ms
	}
	g.lastRand = entropy
	g.mu.Unlock()

	return encode(ms, entropy)
}

// encode mã hóa 48 bit thời gian + 80 bit ngẫu nhiên thành 26 ký tự base32.
func encode(ms uint64, entropy [10]byte) string {
	var b [16]byte
	// 48 bit đầu: thời gian mili-giây
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	copy(b[6:], entropy[:])

	// Mã hóa 128 bit thành 26 ký tự, mỗi ký tự 5 bit (26*5 = 130 bit,
	// 2 bit đầu là đệm).
	out := make([]byte, encodedLen)
	out[0] = alphabet[(b[0]&224)>>5]
	out[1] = alphabet[b[0]&31]
	out[2] = alphabet[(b[1]&248)>>3]
	out[3] = alphabet[((b[1]&7)<<2)|((b[2]&192)>>6)]
	out[4] = alphabet[(b[2]&62)>>1]
	out[5] = alphabet[((b[2]&1)<<4)|((b[3]&240)>>4)]
	out[6] = alphabet[((b[3]&15)<<1)|((b[4]&128)>>7)]
	out[7] = alphabet[(b[4]&124)>>2]
	out[8] = alphabet[((b[4]&3)<<3)|((b[5]&224)>>5)]
	out[9] = alphabet[b[5]&31]
	out[10] = alphabet[(b[6]&248)>>3]
	out[11] = alphabet[((b[6]&7)<<2)|((b[7]&192)>>6)]
	out[12] = alphabet[(b[7]&62)>>1]
	out[13] = alphabet[((b[7]&1)<<4)|((b[8]&240)>>4)]
	out[14] = alphabet[((b[8]&15)<<1)|((b[9]&128)>>7)]
	out[15] = alphabet[(b[9]&124)>>2]
	out[16] = alphabet[((b[9]&3)<<3)|((b[10]&224)>>5)]
	out[17] = alphabet[b[10]&31]
	out[18] = alphabet[(b[11]&248)>>3]
	out[19] = alphabet[((b[11]&7)<<2)|((b[12]&192)>>6)]
	out[20] = alphabet[(b[12]&62)>>1]
	out[21] = alphabet[((b[12]&1)<<4)|((b[13]&240)>>4)]
	out[22] = alphabet[((b[13]&15)<<1)|((b[14]&128)>>7)]
	out[23] = alphabet[(b[14]&124)>>2]
	out[24] = alphabet[((b[14]&3)<<3)|((b[15]&224)>>5)]
	out[25] = alphabet[b[15]&31]
	return string(out)
}

// Parse kiểm tra chuỗi có phải ID hợp lệ với tiền tố mong đợi không.
//
// Dùng ở tầng interfaces để kiểm tra tham số đầu vào — trả lỗi rõ ràng
// thay vì để truy vấn database thất bại.
func Parse(s string, want Prefix) (ID, error) {
	i := strings.IndexByte(s, '_')
	if i < 0 {
		return "", fmt.Errorf("%w: thiếu dấu gạch dưới: %q", ErrInvalidFormat, s)
	}
	gotPrefix := Prefix(s[:i])
	if gotPrefix != want {
		return "", fmt.Errorf("%w: mong %q, nhận %q", ErrWrongPrefix, want, gotPrefix)
	}
	body := s[i+1:]
	if len(body) != encodedLen {
		return "", fmt.Errorf("%w: phần ULID phải %d ký tự, nhận %d",
			ErrInvalidFormat, encodedLen, len(body))
	}
	for _, c := range body {
		if !strings.ContainsRune(alphabet, c) {
			return "", fmt.Errorf("%w: ký tự %q không thuộc bảng chữ Crockford base32",
				ErrInvalidFormat, c)
		}
	}
	return ID(s), nil
}

// IsValid kiểm tra nhanh không quan tâm tiền tố cụ thể.
func IsValid(s string) bool {
	i := strings.IndexByte(s, '_')
	if i <= 0 {
		return false
	}
	_, err := Parse(s, Prefix(s[:i]))
	return err == nil
}
