import type { Metadata } from "next";
import "./globals.css";
import "./aegis-workbench-refresh.css";

export const metadata: Metadata = {
  title: "Aegis · AI Research Workspace",
  description: "多 Agent 股票研究工作台 · Go Live Research",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="zh-CN">
      <body>{children}</body>
    </html>
  );
}
