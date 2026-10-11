import ResearchWorkbench from "@/components/research-workbench";
import LiveResearchWorkbench from "@/components/live-research-workbench";
import { isDemoMode } from "@/lib/web-mode";

export const dynamic = "force-dynamic";

export default function HomePage() {
  // Real workbench by default. Synthetic charts require explicit demo opt-in.
  return isDemoMode(process.env.AEGIS_WEB_MODE) ? <ResearchWorkbench /> : <LiveResearchWorkbench />;
}
