"use client";

import { useCallback, useEffect, useState } from "react";
import { CheckCircle2, Database, KeyRound, LockKeyhole, ShieldCheck, TriangleAlert } from "lucide-react";

type CredentialName = "DEEPSEEK_API_KEY" | "TUSHARE_TOKEN" | "TAVILY_API_KEY";
interface CredentialStatus { editable: boolean; configured: Record<CredentialName, boolean> }
const providers: { id: CredentialName; title: string; description: string }[] = [
  { id: "DEEPSEEK_API_KEY", title: "DeepSeek", description: "多 Agent 研究、论证和最终研究结论的模型密钥" },
  { id: "TUSHARE_TOKEN", title: "TuShare", description: "股票市场与技术指标、基本面研究数据源" },
  { id: "TAVILY_API_KEY", title: "Tavily Search", description: "事件研究的可选 Web Search 数据源" },
];

export default function ProviderSettings() {
  const [status, setStatus] = useState<CredentialStatus | null>(null);
  const [inputs, setInputs] = useState<Record<CredentialName, string>>({ DEEPSEEK_API_KEY: "", TUSHARE_TOKEN: "", TAVILY_API_KEY: "" });
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
      setInputs({ DEEPSEEK_API_KEY: "", TUSHARE_TOKEN: "", TAVILY_API_KEY: "" });
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
  return <div className="provider-settings">
    <section className="surface provider-settings-intro">
      <div className="eyebrow">Provider connections / Secure settings</div>
      <h2><KeyRound size={20}/> 模型与数据源凭证</h2>
      <p>凭证仅发送给当前登录的 Aegis Go 服务，由服务器加密保存。页面不会读取或回显密钥，浏览器刷新后输入框保持空白。</p>
      {status && <div className="provider-settings-overview"><ShieldCheck size={16}/>{status.editable ? "加密凭证存储已启用" : "只读状态：尚未配置 Go 服务端加密主密钥"}</div>}
    </section>
    <form className="surface provider-settings-form" onSubmit={e => void save(e)}>
      <div className="panel-head"><h2>连接凭证</h2><span className="badge blue">Server-side only</span></div>
      <div className="provider-settings-fields">
        {providers.map(provider => <div className="provider-settings-row" key={provider.id}>
          <div className="provider-settings-meta"><strong>{provider.title}</strong><small>{provider.description}</small><code>{provider.id}</code></div>
          <div className="provider-settings-entry">
            <span className={`badge ${status?.configured[provider.id] ? "green" : "gray"}`}>
              {status?.configured[provider.id] ? <CheckCircle2 size={12}/> : <LockKeyhole size={12}/>}
              {status ? (status.configured[provider.id] ? "已配置" : "未配置") : "检测中"}
            </span>
            <label htmlFor={`credential-${provider.id}`}>填写新密钥（留空不修改）</label>
            <input id={`credential-${provider.id}`} aria-label={`${provider.title} API Key`} type="password" autoComplete="off" spellCheck={false}
              disabled={saving || !status?.editable} value={inputs[provider.id]} maxLength={4096} minLength={8}
              onChange={e => setInputs(current => ({ ...current, [provider.id]: e.target.value }))} placeholder="••••••••••••••••" />
            {status?.editable && status.configured[provider.id] && <button className="provider-clear" type="button" disabled={saving} onClick={() => void clearCredential(provider.id)}>清除保存的覆盖值</button>}
          </div>
        </div>)}
      </div>
      <div className="provider-settings-actions">
        <button type="submit" className="btn primary" disabled={!status?.editable || saving}><LockKeyhole size={15}/>{saving ? "安全保存中…" : "保存凭证"}</button>
        <button type="button" className="btn" disabled={saving} onClick={() => void load().catch(err => setError((err as Error).message))}>刷新状态</button>
      </div>
      {error && <p role="alert" className="live-alert"><TriangleAlert size={16}/>{error}</p>}
      {message && <p role="status" className="live-info">{message}</p>}
    </form>
    <section className="surface provider-settings-help">
      <h3><Database size={17}/> 部署与安全边界</h3>
      <p>Go API Token、Web 访问口令、会话签名密钥和加密主密钥属于部署凭证，仍在服务端环境变量中设置。它们不会通过此页面修改。</p>
      <p>服务端加密文件依赖 <code>AEGIS_CREDENTIAL_MASTER_KEY</code>。如果不提供，该页只展示环境变量是否已配置。此页面仅面向单操作者的可信本地/内网部署。</p>
    </section>
  </div>;
}
