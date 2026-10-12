"use client";

import type { LiveRun } from "@/lib/live-contract";
import { readHistoricalSeries } from "@/lib/historical-ohlcv";

const n=(v:number)=>new Intl.NumberFormat("zh-CN",{maximumFractionDigits:2}).format(v);
export default function LiveHistoricalChart({run}:{run:LiveRun}) {
 const history=readHistoricalSeries(run);
 if(!history)return null;
 const bars=history.bars.slice(-60);
 const hi=Math.max(...bars.map(b=>b.high));
 const lo=Math.min(...bars.map(b=>b.low));
 const delta=hi-lo || 1;
 const y=(value:number)=>20+(hi-value)*170/delta;
 const w=720;
 const step=(w-50)/bars.length;
 return <section className="surface technical-visual" aria-label="真实多日行情 K 线">
  <header className="technical-visual-head"><div><div className="eyebrow">Persisted research-time OHLCV · RAW</div><h3>真实日 K 线 <span>最近 {bars.length} 个已观测交易日 · 非实时</span></h3></div><span className="badge blue">原始价格</span></header>
  <p className="technical-source-note">标的 {history.symbol} · 截止 {history.requested_as_of} · 最新交易日 {history.effective_as_of}。此图来自当前 ResearchRun 持久化的原始日线；与基于 PIT 前复权价格计算的 MA 不叠加。</p>
  <svg viewBox="0 0 720 235" role="img" aria-label={`真实历史 K 线，${bars.length} 个交易日，最低 ${n(lo)}，最高 ${n(hi)}`} style={{width:"100%",height:"auto"}}>
   <line x1="25" x2="710" y1="20" y2="20" className="technical-chart-grid"/>
   <line x1="25" x2="710" y1="190" y2="190" className="technical-chart-grid"/>
   {bars.map((b,i)=>{
    const x=32+i*step+step/2;const up=b.close>=b.open;const top=Math.min(y(b.open),y(b.close));const height=Math.max(1,Math.abs(y(b.open)-y(b.close)));
    return <g key={b.trade_date} className={up?"technical-candle-up":"technical-candle-down"}>
     <title>{`${b.trade_date}: 开 ${n(b.open)} 高 ${n(b.high)} 低 ${n(b.low)} 收 ${n(b.close)} 量 ${n(b.volume)}`}</title>
     <line x1={x} x2={x} y1={y(b.high)} y2={y(b.low)} className="technical-candle-wick"/>
     <rect x={x-Math.max(1,step*0.28)} y={top} width={Math.max(2,step*0.56)} height={height} className="technical-candle-body"/>
    </g>;
   })}
   <text x="26" y="12" className="technical-chart-label">HIGH {n(hi)}</text>
   <text x="26" y="207" className="technical-chart-label">LOW {n(lo)}</text>
   <text x="25" y="228" className="technical-chart-label">{bars[0].trade_date}</text>
   <text x="610" y="228" className="technical-chart-label">{bars[bars.length-1].trade_date}</text>
  </svg>
 </section>;
}
