/** Deterministic synthetic fixtures for the UI prototype. Never use for real research. */
export type StockId = "600519.SH" | "000001.SZ" | "300750.SZ";

export interface Evidence {
  id: string;
  title: string;
  category: string;
  note: string;
  source: string;
  asOf: string;
  kind: "support" | "risk" | "neutral";
}

export interface StockFixture {
  symbol: StockId;
  name: string;
  sector: string;
  price: number;
  change: number;
  volumes: number[];
  closes: number[];
  researchSummary: string;
  bull: string;
  bear: string;
  risk: string;
  score: number;
  evidence: Evidence[];
}

const demoDate = "2026-09-24";

export const demoStocks: StockFixture[] = [
  {
    symbol: "600519.SH", name: "贵州茅台", sector: "食品饮料 · 白酒", price: 1512.80, change: 1.84, score: 72,
    closes: [1438,1448,1445,1456,1460,1454,1463,1472,1466,1469,1484,1481,1492,1486,1498,1495,1506,1499,1509,1513,1508,1515,1509,1519,1512.8],
    volumes: [29,28,34,24,26,30,31,27,33,32,39,37,31,34,42,38,40,37,44,39,38,41,37,43,36],
    researchSummary: "结构维持偏强，但短线量价背离值得关注。建议重点追踪支撑有效性，并等待新增证据确认。",
    bull: "中期趋势上行，回踩后结构尚未破坏；相对稳定的现金流为研究提供支撑。",
    bear: "短期反弹的量能持续性不足；若关键支撑失效，原有趋势假设需要重新评估。",
    risk: "量价背离与板块轮动可能造成短期回撤；账户信息尚未提供，不能计算组合仓位。",
    evidence: [
      {id:"EV-001",title:"价格维持在中期观察区间上方",category:"技术",note:"演示趋势样本，非真实 MA20 结果。用于展示研究观点到来源的关系。",source:"DEMO · synthetic-ohlcv",asOf:demoDate,kind:"support"},
      {id:"EV-002",title:"短期交易量增长不足",category:"风险",note:"演示成交量序列；真实应用应绑定交易所数据、时间戳和实际计算结果。",source:"DEMO · synthetic-volume",asOf:demoDate,kind:"risk"},
      {id:"EV-003",title:"公告与基本面信息尚待核验",category:"事件",note:"本条专门用于展示未证实证据；不代表存在任何真实公告。",source:"DEMO · no external provider",asOf:demoDate,kind:"neutral"},
    ],
  },
  {
    symbol:"000001.SZ",name:"平安银行",sector:"银行 · 股份制银行",price:11.42,change:-0.63,score:61,
    closes:[10.96,10.99,11.03,11.06,11.11,11.05,11.10,11.07,11.14,11.21,11.18,11.25,11.29,11.21,11.27,11.35,11.30,11.38,11.31,11.40,11.37,11.46,11.43,11.49,11.42],
    volumes:[27,26,28,31,28,37,25,24,38,35,29,40,39,42,27,32,35,40,37,34,41,30,36,38,33],
    researchSummary:"价格仍在观察区域内震荡，趋势强度有限。研究重点转向风险偏好与相对估值变化。",
    bull:"防御属性及估值修复假设有一定吸引力，但需要更充分的业绩证据。",
    bear:"波动与量能不能独立支持强趋势判断，市场风格切换仍是主要变量。",
    risk:"当前演示数据不足以支持组合决策；需实际账户快照与风险政策。",
    evidence:[
      {id:"EV-011",title:"价格波动处于样例震荡带",category:"技术",note:"完全模拟的价格区间，供交互展示。",source:"DEMO · synthetic-ohlcv",asOf:demoDate,kind:"neutral"},
      {id:"EV-012",title:"成交活跃度缺乏趋势确认",category:"风险",note:"演示走势，不是真实市场信号。",source:"DEMO · synthetic-volume",asOf:demoDate,kind:"risk"},
      {id:"EV-013",title:"行业基本面来源未连接",category:"基本面",note:"前端尚未接入财报 Provider，因此不能提供确定性基本面观点。",source:"DEMO · no provider",asOf:demoDate,kind:"neutral"},
    ],
  },
  {
    symbol:"300750.SZ",name:"宁德时代",sector:"电力设备 · 电池",price:284.60,change:2.36,score:78,
    closes:[255,258,257,261,259,263,266,265,269,272,271,269,273,275,274,278,276,279,283,280,282,285,281,286,284.6],
    volumes:[52,56,49,54,59,57,62,53,60,66,59,58,61,70,64,62,69,74,64,68,72,70,75,73,71],
    researchSummary:"示例趋势较为积极，但波动率提高。研究应兼顾成长论点和事件驱动风险。",
    bull:"演示数据呈阶梯式上行结构；若行业信息验证有效，可能增强趋势假设。",
    bear:"价格波动率上升，业绩敏感性和行业变化可能导致观点快速失效。",
    risk:"主题轮动和高波动将影响假设稳定性；演示样本不可用于真实投资判断。",
    evidence:[
      {id:"EV-021",title:"演示价格结构保持抬高",category:"技术",note:"仅用于前端可视化，不对应真实交易日。",source:"DEMO · synthetic-ohlcv",asOf:demoDate,kind:"support"},
      {id:"EV-022",title:"较高的演示波动率",category:"风险",note:"需对接真实历史数据才能定量评估风险。",source:"DEMO · synthetic-price",asOf:demoDate,kind:"risk"},
      {id:"EV-023",title:"行业信息源尚未接入",category:"事件",note:"无可引用的正式公告或新闻。",source:"DEMO · no provider",asOf:demoDate,kind:"neutral"},
    ],
  },
];

export interface AgentStage {
  id: string;
  short: string;
  name: string;
  title: string;
  duration: string;
  detail: string;
}

export const stages: AgentStage[] = [
  {id:"market",short:"MK",name:"Market Analyst",title:"市场环境研究",duration:"1.8s",detail:"评估市场风险偏好、指数与交易环境。本页结果为模拟。"},
  {id:"technical",short:"TA",name:"Technical Analyst",title:"技术结构分析",duration:"2.4s",detail:"处理演示 OHLCV、趋势、成交量以及重要价位。"},
  {id:"event",short:"EV",name:"Event Analyst",title:"公告与事件核验",duration:"3.2s",detail:"展示事件研究阶段。当前没有连接实际公告和新闻 API。"},
  {id:"fundamental",short:"FA",name:"Fundamental Analyst",title:"基本面研究",duration:"2.1s",detail:"演示基本面角色。真实后端可以有条件启用该角色。"},
  {id:"debate",short:"DB",name:"Bull / Bear Debate",title:"多空研究辩论",duration:"4.6s",detail:"比较支持与反对论点，并标记尚未解决的证据缺口。"},
  {id:"manager",short:"RM",name:"Research Manager",title:"研究观点整合",duration:"1.5s",detail:"在结构化证据上形成结论与研究计划，不代表真实订单。"},
  {id:"trading",short:"TI",name:"Trading Agent",title:"研究意图生成",duration:"1.7s",detail:"生成研究性 TradeIntent，不连接券商或交易权限。"},
  {id:"risk",short:"RK",name:"Risk / Portfolio",title:"风险与组合审查",duration:"2.7s",detail:"账户状态 NOT_PROVIDED，不能推断零仓位或目标仓位。"},
];

export const demoHistory = [
  {id:"DEMO-20260924-001",symbol:"600519.SH" as StockId,when:"2026-09-24 15:42",state:"COMPLETED",headline:"趋势待确认 · 保持观察",duration:"19.6s"},
  {id:"DEMO-20260923-002",symbol:"000001.SZ" as StockId,when:"2026-09-23 16:18",state:"COMPLETED",headline:"横盘整理 · 关注风险",duration:"17.2s"},
  {id:"DEMO-20260922-003",symbol:"300750.SZ" as StockId,when:"2026-09-22 15:28",state:"COMPLETED",headline:"波动上行 · 留意事件",duration:"20.4s"},
  {id:"DEMO-20260921-004",symbol:"600519.SH" as StockId,when:"2026-09-21 16:01",state:"FAILED",headline:"演示来源不可用",duration:"8.9s"},
];

export const findStock = (symbol: StockId) => demoStocks.find(s=>s.symbol===symbol) ?? demoStocks[0];
