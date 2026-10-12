import type { LiveRun } from "./live-contract";

export interface HistoricalBar { trade_date: string; open: number; high: number; low: number; close: number; volume: number; amount: number }
export interface HistoricalSeries { symbol: string; requested_as_of: string; effective_as_of: string; price_basis: "RAW"; bars: HistoricalBar[] }
const object = (v: unknown): Record<string,unknown> | null => v !== null && typeof v === "object" && !Array.isArray(v) ? v as Record<string,unknown> : null;
const ymd = (v: string) => /^\d{8}$/.test(v) && !Number.isNaN(Date.parse(v.slice(0,4)+"-"+v.slice(4,6)+"-"+v.slice(6,8)+"T00:00:00Z"));
const num = (v: unknown): v is number => typeof v === "number" && Number.isFinite(v);
export function readHistoricalSeries(run: LiveRun): HistoricalSeries | null {
  if (run.state !== "COMPLETED") return null;
  const raw=object(run.historical_ohlcv);
  if (!raw || raw.symbol !== run.symbol || raw.price_basis !== "RAW" || !Array.isArray(raw.bars) || raw.bars.length<2 || raw.bars.length>512) return null;
  const requested=typeof raw.requested_as_of==="string"?raw.requested_as_of:"";
  const effective=typeof raw.effective_as_of==="string"?raw.effective_as_of:"";
  const cutoff = new Date(run.requested_as_of);
  if (!Number.isFinite(cutoff.getTime())) return null;
  const cnDay=new Intl.DateTimeFormat("en-CA",{timeZone:"Asia/Shanghai",year:"numeric",month:"2-digit",day:"2-digit"}).format(cutoff).replaceAll("-","");
  if (!ymd(requested) || requested!==cnDay || !ymd(effective) || effective>requested) return null;
  let last="";
  const bars: HistoricalBar[]=[];
  for (const item of raw.bars) {
    const b=object(item);
    if (!b || typeof b.trade_date!=="string" || !ymd(b.trade_date) || b.trade_date<=last || b.trade_date>effective) return null;
    if (![b.open,b.high,b.low,b.close,b.volume,b.amount].every(num)) return null;
    const {open,high,low,close,volume,amount}=b as unknown as HistoricalBar;
    if (open<=0||close<=0||low<=0||high<Math.max(open,close)||low>Math.min(open,close)||high<low||volume<0||amount<0) return null;
    bars.push({trade_date:b.trade_date,open,high,low,close,volume,amount});
    last=b.trade_date;
  }
  if (last!==effective) return null;
  return {symbol:run.symbol,requested_as_of:requested,effective_as_of:effective,price_basis:"RAW",bars};
}
