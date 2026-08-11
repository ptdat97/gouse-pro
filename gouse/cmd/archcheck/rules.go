// Command archcheck thực thi ranh giới module bằng phân tích tĩnh.
//
// Kỷ luật con người KHÔNG ĐỦ để giữ ranh giới module: người mới không biết
// quy tắc, áp lực deadline làm người ta đi đường tắt, người rà soát bỏ sót.
// Vi phạm nhỏ tích lũy → sau 2 năm không tách service được nữa.
//
// Công cụ này chạy trong CI. Vi phạm làm CI THẤT BẠI, không phải cảnh báo —
// cảnh báo sẽ bị bỏ qua.
//
// Xem docs/adr/0005-module-boundaries.md và
// docs/03-architecture/dependency-rules.md mục 9.
package main

import (
	"fmt"
	"strings"
)

// Violation là một vi phạm ranh giới được phát hiện.
type Violation struct {
	Rule    string // mã quy tắc, ví dụ "R1"
	File    string // đường dẫn tương đối
	Line    int
	Message string
	Hint    string // gợi ý cách sửa
}

func (v Violation) String() string {
	s := fmt.Sprintf("%s:%d\n  [%s] %s", v.File, v.Line, v.Rule, v.Message)
	if v.Hint != "" {
		s += "\n  → " + v.Hint
	}
	return s
}

// Layer là tầng trong một module.
type Layer string

const (
	LayerDomain         Layer = "domain"
	LayerApplication    Layer = "application"
	LayerInfrastructure Layer = "infrastructure"
	LayerInterfaces     Layer = "interfaces"
	LayerPublic         Layer = "public" // file public.go ở gốc module
	LayerUnknown        Layer = ""
)

// pkgInfo mô tả vị trí của một package trong kiến trúc.
type pkgInfo struct {
	ImportPath string
	Module     string // tên module nghiệp vụ, rỗng nếu không thuộc module nào
	Layer      Layer
	IsKernel   bool
	IsPlatform bool
	IsCmd      bool
}

// classify xác định package thuộc phần nào của kiến trúc.
//
// Cấu trúc chuẩn:
//
//	internal/modules/<name>/public.go        → LayerPublic
//	internal/modules/<name>/domain/...       → LayerDomain
//	internal/modules/<name>/application/...  → LayerApplication
//	internal/modules/<name>/infrastructure/… → LayerInfrastructure
//	internal/modules/<name>/interfaces/...   → LayerInterfaces
//	internal/kernel/...                      → IsKernel
//	internal/platform/...                    → IsPlatform
//	cmd/...                                  → IsCmd
func classify(modulePath, importPath string) pkgInfo {
	info := pkgInfo{ImportPath: importPath}

	rel, ok := strings.CutPrefix(importPath, modulePath+"/")
	if !ok {
		return info // package ngoài dự án
	}

	switch {
	case strings.HasPrefix(rel, "internal/kernel"):
		info.IsKernel = true
	case strings.HasPrefix(rel, "internal/platform"):
		info.IsPlatform = true
	case strings.HasPrefix(rel, "cmd/"):
		info.IsCmd = true
	case strings.HasPrefix(rel, "internal/modules/"):
		parts := strings.Split(strings.TrimPrefix(rel, "internal/modules/"), "/")
		info.Module = parts[0]
		if len(parts) == 1 {
			info.Layer = LayerPublic
		} else {
			switch parts[1] {
			case "domain":
				info.Layer = LayerDomain
			case "application":
				info.Layer = LayerApplication
			case "infrastructure":
				info.Layer = LayerInfrastructure
			case "interfaces":
				info.Layer = LayerInterfaces
			default:
				info.Layer = LayerUnknown
			}
		}
	}
	return info
}

// forbiddenDirs là các thư mục bị CẤM tồn tại (nguyên tắc P12).
//
// Những thư mục này trở thành bãi rác phụ thuộc: mọi module import chúng,
// logic nghiệp vụ đi lạc vào đó, và chúng phá hủy tính module một cách âm thầm.
//
// Code dùng chung phải được phân loại rõ ràng thành kernel/ (khái niệm domain
// chung), platform/ (hạ tầng trung lập domain), hoặc pkg/ (tiện ích thuần).
var forbiddenDirs = []string{"common", "utils", "helpers", "services", "shared", "misc"}

// checkImport áp dụng các quy tắc phụ thuộc cho một cặp (package, import).
//
// Trả về mã quy tắc bị vi phạm và thông báo, hoặc chuỗi rỗng nếu hợp lệ.
func checkImport(from, to pkgInfo) (rule, msg, hint string) {
	// Bỏ qua import ngoài dự án và import chính mình.
	if to.ImportPath == "" || (to.Module == "" && !to.IsKernel && !to.IsPlatform && !to.IsCmd) {
		return "", "", ""
	}

	// ---- R4: kernel chỉ dùng thư viện chuẩn ----
	// kernel là phụ thuộc của TOÀN BỘ hệ thống. Thay đổi ở đây ảnh hưởng
	// mọi module, nên phải giữ nó tối thiểu tuyệt đối.
	if from.IsKernel {
		if to.IsPlatform || to.Module != "" {
			return "R4",
				fmt.Sprintf("kernel không được import %s", to.ImportPath),
				"kernel chỉ được dùng thư viện chuẩn và các package kernel khác"
		}
	}

	// ---- R3: platform không biết nghiệp vụ ----
	// Nếu platform biết về "order" hay "seller", nó không còn trung lập
	// và trở thành điểm phụ thuộc của toàn hệ thống.
	if from.IsPlatform && to.Module != "" {
		return "R3",
			fmt.Sprintf("platform không được import module nghiệp vụ %q", to.Module),
			"nếu platform cần khái niệm nghiệp vụ, nó đặt sai chỗ — chuyển vào module"
	}

	// Các quy tắc còn lại chỉ áp dụng cho code trong module nghiệp vụ.
	if from.Module == "" {
		return "", "", ""
	}

	// ---- R2: domain layer sạch ----
	// Điều kiện để kiểm thử quy tắc nghiệp vụ mà không cần database,
	// không cần HTTP, không cần dịch vụ ngoài.
	if from.Layer == LayerDomain {
		if to.IsPlatform {
			return "R2",
				fmt.Sprintf("domain layer không được import platform: %s", to.ImportPath),
				"domain định nghĩa port (interface); infrastructure cài đặt port đó"
		}
		if to.Module != "" && to.Module != from.Module {
			return "R2",
				fmt.Sprintf("domain layer không được import module khác: %s", to.ImportPath),
				"domain chỉ phụ thuộc chính nó và kernel"
		}
	}

	// ---- R1: chỉ import public.go của module khác ----
	// Interface công khai là điểm vào DUY NHẤT. Import sâu vào domain/
	// hay infrastructure/ của module khác phá vỡ tính đóng gói và làm
	// mọi thay đổi nội bộ trở thành thay đổi phá vỡ.
	if to.Module != "" && to.Module != from.Module && to.Layer != LayerPublic {
		return "R1",
			fmt.Sprintf("import sâu vào module %q (tầng %q): %s", to.Module, to.Layer, to.ImportPath),
			fmt.Sprintf("chỉ được import internal/modules/%s (public.go)", to.Module)
	}

	// ---- R8: tầng trong module đi đúng chiều ----
	// interfaces → application → domain ← infrastructure
	if from.Layer == LayerInterfaces && to.Module == from.Module {
		if to.Layer == LayerInfrastructure {
			return "R8",
				"interfaces không được import infrastructure trực tiếp",
				"interfaces gọi application; application dùng port do domain định nghĩa"
		}
	}
	if from.Layer == LayerInfrastructure && to.Module == from.Module {
		if to.Layer == LayerApplication || to.Layer == LayerInterfaces {
			return "R8",
				fmt.Sprintf("infrastructure không được import %s", to.Layer),
				"infrastructure chỉ cài đặt port do domain định nghĩa"
		}
	}

	return "", "", ""
}
