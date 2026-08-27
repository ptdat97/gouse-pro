import "@fc/design-tokens/tokens.css";
import "@fc/ui/styles.css";
import "./globals.css";

import type { Metadata } from "next";

import { Header } from "@/components/header";
import { ShopProvider } from "@/lib/shop";

export const metadata: Metadata = {
  title: "Fashion Commerce",
  description: "Thời trang thiết kế và hàng chọn lọc từ các nhà bán.",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="vi">
      <body>
        <ShopProvider>
          <Header />
          <main className="page">{children}</main>
        </ShopProvider>
      </body>
    </html>
  );
}
