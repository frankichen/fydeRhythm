import { useEffect, useState, type CSSProperties } from "react";
import {
  kDefaultAiSettings,
  loadAiSettings,
  normalizeAiSettings,
  saveAiSettings,
  type AiSettings,
} from "@/lib/ai-settings";
import { rankCandidatesWithDeepSeek } from "@/lib/deepseek-ranker";

const fieldStyle: CSSProperties = {
  width: "100%",
  boxSizing: "border-box",
  marginTop: 5,
  padding: "8px 10px",
  border: "1px solid #c7c7c7",
  borderRadius: 8,
  fontSize: 13,
};

export default function App() {
  const [settings, setSettings] = useState<AiSettings>(kDefaultAiSettings);
  const [status, setStatus] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    void loadAiSettings().then(setSettings).catch((error) => setStatus(`读取失败：${String(error)}`));
  }, []);

  function update<K extends keyof AiSettings>(key: K, value: AiSettings[K]) {
    setSettings((current) => normalizeAiSettings({ ...current, [key]: value }));
    setStatus("设置尚未保存");
  }

  async function save() {
    setBusy(true);
    try {
      await saveAiSettings(settings);
      setStatus(settings.enabled ? "已保存，AI 候选排序已开启" : "已保存，AI 候选排序已关闭");
    } catch (error) {
      setStatus(`保存失败：${String(error)}`);
    } finally {
      setBusy(false);
    }
  }

  async function testConnection() {
    setBusy(true);
    setStatus("正在测试 DeepSeek……");
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), settings.timeoutMs);
    try {
      const order = await rankCandidatesWithDeepSeek(
        { ...settings, enabled: true },
        "这个功能修改完成后，我们先进行",
        "cs",
        ["测试", "尝试", "超时", "重试"],
        controller.signal,
      );
      setStatus(order ? `连接成功，排序：${order.join(" → ")}` : "接口已响应，但没有返回有效排序");
    } catch (error) {
      setStatus(`连接失败：${error instanceof Error ? error.message : String(error)}`);
    } finally {
      clearTimeout(timer);
      setBusy(false);
    }
  }

  return (
    <main style={{ width: 370, padding: 16, fontFamily: "system-ui, sans-serif", color: "#202124" }}>
      <h2 style={{ margin: "0 0 6px", fontSize: 18 }}>DeepSeek 候选优化</h2>
      <p style={{ margin: "0 0 14px", fontSize: 12, lineHeight: 1.55, color: "#5f6368" }}>
        Rime 候选先立即显示，DeepSeek 只在后台根据短前文重新排序。密码、网址、邮箱、电话、数字和禁止学习的输入框不会发送请求。
      </p>

      <label style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 14 }}>
        <input type="checkbox" checked={settings.enabled} onChange={(event) => update("enabled", event.target.checked)} />
        <strong>启用 AI 候选重排</strong>
      </label>

      <label style={{ display: "block", marginBottom: 12, fontSize: 12 }}>
        DeepSeek API Key（仅保存在本机扩展存储）
        <input
          type="password"
          value={settings.apiKey}
          placeholder="sk-..."
          onChange={(event) => update("apiKey", event.target.value)}
          style={fieldStyle}
        />
      </label>

      <label style={{ display: "block", marginBottom: 12, fontSize: 12 }}>
        模型
        <select value={settings.model} onChange={(event) => update("model", event.target.value as AiSettings["model"])} style={fieldStyle}>
          <option value="deepseek-v4-flash">deepseek-v4-flash（推荐）</option>
          <option value="deepseek-v4-pro">deepseek-v4-pro</option>
        </select>
      </label>

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10, marginBottom: 12 }}>
        <label style={{ fontSize: 12 }}>
          停止输入等待（ms）
          <input type="number" min={100} max={1000} value={settings.debounceMs} onChange={(event) => update("debounceMs", Number(event.target.value))} style={fieldStyle} />
        </label>
        <label style={{ fontSize: 12 }}>
          超时（ms）
          <input type="number" min={250} max={3000} value={settings.timeoutMs} onChange={(event) => update("timeoutMs", Number(event.target.value))} style={fieldStyle} />
        </label>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10, marginBottom: 14 }}>
        <label style={{ fontSize: 12 }}>
          前文字符数
          <input type="number" min={0} max={100} value={settings.contextChars} onChange={(event) => update("contextChars", Number(event.target.value))} style={fieldStyle} />
        </label>
        <label style={{ fontSize: 12 }}>
          候选数量
          <input type="number" min={2} max={9} value={settings.candidateLimit} onChange={(event) => update("candidateLimit", Number(event.target.value))} style={fieldStyle} />
        </label>
      </div>

      <div style={{ display: "flex", gap: 8 }}>
        <button onClick={() => void save()} disabled={busy} style={{ flex: 1, padding: "10px", border: 0, borderRadius: 8, background: "#1a73e8", color: "white", fontWeight: 600 }}>
          保存设置
        </button>
        <button onClick={() => void testConnection()} disabled={busy || !settings.apiKey} style={{ flex: 1, padding: "10px", border: "1px solid #1a73e8", borderRadius: 8, background: "white", color: "#1a73e8", fontWeight: 600 }}>
          测试连接
        </button>
      </div>

      {status && <p style={{ margin: "10px 0 0", fontSize: 12, lineHeight: 1.5, color: status.includes("失败") ? "#b3261e" : "#5f6368" }}>{status}</p>}
    </main>
  );
}
