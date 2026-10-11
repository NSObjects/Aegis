"use client";

import type { LiveArtifact, LiveRun } from "@/lib/live-contract";
import { readTechnicalVisual, type Candle, type Scalar } from "@/lib/technical-visual";

const display = (n: number, digits = 4) =>
  new Intl.NumberFormat("zh-CN", { maximumFractionDigits: digits }).format(n);
const dateLabel = (date: string | null) =>
  date && date.length === 8 ? date.slice(0, 4) + "-" + date.slice(4, 6) + "-" + date.slice(6) : "未提供";

function DailyCandle({ bar }: { bar: Candle }) {
  const range = bar.high - bar.low;
  const y = (price: number) => range === 0 ? 116 : 32 + (bar.high - price) * 158 / range;
  const openY = y(bar.open);
  const closeY = y(bar.close);
  const top = Math.min(openY, closeY);
  const height = Math.max(2, Math.abs(openY - closeY));
  const direction = bar.close > bar.open ? "up" : bar.close < bar.open ? "down" : "flat";
  return <div className={"technical-candle-chart technical-candle-" + direction}>
    <svg viewBox="0 0 460 228" role="img" aria-label="单个有效交易日的开盘价、最高价、最低价和收盘价（非历史走势图）">
      <line x1="30" x2="435" y1="32" y2="32" className="technical-chart-grid"/>
      <line x1="30" x2="435" y1="111" y2="111" className="technical-chart-grid"/>
      <line x1="30" x2="435" y1="190" y2="190" className="technical-chart-grid"/>
      <line x1="226" x2="226" y1={y(bar.high)} y2={y(bar.low)} className="technical-candle-wick"/>
      <rect x="196" width="60" y={top} height={height} rx="2" className="technical-candle-body"/>
      <text x="33" y="23" className="technical-chart-label">HIGH  {display(bar.high)}</text>
      <text x="33" y="211" className="technical-chart-label">LOW  {display(bar.low)}</text>
      <text x="283" y="104" className="technical-chart-label">OPEN  {display(bar.open)}</text>
      <text x="283" y="121" className="technical-chart-label">CLOSE  {display(bar.close)}</text>
    </svg>
    <div className="technical-candle-prices">
      <span>开 <strong>{display(bar.open)}</strong></span>
      <span>高 <strong>{display(bar.high)}</strong></span>
      <span>低 <strong>{display(bar.low)}</strong></span>
      <span>收 <strong>{display(bar.close)}</strong></span>
      {bar.preClose !== null && <span>昨收 <strong>{display(bar.preClose)}</strong></span>}
      {bar.changePercent !== null && <span>源涨跌幅 <strong>{display(bar.changePercent)}%</strong></span>}
    </div>
  </div>;
}

function FigureMetrics({ title, items, empty = "后端未提供该组指标" }: {
  title: string; items: Scalar[]; empty?: string;
}) {
  return <div className="technical-metric-panel">
    <h4>{title}</h4>
    {items.length ? <dl className="technical-metric-grid">{items.map(item =>
      <div key={item.key}><dt>{item.label}</dt><dd>{display(item.value)}{item.unit ? " " + item.unit : ""}</dd></div>
    )}</dl> : <p className="technical-missing">{empty}</p>}
  </div>;
}

export default function LiveTechnicalVisual({ artifact, run }: { artifact: LiveArtifact; run: LiveRun }) {
  const tech = readTechnicalVisual(artifact, run);
  if (!tech || run.state !== "COMPLETED") return null;

  const rsi = tech.momentum.find(m => m.key === "rsi14");
  const ratio = tech.volumes.find(m => m.key === "volume_vs_ma5");

  return <section id="report-technical-visual" className="surface technical-visual" aria-label="技术行情快照可视化">
    <header className="technical-visual-head">
      <div>
        <div className="eyebrow">Research-time market data · read only</div>
        <h3>技术行情快照 <span>单日 · 非实时</span></h3>
      </div>
      <span className="badge blue">{tech.status}</span>
    </header>
    <div className="technical-visual-cutoffs">
      <span>标的 <strong>{tech.symbol}</strong></span>
      <span>请求交易日 <strong>{dateLabel(tech.requested)}</strong></span>
      <span>有效交易日 <strong>{dateLabel(tech.effective)}</strong></span>
    </div>
    {tech.unavailable || !tech.candle ? <div className="technical-snapshot-unavailable" role="status">
      <strong>不可绘制单日行情</strong><p>{tech.unavailable ?? "研究工件缺少有效行情"}</p>
      <p>不展示替代价格、假 K 线或伪造技术指标。</p>
    </div> : <>
      <div className="technical-chart-grid-layout">
        <div className="technical-metric-panel technical-price-panel">
          <h4>原始单日 OHLC · 仅一个交易日</h4>
          <DailyCandle bar={tech.candle}/>
        </div>
        <div className="technical-metrics-stack">
          <FigureMetrics title="移动平均 · 独立技术指标" items={tech.movingAverages}/>
          <FigureMetrics title="动量指标" items={tech.momentum}/>
          {rsi && <div className="technical-rsi-gauge" aria-label={"RSI14 源数值 " + display(rsi.value)}>
            <div className="technical-rsi-header"><span>RSI14 · 0–100</span><strong>{display(rsi.value)}</strong></div>
            <div className="technical-rsi-track"><span style={{width:rsi.value+"%"}}/></div>
          </div>}
        </div>
      </div>
      <div className="technical-chart-grid-layout technical-bottom-metrics">
        <FigureMetrics title="成交量与量能比较 · 源单位" items={tech.volumes}/>
        <div className="technical-metrics-stack">
          <FigureMetrics title="历史收益率快照 · %" items={tech.returns}/>
          {ratio && <div className="technical-ratio-gauge" aria-label={"成交量相对 MA5 的比值 " + display(ratio.value)}>
            <div className="technical-rsi-header"><span>成交量 / 5 日均量</span><strong>{display(ratio.value)}×</strong></div>
            <div className="technical-ratio-track"><span style={{width: Math.min(ratio.value,3)/3*100+"%"}}/></div>
            <small>0–3× 刻度；超过 3× 的条形封顶，数字保留原值</small>
          </div>}
        </div>
      </div>
    </>}
    <p className="technical-source-note">本图表仅依据当前 Run 中的 Go TechnicalContext；OHLC 为原始单日价，MA/RSI/MACD 是后端计算的研究时点技术指标（TuShare 研究实现采用 PIT 前复权序列）。两者不混画为同口径历史价格趋势。当前工件未包含多日 OHLCV 序列、原始来源逐条时间戳或实时报价。</p>
  </section>;
}
