// Package e2e chạy các luồng đi QUA NHIỀU MODULE bằng module thật.
//
// # Vì sao cần một gói riêng
//
// Test trong từng module dựng bản giả cho hàng xóm — đúng, và cần thiết:
// nhờ vậy mỗi module kiểm chứng được logic của mình mà không phải khởi
// động nửa hệ thống.
//
// Nhưng bản giả luôn cư xử như bên viết test NGHĨ là hàng xóm sẽ cư xử.
// Khi giả định đó sai, mọi test đều xanh và lỗi vẫn còn nguyên. P3-18 sống
// sót đúng kiểu đó: test checkout nhập hàng cho nền tảng rồi bán qua một
// seller sinh ngẫu nhiên — một tổ hợp không tồn tại được ngoài đời, và
// không có gì trong gói checkout đủ tầm nhìn để nhận ra.
//
// Gói này dựng chuỗi thật: giữ hàng → đặt đơn → phát event → tách đơn thực
// hiện. Nó KHÔNG thay thế test từng module; nó bắt loại lỗi mà test từng
// module không thể thấy — lỗi nằm ở KHOẢNG GIỮA.
//
// # Ranh giới
//
// Ở đây dùng module thật và PostgreSQL thật. Chậm hơn test đơn vị, nên chỉ
// dựng những luồng mà việc đi qua ranh giới CHÍNH LÀ điều cần kiểm chứng.
package e2e
