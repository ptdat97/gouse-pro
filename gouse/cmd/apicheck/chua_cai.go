package main

// chuaCai là những thao tác ĐÃ khai trong đặc tả mà CHƯA có route, kèm lý
// do vì sao chưa.
//
// # Vì sao phải khai từng dòng thay vì đếm tổng
//
// Backlog từng ghi "24 thao tác chưa cài, 20 thuộc Phase 2/3". Câu ấy đúng
// vào ngày viết và không có gì giữ cho nó đúng: một thao tác mới thêm vào
// đặc tả mà quên cài sẽ lẫn vào con số 24 và không ai thấy.
//
// Danh sách này biến câu văn đó thành thứ máy giữ. Thêm thao tác vào đặc tả
// mà chưa cài thì CI đỏ cho tới khi có người viết ra LÝ DO; cài xong mà
// quên xóa dòng ở đây thì CI cũng đỏ.
//
// Mỗi lý do phải nói được PHASE hoặc ĐIỀU KIỆN, không phải "chưa làm".
var chuaCai = map[string]string{
	// Nội dung bên dưới. Danh sách thứ hai — `ngoaiHopDong` — ở cuối file.
	// ------------------------------------------------ Phase 2 — Creator Commerce
	//
	// Bảy module của Phase 2 (creator, content, affiliate, campaign…) chưa
	// tồn tại. Xem docs/10-roadmap/future-phases.md mục 2.
	"POST /api/v1/creators":                             "Phase 2 — module creator chưa có",
	"GET /api/v1/creator/content":                       "Phase 2 — module content chưa có",
	"POST /api/v1/creator/content":                      "Phase 2 — module content chưa có",
	"POST /api/v1/creator/content/{content_id}/publish": "Phase 2 — module content chưa có",
	"GET /api/v1/creator/affiliate-links":               "Phase 2 — module affiliate chưa có",
	"POST /api/v1/creator/affiliate-links":              "Phase 2 — module affiliate chưa có",
	"GET /api/v1/creator/earnings":                      "Phase 2 — thu nhập creator cần affiliate + campaign",
	"GET /api/v1/creator/analytics":                     "Phase 2 — chỉ số creator cần content + affiliate",
	"GET /api/v1/content/{content_id}":                  "Phase 2 — module content chưa có",
	"GET /api/v1/feed":                                  "Phase 2 — feed khám phá cần content",
	"GET /api/v1/outfits/{outfit_id}":                   "Phase 2 — outfit thuộc module content",

	// `review` thuộc module content theo module-boundaries.md, nên nó đi
	// cùng Phase 2 chứ không phải một tính năng lẻ bị bỏ quên.
	"GET /api/v1/products/{product_id}/reviews": "Phase 2 — review thuộc module content",

	// ------------------------------------------------ Phase 2 — vận hành
	"POST /api/v1/admin/payouts": "Phase 2 — chi trả tự động; MVP chi trả tay sau đối soát",

	// ------------------------------------------------ Phase 3 — chuỗi cung ứng
	"POST /api/v1/admin/production-orders":        "Phase 3 — lệnh sản xuất",
	"GET /api/v1/admin/replenishment-suggestions": "Phase 3 — gợi ý nhập bổ sung",
}

// ngoaiHopDong là những tuyến CỐ Ý không nằm trong OpenAPI.
//
// # Vì sao có ngoại lệ, và vì sao nó phải ngắn
//
// `api/openapi.yaml` mô tả hợp đồng với CLIENT: cửa hàng, trang nhà bán,
// trang quản trị. Bốn đường dưới đây không phục vụ client nào — chúng phục
// vụ hạ tầng, và người gọi chúng (Kubernetes, Prometheus) đã có hợp đồng
// riêng do chính hạ tầng ấy định nghĩa.
//
// Đưa chúng vào đặc tả sẽ sinh kiểu TypeScript cho những thứ không trang
// nào gọi, và tệ hơn: nó gợi ý rằng `/metrics` là một phần của sản phẩm.
//
// Danh sách này phải NGẮN. Mỗi dòng thêm vào là một endpoint không ai rà
// soát khi đổi, nên lý do phải nói được vì sao client KHÔNG BAO GIỜ gọi nó.
var ngoaiHopDong = map[string]string{
	"GET /health/live":  "thăm dò của trình điều phối; hợp đồng do Kubernetes định nghĩa",
	"GET /health/ready": "thăm dò của trình điều phối; hợp đồng do Kubernetes định nghĩa",
	"GET /metrics":      "Prometheus thu thập; định dạng do Prometheus định nghĩa",
	"GET /version":      "chẩn đoán vận hành, không phải dữ liệu sản phẩm",
}
