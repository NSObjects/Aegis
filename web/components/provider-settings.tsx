"use client";

import { useCallback, useEffect, useState } from "react";
import { BrainCircuit, ChartNoAxesCombined, CheckCircle2, Database, Globe2, KeyRound, LockKeyhole, ShieldCheck, TriangleAlert } from "lucide-react";

type CredentialName = "DEEPSEEK_API_KEY" | "TUSHARE_TOKEN" | "TAVILY_API_KEY";
interface CredentialStatus { editable: boolean; configured: Record<CredentialName, boolean> }
interface ProviderDefinition { id: CredentialName; title: string; description: string; inputLabel: string; optional?: boolean }
const providerGroups: { id: string; title: string; subtitle: string; icon: typeof KeyRound; providers: ProviderDefinition[] }[] = [
  {
    id: "model", title: "AI 模型", subtitle: "研究推理、Agent 分析与结论生成", icon: BrainCircuit,
    providers: [{ id: "DEEPSEEK_API_KEY", title: "DeepSeek", description: "多 Agent 研究、辩论与最终结论所使用的模型服务", inputLabel: "DeepSeek API Key" }],
  },
  {
    id: "market", title: "金融数据", subtitle: "股票行情、技术指标和财务数据", icon: ChartNoAxesCombined,
    providers: [{ id: "TUSHARE_TOKEN", title: "TuShare", description: "A 股行情、技术指标及基本面研究数据", inputLabel: "TuShare Token" }],
  },
  {
    id: "search", title: "资讯搜索", subtitle: "事件研究与外部信息检索", icon: Globe2,
    providers: [{ id: "TAVILY_API_KEY", title: "Tavily Search", description: "新闻及事件研究的可选 Web Search 服务", inputLabel: "Tavily API Key", optional: true }],
  },
];
const blankInputs: Record<CredentialName, string> = { DEEPSEEK_API_KEY: "", TUSHARE_TOKEN: "", TAVILY_API_KEY: "" };

export default function ProviderSettings({ localDev = false }: { localDev?: boolean }) {
  const [status, setStatus] = useState<CredentialStatus | null>(null);
  const [inputs, setInputs] = useState<Record<CredentialName, string>>(blankInputs);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const load = useCallback(async () => {
    const response = await fetch("/api/workbench/credentials", { credentials: "same-origin", cache: "no-store" });
    if (!response.ok) throw Error(`无法读取凭证状态（HTTP ${response.status}）`);
    const data = await response.json() as CredentialStatus;
    setStatus(data);
  }, []);
  useEffect(() => {
    let active = true;
    void (async () => {
      try {
        const response = await fetch("/api/workbench/credentials", { credentials: "same-origin", cache: "no-store" });
        if (!response.ok) throw Error(`无法读取凭证状态（HTTP ${response.status}）`);
        const data = await response.json() as CredentialStatus;
        if (active) setStatus(data);
      } catch (cause) { if (active) setError((cause as Error).message); }
    })();
    return () => { active = false; };
  }, []);
  async function update(changes: Partial<Record<CredentialName, string>>, clear: CredentialName[] = []) {
    setSaving(true); setError(""); setMessage("");
    try {
      const response = await fetch("/api/workbench/credentials", {
        method: "PUT", credentials: "same-origin", cache: "no-store",
        headers: { "Content-Type": "application/json" }, body: JSON.stringify({ changes, clear }),
      });
      if (!response.ok) throw Error(response.status === 409 ? "服务端未启用加密存储，请先配置 AEGIS_CREDENTIAL_MASTER_KEY" : `保存失败（HTTP ${response.status}）`);
      setInputs(current => Object.fromEntries(Object.entries(current).map(([name, value]) => [name, name in changes || clear.includes(name as CredentialName) ? "" : value])) as Record<CredentialName, string>);
      setStatus(await response.json() as CredentialStatus);
      setMessage("已保存至 Go 服务端，新建研究任务将使用最新凭证。正在运行的任务保持原配置。");
    } catch (cause) { setError((cause as Error).message); } finally { setSaving(false); }
  }
  async function save(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const changes = Object.fromEntries(Object.entries(inputs).filter(([, value]) => value.length > 0)) as Partial<Record<CredentialName, string>>;
    if (Object.keys(changes).length === 0) { setError("请先输入至少一个新凭证"); return; }
    await update(changes);
  }
  async function clearCredential(name: CredentialName) {
    if (!window.confirm(`清除 ${name} 的加密覆盖值？若服务器环境变量仍存在，将继续使用环境变量。`)) return;
    await update({}, [name]);
  }
  const count = status ? providerGroups.reduce((sum, group) => sum + group.providers.filter(provider => status.configured[provider.id]).length, 0) : 0;
  const total = providerGroups.reduce((sum, group) => sum + group.providers.length, 0);
  return <div className="provider-settings">
    <section className="surface provider-settings-intro">
      <div className="eyebrow">Integrations / Provider credentials</div>
      <h2><KeyRound size={20}/> 模型与数据源凭证</h2>
      <p>第三方服务按用途分组管理。新密钥只发送给 Aegis Go 服务端加密保存；浏览器仅显示配置状态，不读取或回显已保存的密钥。</p>
      {status && <div className="provider-settings-overview"><ShieldCheck size={16}/><span>{status.editable ? "服务端加密存储已启用" : "只读状态：尚未配置 Go 服务端加密主密钥"}</span><strong>{count} / {total} 已配置</strong></div>}
    </section>
    <form className="surface provider-settings-form" onSubmit={e => void save(e)}>
      <div className="panel-head"><h2>第三方服务配置</h2><span className="badge blue">Server-side only</span></div>
      <div className="provider-settings-groups">
        {providerGroups.map(group => {
          const GroupIcon = group.icon;
          const configured = status ? group.providers.filter(provider => status.configured[provider.id]).length : 0;
          return <section className="provider-settings-group" aria-labelledby={"provider-group-" + group.id} key={group.id}>
            <header className="provider-settings-group-header">
              <span className="provider-settings-group-icon"><GroupIcon size={19}/></span>
              <div className="provider-settings-group-title">
                <h3 id={"provider-group-" + group.id}>{group.title}</h3>
                <p>{group.subtitle}</p>
              </div>
              <span className="provider-settings-group-count">{status ? configured + " / " + group.providers.length + " 已配置" : "检测中"}</span>
            </header>
            <div className="provider-settings-fields">
              {group.providers.map(provider => <div className="provider-settings-row" key={provider.id}>
                <div className="provider-settings-meta">
                  <div className="provider-settings-name"><strong>{provider.title}</strong>{provider.optional && <span>可选</span>}</div>
                  <small>{provider.description}</small>
                  <code>{provider.id}</code>
                </div>
                <div className="provider-settings-entry">
                  <span className={"badge " + (status?.configured[provider.id] ? "green" : "gray")}>
                    {status?.configured[provider.id] ? <CheckCircle2 size={12}/> : <LockKeyhole size={12}/>}
                    {status ? (status.configured[provider.id] ? "已配置" : "未配置") : "检测中"}
                  </span>
                  <label htmlFor={"credential-" + provider.id}>填写新密钥（留空不修改）</label>
                  <input id={"credential-" + provider.id} aria-label={provider.inputLabel} type="password" autoComplete="off" spellCheck={false}
                    disabled={saving || !status?.editable} value={inputs[provider.id]} maxLength={4096} minLength={8}
                    onChange={e => setInputs(current => ({ ...current, [provider.id]: e.target.value }))} placeholder="••••••••••••••••" />
                  {status?.editable && status.configured[provider.id] && <button className="provider-clear" type="button" disabled={saving} onClick={() => void clearCredential(provider.id)}>清除保存的覆盖值</button>}
                </div>
              </div>)}
            </div>
          </section>;
        })}
      </div>
      <div className="provider-settings-actions">
        <button type="submit" className="btn primary" disabled={!status?.editable || saving}><LockKeyhole size={15}/>{saving ? "安全保存中…" : "保存凭证"}</button>
        <button type="button" className="btn" disabled={saving} onClick={() => void load().then(() => setError("")).catch(err => setError((err as Error).message))}>刷新状态</button>
        <span className="provider-settings-action-note">仅修改已填写的密钥，其他分组保持不变</span>
      </div>
      {error && <p role="alert" className="live-alert"><TriangleAlert size={16}/>{error}</p>}
      {message && <p role="status" className="live-info">{message}</p>}
    </form>
    <section className="surface provider-settings-help">
      <h3><Database size={17}/> 部署与安全边界</h3>
      <p>Go API Token、Web 访问口令、会话签名密钥和加密主密钥属于部署凭证，仍在服务端环境变量中设置。它们不会通过此页面修改。</p>
      <p>{localDev ? "当前为本地调试：Go 已自动生成并保存本地加密主密钥，无需设置环境变量。请勿把此模式暴露到网络。" : <>服务端加密文件依赖 <code>AEGIS_CREDENTIAL_MASTER_KEY</code>。如果不提供，该页只展示环境变量是否已配置。此页面仅面向单操作者的可信本地/内网部署。</>}</p>
    </section>
  </div>;
}
