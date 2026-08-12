// Package application chứa use case của module catalog.
//
// Tầng này ĐIỀU PHỐI: gọi domain để áp dụng quy tắc nghiệp vụ, gọi
// repository để lưu trữ, quản lý ranh giới giao dịch.
//
// Nó KHÔNG chứa quy tắc nghiệp vụ (thuộc domain) và KHÔNG biết về HTTP
// (thuộc interfaces).
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/catalog/domain"
)

// Clock cho phép test kiểm soát thời gian.
//
// Không dùng time.Now() trực tiếp trong use case: test về hạn hiệu lực
// giấy ủy quyền và lịch ra mắt bộ sưu tập cần thời gian xác định, không
// phụ thuộc lúc chạy test.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// SystemClock là đồng hồ thật, dùng ở production.
var SystemClock Clock = systemClock{}

// Service là tầng application của module catalog.
type Service struct {
	brands      domain.BrandRepository
	auths       domain.AuthorizationRepository
	collections domain.CollectionRepository
	categories  domain.CategoryRepository
	sizeCharts  domain.SizeChartRepository
	clock       Clock
}

// Deps gom các phụ thuộc — dễ đọc hơn hàm 6 tham số cùng kiểu interface.
type Deps struct {
	Brands      domain.BrandRepository
	Auths       domain.AuthorizationRepository
	Collections domain.CollectionRepository
	Categories  domain.CategoryRepository
	SizeCharts  domain.SizeChartRepository
	Clock       Clock
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	return &Service{
		brands:      d.Brands,
		auths:       d.Auths,
		collections: d.Collections,
		categories:  d.Categories,
		sizeCharts:  d.SizeCharts,
		clock:       clock,
	}
}

// Now trả về thời điểm hiện tại theo đồng hồ của service.
//
// Tầng ngoài (module.go, interfaces) phải dùng hàm này thay vì time.Now()
// để test kiểm soát được thời gian trên TOÀN BỘ đường đi, không chỉ trong
// use case.
func (s *Service) Now() time.Time { return s.clock.Now() }

// ---------------------------------------------------------------- Brand

type CreateBrandInput struct {
	Name            string
	Slug            string
	Description     string
	LogoURL         string
	BrandType       domain.BrandType
	ProtectionLevel domain.ProtectionLevel
	OwnerSellerID   ids.ID
	CountryOfOrigin string
}

func (s *Service) CreateBrand(ctx context.Context, in CreateBrandInput) (*domain.Brand, error) {
	b, err := domain.NewBrand(domain.NewBrandParams{
		Name:            in.Name,
		Slug:            in.Slug,
		Description:     in.Description,
		LogoURL:         in.LogoURL,
		BrandType:       in.BrandType,
		ProtectionLevel: in.ProtectionLevel,
		OwnerSellerID:   in.OwnerSellerID,
		CountryOfOrigin: in.CountryOfOrigin,
		Now:             s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}
	if err := s.brands.Save(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *Service) GetBrand(ctx context.Context, id ids.ID) (*domain.Brand, error) {
	return s.brands.FindByID(ctx, id)
}

func (s *Service) GetBrandBySlug(ctx context.Context, slug string) (*domain.Brand, error) {
	return s.brands.FindBySlug(ctx, slug)
}

func (s *Service) GetBrandsByIDs(ctx context.Context, list []ids.ID) (map[ids.ID]*domain.Brand, error) {
	return s.brands.FindByIDs(ctx, list)
}

func (s *Service) ListBrands(ctx context.Context, f domain.BrandFilter) ([]*domain.Brand, error) {
	return s.brands.List(ctx, f)
}

// ---------------------------------------------------- Kiểm tra quyền bán

// SellCheckResult là kết quả kiểm tra quyền bán một thương hiệu.
//
// Trả cấu trúc thay vì (bool, error) vì bên gọi cần BIẾT LÝ DO để hiển thị
// thông báo hữu ích cho seller — "cần giấy ủy quyền" khác hẳn "thương hiệu
// không tồn tại".
type SellCheckResult struct {
	Allowed         bool
	Reason          string
	ProtectionLevel domain.ProtectionLevel
	RequiredAction  string
}

// Các mã lý do — máy đọc được, để interfaces ánh xạ sang mã lỗi API.
const (
	ReasonOK                = "OK"
	ReasonBrandNotFound     = "BRAND_NOT_FOUND"
	ReasonBrandInactive     = "BRAND_INACTIVE"
	ReasonNoAuthorization   = "NO_AUTHORIZATION"
	ReasonAuthExpired       = "AUTHORIZATION_EXPIRED"
	ReasonRestrictedToOwner = "RESTRICTED_TO_DESIGNATED_SELLER"
)

// CanSellerSellBrand kiểm tra seller có được phép bán thương hiệu không.
//
// ĐÂY LÀ QUY TẮC DOMAIN BẮT BUỘC, không phải quy trình thủ công bên ngoài
// hệ thống. Hàng giả là rủi ro sống còn của marketplace thời trang:
// kiện tụng, mất quyền phân phối thương hiệu chính hãng, mất niềm tin khách.
//
// Xem docs/01-business/marketplace.md mục 4.1.
func (s *Service) CanSellerSellBrand(
	ctx context.Context, brandID, sellerID ids.ID,
) (SellCheckResult, error) {
	b, err := s.brands.FindByID(ctx, brandID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return SellCheckResult{Allowed: false, Reason: ReasonBrandNotFound}, nil
		}
		return SellCheckResult{}, err
	}

	if b.Status() != domain.StatusActive {
		return SellCheckResult{
			Allowed: false, Reason: ReasonBrandInactive,
			ProtectionLevel: b.ProtectionLevel(),
		}, nil
	}

	switch b.ProtectionLevel() {
	case domain.ProtectionOpen:
		return SellCheckResult{Allowed: true, Reason: ReasonOK,
			ProtectionLevel: domain.ProtectionOpen}, nil

	case domain.ProtectionRestricted:
		// Chỉ seller được chỉ định (chủ sở hữu thương hiệu) được bán.
		if !b.OwnerSellerID().IsZero() && b.OwnerSellerID() == sellerID {
			return SellCheckResult{Allowed: true, Reason: ReasonOK,
				ProtectionLevel: domain.ProtectionRestricted}, nil
		}
		return SellCheckResult{
			Allowed: false, Reason: ReasonRestrictedToOwner,
			ProtectionLevel: domain.ProtectionRestricted,
			RequiredAction:  "CONTACT_PLATFORM",
		}, nil

	case domain.ProtectionVerifiedOnly:
		auth, err := s.auths.FindActiveForSeller(ctx, brandID, sellerID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return SellCheckResult{
					Allowed: false, Reason: ReasonNoAuthorization,
					ProtectionLevel: domain.ProtectionVerifiedOnly,
					RequiredAction:  "UPLOAD_AUTHORIZATION",
				}, nil
			}
			return SellCheckResult{}, err
		}

		// Hạn hiệu lực kiểm tra tại THỜI ĐIỂM HIỆN TẠI — giấy hết hạn
		// tự động chặn, không cần ai nhớ thu hồi thủ công.
		if !auth.IsValidAt(s.clock.Now()) {
			return SellCheckResult{
				Allowed: false, Reason: ReasonAuthExpired,
				ProtectionLevel: domain.ProtectionVerifiedOnly,
				RequiredAction:  "RENEW_AUTHORIZATION",
			}, nil
		}
		return SellCheckResult{Allowed: true, Reason: ReasonOK,
			ProtectionLevel: domain.ProtectionVerifiedOnly}, nil

	default:
		return SellCheckResult{}, fmt.Errorf(
			"catalog: mức bảo vệ không xử lý được: %q", b.ProtectionLevel())
	}
}

// ---------------------------------------------------------------- Authorization

type GrantAuthorizationInput struct {
	BrandID     ids.ID
	SellerID    ids.ID
	DocumentURL string
	ValidFrom   time.Time
	ValidUntil  time.Time
}

func (s *Service) GrantAuthorization(
	ctx context.Context, in GrantAuthorizationInput,
) (*domain.BrandAuthorization, error) {
	// Thương hiệu phải tồn tại — tránh tạo ủy quyền cho brand không có.
	if _, err := s.brands.FindByID(ctx, in.BrandID); err != nil {
		return nil, err
	}

	a, err := domain.NewBrandAuthorization(domain.NewAuthorizationParams{
		BrandID:     in.BrandID,
		SellerID:    in.SellerID,
		DocumentURL: in.DocumentURL,
		ValidFrom:   in.ValidFrom,
		ValidUntil:  in.ValidUntil,
		Now:         s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}
	if err := s.auths.Save(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Service) ApproveAuthorization(
	ctx context.Context, authID ids.ID, approvedBy string,
) (*domain.BrandAuthorization, error) {
	a, err := s.auths.FindByID(ctx, authID)
	if err != nil {
		return nil, err
	}
	if err := a.Approve(approvedBy, s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.auths.Save(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// FindExpiringAuthorizations tìm giấy sắp hết hạn để cảnh báo seller.
//
// Cảnh báo TRƯỚC khi hết hạn: nếu chỉ chặn khi đã hết, seller mất doanh số
// đột ngột mà không hiểu vì sao.
func (s *Service) FindExpiringAuthorizations(
	ctx context.Context, withinDays int,
) ([]*domain.BrandAuthorization, error) {
	return s.auths.FindExpiring(ctx, withinDays)
}

// ---------------------------------------------------------------- Collection

type CreateCollectionInput struct {
	BrandID         ids.ID
	Name            string
	Slug            string
	Season          string
	Theme           string
	LaunchDate      time.Time
	EndOfSeasonDate time.Time
}

func (s *Service) CreateCollection(
	ctx context.Context, in CreateCollectionInput,
) (*domain.Collection, error) {
	if _, err := s.brands.FindByID(ctx, in.BrandID); err != nil {
		return nil, err
	}

	c, err := domain.NewCollection(domain.NewCollectionParams{
		BrandID:         in.BrandID,
		Name:            in.Name,
		Slug:            in.Slug,
		Season:          in.Season,
		Theme:           in.Theme,
		LaunchDate:      in.LaunchDate,
		EndOfSeasonDate: in.EndOfSeasonDate,
		Now:             s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}
	if err := s.collections.Save(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) GetCollection(ctx context.Context, id ids.ID) (*domain.Collection, error) {
	return s.collections.FindByID(ctx, id)
}

// ListCollectionsByBrand trả MỌI bộ sưu tập của thương hiệu, kể cả bộ chưa
// ra mắt.
//
// Việc lọc theo người xem thuộc về bên gọi: trang khách hàng chỉ hiện bộ
// đang bán, còn trang quản trị cần thấy cả bộ đang chuẩn bị. Nếu lọc sẵn ở
// đây, tầng quản trị sẽ không có cách nào lấy đủ dữ liệu.
func (s *Service) ListCollectionsByBrand(ctx context.Context, brandID ids.ID) ([]*domain.Collection, error) {
	return s.collections.FindByBrand(ctx, brandID)
}

func (s *Service) LaunchCollection(ctx context.Context, id ids.ID) (*domain.Collection, error) {
	c, err := s.collections.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := c.Launch(s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.collections.Save(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// ProcessScheduledCollections chuyển trạng thái bộ sưu tập theo lịch.
//
// Chạy bởi job định kỳ trong cmd/worker. Đây là cơ chế "xuất bản có lịch"
// thay cho việc nhân đôi bảng như QOR Publish2 — xem docs/11-oss/qor.md.
//
// Trả về số bộ sưu tập đã ra mắt và số đã chuyển sang giai đoạn xả hàng.
func (s *Service) ProcessScheduledCollections(ctx context.Context) (launched, ending int, err error) {
	now := s.clock.Now()

	planning, err := s.collections.FindByStatus(ctx, domain.CollectionPlanning)
	if err != nil {
		return 0, 0, err
	}
	for _, c := range planning {
		if !c.ShouldLaunch(now) {
			continue
		}
		if err := c.Launch(now); err != nil {
			return launched, ending, err
		}
		if err := s.collections.Save(ctx, c); err != nil {
			return launched, ending, err
		}
		launched++
	}

	active, err := s.collections.FindByStatus(ctx, domain.CollectionActive)
	if err != nil {
		return launched, ending, err
	}
	for _, c := range active {
		if !c.ShouldMarkEnding(now) {
			continue
		}
		if err := c.MarkEnding(now); err != nil {
			return launched, ending, err
		}
		if err := s.collections.Save(ctx, c); err != nil {
			return launched, ending, err
		}
		ending++
	}

	return launched, ending, nil
}

// ---------------------------------------------------------------- Category

type CreateCategoryInput struct {
	ParentID     ids.ID
	Name         string
	Slug         string
	DisplayOrder int
}

func (s *Service) CreateCategory(ctx context.Context, in CreateCategoryInput) (*domain.Category, error) {
	parentDepth := -1
	if !in.ParentID.IsZero() {
		parent, err := s.categories.FindByID(ctx, in.ParentID)
		if err != nil {
			return nil, err
		}
		parentDepth = parent.Depth()
	}

	c, err := domain.NewCategory(domain.NewCategoryParams{
		ParentID:     in.ParentID,
		ParentDepth:  parentDepth,
		Name:         in.Name,
		Slug:         in.Slug,
		DisplayOrder: in.DisplayOrder,
		Now:          s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}
	if err := s.categories.Save(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// CategoryNode là một nút trong cây danh mục trả về cho client.
type CategoryNode struct {
	Category *domain.Category
	Children []*CategoryNode
}

// GetCategoryTree dựng cây danh mục từ danh sách phẳng.
//
// Tải toàn bộ rồi dựng cây trong bộ nhớ: danh mục ít (hàng trăm) và đổi
// hiếm, nên cách này rẻ hơn truy vấn đệ quy trong database.
func (s *Service) GetCategoryTree(ctx context.Context) ([]*CategoryNode, error) {
	all, err := s.categories.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	nodes := make(map[ids.ID]*CategoryNode, len(all))
	for _, c := range all {
		nodes[c.ID()] = &CategoryNode{Category: c}
	}

	roots := make([]*CategoryNode, 0)
	for _, c := range all {
		node := nodes[c.ID()]
		if c.IsRoot() {
			roots = append(roots, node)
			continue
		}
		if parent, ok := nodes[c.ParentID()]; ok {
			parent.Children = append(parent.Children, node)
			continue
		}
		// Cha không tồn tại (dữ liệu không nhất quán) → coi như gốc,
		// để danh mục không biến mất khỏi giao diện.
		roots = append(roots, node)
	}
	return roots, nil
}

func (s *Service) GetCategory(ctx context.Context, id ids.ID) (*domain.Category, error) {
	return s.categories.FindByID(ctx, id)
}

// ---------------------------------------------------------------- SizeChart

type CreateSizeChartInput struct {
	BrandID     ids.ID
	ProductType domain.ProductType
	System      domain.SizeSystem
	Note        string
	Entries     []domain.SizeEntry
}

func (s *Service) CreateSizeChart(ctx context.Context, in CreateSizeChartInput) (*domain.SizeChart, error) {
	if _, err := s.brands.FindByID(ctx, in.BrandID); err != nil {
		return nil, err
	}

	sc, err := domain.NewSizeChart(domain.NewSizeChartParams{
		BrandID:     in.BrandID,
		ProductType: in.ProductType,
		System:      in.System,
		Note:        in.Note,
		Entries:     in.Entries,
		Now:         s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}
	if err := s.sizeCharts.Save(ctx, sc); err != nil {
		return nil, err
	}
	return sc, nil
}

func (s *Service) GetSizeChart(ctx context.Context, id ids.ID) (*domain.SizeChart, error) {
	return s.sizeCharts.FindByID(ctx, id)
}

// GetSizeChartFor tìm bảng size áp dụng cho một loại sản phẩm của thương hiệu.
//
// Dùng khi hiển thị trang sản phẩm — bảng size có số đo thực tế là một
// trong ba yếu tố giảm trực tiếp tỷ lệ hoàn hàng.
func (s *Service) GetSizeChartFor(
	ctx context.Context, brandID ids.ID, pt domain.ProductType,
) (*domain.SizeChart, error) {
	return s.sizeCharts.FindForBrandAndType(ctx, brandID, pt)
}
