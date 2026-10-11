import { describe, expect, it } from "vitest";
import { filterOverviewRuns, runStateLabel, summarizeRunOverview } from "./run-overview";
import type { LiveRun, LiveState } from "./live-contract";

const now = new Date("2026-10-11T17:30:00Z");
function run(id: string, state: LiveState, date="2026-10-11T08:00:00Z", symbol="600519.SH"): LiveRun {
  return { run_id: id.repeat(32), symbol, mode: "PORTFOLIO_REVIEW", state,
    created_at: date, requested_as_of: date, ...(state === "FAILED" ? {failure_code:"RESEARCH_FAILED"} : {}) };
}
describe("Go Run overview returns a bounded and truthful sample", () => {
  it("counts each Run only once, sorts by Go created_at, and distinguishes all five states", () => {
    const a=run("a","COMPLETED","2026-10-09T10:00:00Z");
    const b=run("b","RUNNING");
    const c=run("c","PENDING");
    const d=run("d","FAILED","2026-10-10T12:00:00Z","000001.SZ");
    const e=run("e","CANCELLED","2026-10-05T09:00:00Z");
    const s = summarizeRunOverview([a,b,c,d,e,a],now);
    expect(s).toMatchObject({ total:5, active:2, completed:1, failed:1, cancelled:1, distinctSymbols:2, capped:false });
    expect(s.runs.map(r=>r.run_id)).toEqual([b.run_id,c.run_id,d.run_id,a.run_id,e.run_id]);
    expect(s.recentFailures).toEqual([d]);
    expect(s.latestCompleted?.run_id).toBe(a.run_id);
    expect(s.daily.reduce((acc,x)=>acc+x.count,0)).toBe(5);
    expect(s.daily.at(-1)).toEqual({utcDate:"2026-10-11",count:2});
    expect(s.daily[0]).toEqual({utcDate:"2026-10-05",count:1});
  });

  it("does not invent data for an empty or invalid API list", () => {
    const s=summarizeRunOverview([{},null,{run_id:"bad"}, { ...run("a","COMPLETED"),created_at:"bad"}],now);
    expect(s).toMatchObject({total:0,active:0,completed:0,failed:0,cancelled:0,distinctSymbols:0,latestCompleted:null});
    expect(s.daily).toHaveLength(7);
    expect(s.daily.every(x=>x.count===0)).toBe(true);
  });

  it("labels a list of 100 as capped; it cannot prove there are no older runs", () => {
    const list=Array.from({length:100}, (_,i)=>({
      ...run(i.toString(16).padStart(32,"0").slice(-1),"COMPLETED"),
      run_id:i.toString(16).padStart(32,"0")
    }));
    const s=summarizeRunOverview(list,now);
    expect(s.total).toBe(100);
    expect(s.capped).toBe(true);
  });

  it("filters status, case-insensitive symbol, run ID and combinations", () => {
    const a=run("a","COMPLETED"),b=run("b","FAILED","2026-10-10T00:00:00Z","000001.SZ"),
      c=run("c","RUNNING"),d=run("d","PENDING");
    const runs=summarizeRunOverview([a,b,c,d],now).runs;
    expect(filterOverviewRuns(runs,"active","").map(x=>x.state)).toEqual(["RUNNING","PENDING"]);
    expect(filterOverviewRuns(runs,"failed","000001").map(x=>x.run_id)).toEqual([b.run_id]);
    expect(filterOverviewRuns(runs,"all","600519")).toHaveLength(3);
    expect(filterOverviewRuns(runs,"completed","  AAAAA  ")).toEqual([a]);
    expect(filterOverviewRuns(runs,"failed","600519")).toEqual([]);
    expect(runStateLabel("FAILED")).toBe("失败");
  });

  it("uses UTC creation days and excludes older / future dates without fabricating history", () => {
    const s=summarizeRunOverview([
      run("1","COMPLETED","2026-10-04T23:59:59Z"),
      run("2","COMPLETED","2026-10-05T00:00:00Z"),
      run("3","COMPLETED","2026-10-12T00:00:00Z")
    ],now);
    expect(s.total).toBe(3);
    expect(s.daily.reduce((sum,x)=>sum+x.count,0)).toBe(1);
    expect(s.daily[0].count).toBe(1);
  });
});
