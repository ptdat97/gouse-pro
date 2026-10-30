package main

// ratchet là số định danh còn âm tiết Việt được PHÉP tồn tại.
//
// Đây là BÁNH CÓC MỘT CHIỀU, giống hai ngưỡng của `apicheck`: nó chỉ được
// giảm. Lệnh đỏ ở CẢ HAI phía — đo cao hơn ngưỡng là có tên mới lọt vào,
// đo thấp hơn là ngưỡng đã tụt lại sau sự thật và không còn gác gì.
//
// Lịch sử, để thấy con số này có nghĩa:
//
//	30/10/2026  453  đo lần đầu bằng go/ast + hình dạng âm tiết
//	30/10/2026  281  xong internal/{modules,platform,kernel}
//	30/10/2026  240  xong cmd/ — kể cả dự án GIẢ trong chuỗi thô của test
//	30/10/2026    0  xong internal/app và internal/e2e
//
// Ngưỡng 0 nghĩa là KHÔNG định danh nào còn âm tiết Việt. Nó KHÔNG
// nghĩa là bài toán đã hết: một âm tiết Việt TRÙNG từ tiếng Anh vẫn
// vô hình vì nó nằm trong `allowed`. Đó là giới hạn ĐÃ BIẾT và đã ghi,
// không phải một con số đẹp.
const ratchet = 0

// allowed là các âm tiết CÓ hình dạng tiếng Việt nhưng KHÔNG phải tiếng
// Việt trong repo này.
//
// # Vì sao danh sách này được phép tồn tại khi `loc.py` bị bỏ
//
// `loc.py` liệt kê từ Việt CẦN TÌM — tập mở, mỗi lượt lại thiếu. Danh
// sách này liệt kê từ Anh/kỹ thuật ĐƯỢC BỎ QUA, và nó bị chặn trên bởi
// một thứ có thật: số âm tiết tiếng Anh ba chữ cái. Thiếu một dòng ở đây
// gây CẢNH BÁO GIẢ — ồn nhưng thấy ngay; thiếu một dòng ở `loc.py` gây
// im lặng.
//
// Mỗi dòng phải có lý do đọc được. "Trông như tiếng Anh" không phải lý
// do: `ten`, `ma`, `lo`, `ra`, `cu`, `se`, `vet`, `han`, `ban`, `tin`
// đều trông như tiếng Anh và đều từng là tiếng Việt trong repo này.
var allowed = map[string]bool{
	// --------------------------------------------- viết tắt của Go và SQL
	"ctx": true, "mu": true, "db": true, "tx": true, "fn": true,
	"ok": true, "id": true, "io": true, "os": true, "re": true,
	"rec": true, "req": true, "res": true, "ret": true, "err": true,
	"ptr": true, "buf": true, "cfg": true, "dec": true, "enc": true,
	"src": true, "dst": true, "tmp": true, "idx": true, "len": true,
	"cap": true, "num": true, "str": true, "val": true, "var": true,
	"arg": true, "op": true, "fd": true, "wg": true, "bp": true,

	// --------------------------------------------- miền nghiệp vụ, viết tắt
	"sku": true, "dto": true, "aov": true, "gmv": true, "cod": true,
	"vnd": true, "vat": true, "ip": true, "ui": true, "api": true,
	"url": true, "dsn": true, "ttl": true, "jwt": true, "ulid": true,
	"qty": true, "sec": true, "min": true, "max": true, "sum": true,
	"avg": true, "pct": true, "bps": true, "loc": true, "env": true,
	"vip": true, "cam": true, "svc": true, "pg": true, "mac": true,

	// --------------------------------------------- từ tiếng Anh đúng hình
	//
	// Mọi dòng dưới đây là một từ tiếng Anh TRÙNG hình dạng âm tiết Việt.
	// Chúng ở đây vì repo dùng chúng, không vì chúng vô hại.
	"a": true, "an": true, "at": true, "be": true, "but": true,
	"buy": true, "by": true, "can": true, "cat": true, "day": true,
	"dim": true, "do": true, "doc": true, "due": true, "gen": true,
	"get": true, "go": true, "got": true, "has": true, "hoc": true,
	"in": true, "it": true, "main": true, "me": true, "mem": true,
	"men": true, "my": true, "net": true, "no": true, "non": true,
	"noop": true, "not": true, "on": true, "pay": true, "ping": true,
	"put": true, "room": true, "root": true, "run": true, "say": true,
	"set": true, "so": true, "ten": true, "than": true, "that": true,
	"the": true, "then": true, "to": true, "too": true, "trip": true,
	"up": true, "via": true, "yet": true, "we": true, "if": true,
	"is": true, "as": true, "or": true, "of": true, "he": true,
	"us": true, "am": true,

	// ----------------------------- đã ĐỌC từng chỗ dùng, 30/10/2026
	//
	// Mỗi âm tiết dưới đây đã được xem tại chỗ khai báo và xác nhận là
	// tiếng Anh hoặc viết tắt kỹ thuật. Ghi kèm chỗ dùng tiêu biểu để
	// lượt sau không phải đọc lại — và để thấy ngay nếu nó đổi nghĩa.
	"chain": true, // httpserver.Chain(h, mws...)
	"top":   true, // ProductTypeTop = "TOP"
	"eu":    true, // SizeSystemEU = "EU"
	"long":  true, // long := make([]byte, 100)
	"ac":    true, // ac, ok := httpserver.AuthContextFrom(...)
	"dup":   true, // if _, dup := seen[id]; dup
	"gap":   true, // approveJSON(v, position, gap)
	"hi":    true, // hi, low := bits.Mul64(...)
	"hit":   true, // type hit struct — của chính lệnh này
	"lit":   true, // lit, ok := vs.Values[0].(*ast.CompositeLit)
	"oc":    true, // if oc := m.OpsConfig(); oc != nil
	"pi":    true, // pi, pj := list[i], list[j]
	"rem":   true, // if rem := product % scale
	"rip":   true, // rip := r.Header.Get("X-Real-IP")
	"se":    true, // se := &fakeSeller{...}
	"si":    true, // for _, si := range in.SKUs
	"ty":    true, // ty := y.(time.Time)
	"typ":   true, // id, typ, label, currency string
	"ua":    true, // var ipHash, ua string
	"map":   true, // totalMap — hậu tố `Map` của Go, không phải "máp"
}
