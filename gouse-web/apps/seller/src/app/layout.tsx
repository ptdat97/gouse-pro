import "@fc/design-tokens/tokens.css";
import "@fc/ui/styles.css";
import "./globals.css";

import type { Metadata } from "next";

import { SessionProvider } from "@/lib/session";

export const metadata: Metadata = {
  title: "Nhà bán — Fashion Commerce",
  // Ứng dụng nội bộ của nhà bán: không lập chỉ mục, không cần SEO.
  robots: { index: false, follow: false },
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="vi">
      <body>
        <SessionProvider>{children}</SessionProvider>
      </body>
    </html>
  );
}
