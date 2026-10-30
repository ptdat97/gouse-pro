package main

import "go/ast"

// declaredIdents trả mọi định danh KHAI BÁO trong một tệp.
//
// # Vì sao phải liệt kê từng dạng nút thay vì bắt `*ast.Ident`
//
// Bắt mọi `*ast.Ident` là bắt cả nơi DÙNG, nên mỗi lời gọi `fmt.Println`
// thành một "định danh" và phép đo chìm trong tên của thư viện ngoài.
// Thứ cần đo là tên mà REPO NÀY đặt ra.
//
// # Năm dạng đã bị bỏ sót một lần
//
//	tham số hàm          func f(tapCon bool)        ← sống sót cả lượt rà
//	kết quả có tên       func f() (ra []string)
//	var (…) trong hàm    var ( a = 1; b = 2 )
//	biến for … range     for _, ten := range xs
//	tham số func literal func(ten string) { … }
//
// Dạng đầu là dạng đã cắn thật: `tapCon` ở `cmd/apicheck` sống qua cả
// lượt đổi tên vì phép rà cũ chỉ đọc đầu dòng khai báo.
func declaredIdents(f *ast.File) []*ast.Ident {
	var out []*ast.Ident

	add := func(ids ...*ast.Ident) {
		for _, id := range ids {
			if id != nil && id.Name != "_" {
				out = append(out, id)
			}
		}
	}

	// fieldNames lấy tên của tham số, kết quả, trường struct và method
	// của interface — bốn thứ Go biểu diễn bằng CÙNG một `ast.FieldList`.
	fieldNames := func(fl *ast.FieldList) {
		if fl == nil {
			return
		}
		for _, fld := range fl.List {
			add(fld.Names...)
		}
	}

	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			add(x.Name)
			fieldNames(x.Recv)

		case *ast.FuncType:
			// Bắt ở FuncType, không ở FuncDecl: như vậy phủ CẢ func
			// literal, nơi tham số cũng là tên do repo đặt.
			fieldNames(x.Params)
			fieldNames(x.Results)

		case *ast.StructType:
			fieldNames(x.Fields)

		case *ast.InterfaceType:
			fieldNames(x.Methods)

		case *ast.ValueSpec:
			add(x.Names...)

		case *ast.TypeSpec:
			add(x.Name)
			fieldNames(x.TypeParams)

		case *ast.AssignStmt:
			if x.Tok.String() != ":=" {
				return true
			}
			for _, lhs := range x.Lhs {
				if id, ok := lhs.(*ast.Ident); ok {
					add(id)
				}
			}

		case *ast.RangeStmt:
			if id, ok := x.Key.(*ast.Ident); ok {
				add(id)
			}
			if id, ok := x.Value.(*ast.Ident); ok {
				add(id)
			}

		case *ast.LabeledStmt:
			add(x.Label)
		}
		return true
	})
	return out
}
