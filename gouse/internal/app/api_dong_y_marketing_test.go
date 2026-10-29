package app

import (
	"testing"

	customerdom "github.com/fashion-commerce/platform/internal/modules/customer/domain"
	notifdom "github.com/fashion-commerce/platform/internal/modules/notification/domain"
)

// TestTheTwoConsentCopiesMustAgree.
//
// `notification` CHÉP hai chuỗi loại đồng ý thay vì import `customer`: nó
// không phụ thuộc module ấy, và kéo cả một module vào chỉ vì hai chuỗi là
// cái giá quá đắt.
//
// Cái giá của việc chép là hai bản lệch nhau — và lệch ở đây hỏng IM
// LẶNG: `customer.HasConsent` nhận một loại nó không biết, trả `false`,
// nên MỌI thư marketing bị chặn vĩnh viễn. Không có lỗi nào, không có
// cảnh báo nào; chỉ có một kênh liên lạc chết.
//
// Gói `app` là nơi duy nhất biết cả hai module, nên phép đối chiếu nằm ở
// đây.
func TestTheTwoConsentCopiesMustAgree(t *testing.T) {
	for _, c := range []struct {
		notif    string
		customer customerdom.ConsentType
	}{
		{notifdom.ConsentEmailMarketing, customerdom.ConsentMarketingEmail},
		{notifdom.ConsentSMSMarketing, customerdom.ConsentMarketingSMS},
	} {
		if c.notif != string(c.customer) {
			t.Errorf("notification dùng %q, customer dùng %q — "+
				"`HasConsent` sẽ trả false cho mọi khách và kênh này chết "+
				"im lặng", c.notif, c.customer)
		}
		if !customerdom.ValidConsentType(c.customer) {
			t.Errorf("%q không phải loại đồng ý hợp lệ của customer", c.customer)
		}
	}
}
