import {describe,it,expect} from "vitest";
import {readHistoricalSeries} from "./historical-ohlcv";
import type {LiveRun} from "./live-contract";
const run:LiveRun={run_id:"a".repeat(32),mode:"PORTFOLIO_REVIEW",symbol:"600519.SH",requested_as_of:"2026-10-12T07:00:00Z",state:"COMPLETED",created_at:"2026-10-12T07:00:00Z",
 historical_ohlcv:{symbol:"600519.SH",requested_as_of:"20261012",effective_as_of:"20261009",price_basis:"RAW",bars:[
 {trade_date:"20261008",open:10,high:12,low:9,close:11,volume:0,amount:0},
 {trade_date:"20261009",open:11,high:13,low:10,close:12,volume:100,amount:1000}
 ]}};
describe("historical OHLCV same-run projection",()=>{
 it("preserves zero volume and source daily close",()=>{const x=readHistoricalSeries(run);expect(x?.bars[0].volume).toBe(0);expect(x?.bars[1].close).toBe(12)});
 it("rejects swapped symbols and invalid trading dates",()=>{const bad=structuredClone(run);const obs=bad.historical_ohlcv as Record<string,unknown>;obs.symbol="000001.SZ";expect(readHistoricalSeries(bad)).toBeNull();obs.symbol="600519.SH";const bars=obs.bars as Array<Record<string,unknown>>;bars[1].trade_date="20261007";expect(readHistoricalSeries(bad)).toBeNull()});
 it("rejects future effective dates, adjusted sources, and failed runs",()=>{const bad=structuredClone(run);const obs=bad.historical_ohlcv as Record<string,unknown>;obs.effective_as_of="20261013";expect(readHistoricalSeries(bad)).toBeNull();obs.effective_as_of="20261009";obs.price_basis="PIT_QFQ";expect(readHistoricalSeries(bad)).toBeNull();expect(readHistoricalSeries({...run,state:"FAILED"})).toBeNull()});
 it("rejects missing series rather than inventing prices",()=>expect(readHistoricalSeries({...run,historical_ohlcv:undefined})).toBeNull());
});
