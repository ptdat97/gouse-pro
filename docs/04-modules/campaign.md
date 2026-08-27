# Module: Campaign

| | |
|---|---|
| **Bounded Context** | Growth |
| **Phân loại** | Supporting |
| **Giai đoạn** | Phase 2 |

---

## 1. Trách nhiệm

- Quản lý chiến dịch marketing và chiến dịch creator
- Quản lý ngân sách và cấu trúc chi phí
- Mời và quản lý người tham gia (creator, seller)
- Xác định tỷ lệ hoa hồng và bên chịu chi phí trong chiến dịch

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Quy kết đơn hàng | `affiliate` |
| Nội dung chiến dịch | `content` |
| Mã giảm giá | `promotion` |
| Chi trả tiền | `payment` |

---

## 3. Ba cấu trúc chi phí — yêu cầu bắt buộc

Đây là điểm thiết kế quan trọng nhất của module.

```text
Campaign {
    fee_structure    COMMISSION_ONLY | FIXED_FEE | HYBRID

    commission_rate      -- nếu COMMISSION_ONLY hoặc HYBRID
    fixed_fee_amount     -- nếu FIXED_FEE hoặc HYBRID

    cost_bearer      PLATFORM | SELLER | SHARED
    platform_share   -- nếu SHARED
    seller_share     -- nếu SHARED

    budget_total
    budget_spent
    date_range
}
```

### Vì sao cần cả ba

| Loại creator | Cấu trúc phù hợp | Lý do |
|---|---|---|
| KOC | `COMMISSION_ONLY` | Chấp nhận rủi ro doanh số, quy mô nhỏ |
| KOL | `FIXED_FEE` hoặc `HYBRID` | Không chấp nhận rủi ro, yêu cầu phí trước |
| Content Partner | `FIXED_FEE` | Hợp đồng sản xuất nội dung |

**Sai lầm cần tránh:** thiết kế `Campaign` chỉ với một trường `commission_rate` — sẽ không mô hình hóa được KOL yêu cầu phí cố định, và phải xử lý ngoài hệ thống.

---

## 4. Vòng đời

```text
    DRAFT
      ↓
    SCHEDULED (đã lên lịch, chưa tới ngày)
      ↓
    ACTIVE
      │
      ├──→ PAUSED (tạm dừng, ví dụ hết ngân sách)
      │
      ↓
    ENDED
      ↓
    SETTLED (đã quyết toán chi phí)
```

### Kiểm soát ngân sách

```text
Mỗi khi phát sinh chi phí (hoa hồng, phí cố định):
    budget_spent += chi phí
    nếu budget_spent >= budget_total:
        → chuyển PAUSED
        → phát campaign.budget_exhausted
        → ngừng quy kết hoa hồng mới
```

**Lưu ý:** cần xử lý trường hợp vượt ngân sách do độ trễ — đơn hàng đang xử lý khi ngân sách vừa hết. Chính sách khuyến nghị: **tôn trọng cam kết** với đơn đã phát sinh, chỉ chặn đơn mới.

---

## 5. Dữ liệu sở hữu

```sql
campaign
campaign_participant     -- creator/seller tham gia
campaign_rule            -- điều kiện, tỷ lệ
campaign_budget          -- theo dõi ngân sách
campaign_product         -- sản phẩm trong chiến dịch
```

---

## 6. Interface công khai

```go
type PublicAPI interface {
    GetCampaign(ctx, campaignID string) (*CampaignView, error)
    GetActiveCampaigns(ctx, filter CampaignFilter) ([]CampaignView, error)

    IsCreatorInCampaign(ctx, campaignID, creatorID string) (bool, error)
    GetCampaignCommissionRate(ctx, req RateRequest) (*CommissionTerms, error)

    RecordSpend(ctx, campaignID string, amount Money, ref string) error
}

type CommissionTerms struct {
    Rate        Percentage
    CostBearer  string
    FeeStructure string
}
```

---

## 7. Event

**Phát ra:** `campaign.created`, `campaign.started`, `campaign.paused`, `campaign.budget_exhausted`, `campaign.ended`, `campaign.participant_joined`

**Lắng nghe:**

| Event | Từ | Hành động |
|---|---|---|
| `affiliate.conversion_attributed` | affiliate | Cập nhật ngân sách đã tiêu |
| `affiliate.attribution_reversed` | affiliate | Hoàn lại ngân sách |

---

## 8. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | Hỗ trợ cả ba cấu trúc chi phí |
| 2 | Luôn xác định rõ bên chịu chi phí |
| 3 | Ngày kết thúc sau ngày bắt đầu |
| 4 | Hết ngân sách → tạm dừng, không quy kết mới |
| 5 | Tôn trọng cam kết với đơn đã phát sinh |
| 6 | Chiến dịch có trả phí → nội dung tự động gắn nhãn tài trợ |

---

## 9. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **Phase 2** | Chiến dịch cơ bản, ba cấu trúc chi phí, ngân sách |
| **Phase 3** | Mời creator, báo cáo hiệu quả chiến dịch |
| **Phase 4** | Creator marketplace, đấu giá vị trí |

---

## 10. Tài liệu liên quan

- [affiliate.md](affiliate.md), [creator.md](creator.md), [content.md](content.md)
- [../01-business/monetization.md](../01-business/monetization.md) mục 3
