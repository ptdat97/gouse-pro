import type { NextConfig } from "next";

const config: NextConfig = {
  // Biên dịch package trong workspace: chúng xuất TypeScript nguồn, không
  // phải JS đã build.
  transpilePackages: ["@fc/ui", "@fc/api-client"],

  // KHÔNG chặn lập chỉ mục — ngược hẳn admin. Cửa hàng sống bằng tìm kiếm
  // tự nhiên; chặn robot ở đây là tự cắt nguồn khách.
};

export default config;
