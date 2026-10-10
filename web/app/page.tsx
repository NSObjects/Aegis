import ResearchWorkbench from "@/components/research-workbench";
import LiveResearchWorkbench from "@/components/live-research-workbench";

export const dynamic = "force-dynamic";

export default function HomePage() {
  // Never mix synthetic market data with server-sourced artifacts on one page.
  return process.env.AEGIS_WEB_MODE === "live" ? <LiveResearchWorkbench /> : <ResearchWorkbench />;
}
