import type { NextConfig } from "next";

const config: NextConfig = {
  // Biên dịch package trong workspace: chúng xuất TypeScript nguồn, không
  // phải JS đã build. Đổi lại là không có bước build riêng cho từng package.
  transpilePackages: ["@fc/ui", "@fc/api-client"],

  // Trung tâm người bán là ứng dụng nội bộ: KHÔNG cần SEO, và không được
  // để công cụ tìm kiếm lập chỉ mục.
  async headers() {
    return [
      {
        source: "/:path*",
        headers: [{ key: "X-Robots-Tag", value: "noindex, nofollow" }],
      },
    ];
  },
};

export default config;
