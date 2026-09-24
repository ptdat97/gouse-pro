package main

// Hai sổ MIỄN TRỪ của hợp đồng event.
//
// Cùng khuôn với `chuaCai`, `ngoaiHopDong`, `headerNgoaiDacTa` và
// `capEnumDaKiem` của `apicheck`: miễn trừ phải là một quyết định CÓ
// NGƯỜI KÝ, kèm lý do đọc được, không phải một chỗ bị bỏ quên.
//
// Không có sổ cho R3 (nghe mà không ai phát) — một bên nhận không bao giờ
// chạy không có lý do hợp lệ nào.

// khongAiPhat: loại event khai mà CỐ Ý chưa phát.
//
// Trống là đúng. Một loại event chưa dùng thì XÓA hằng số đi, thêm lại
// khi thật sự phát — ngược lại nó là một lời mời viết bên nhận chết.
//
// Ngày 25/09/2026 bốn loại bị xóa vì lý do ấy: `order.placed`,
// `inventory.reserved`, `inventory.committed`,
// `inventory.reservation_released`. Xem P3-76.
var khongAiPhat = map[string]string{}

// khongAiNghe: loại event CÓ phát mà cố ý chưa ai nghe.
//
// Đây là ngoại lệ hợp lệ, khác R1: một sự thật được ghi lại để sau này
// dùng (phân tích, kiểm toán) vẫn có giá trị dù hôm nay chưa ai hành động
// theo. Nhưng nó tốn một hàng outbox mỗi lần, nên phải là chủ ý.
var khongAiNghe = map[string]string{}
