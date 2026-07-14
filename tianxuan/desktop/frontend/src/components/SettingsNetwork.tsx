import { useState } from "react";
import { Globe } from "lucide-react";
import type { SectionProps } from "./SettingsShared";
import { SettingsPageShell, SettingsSection, SettingsField, SegmentedButton } from "./SettingsPageShell";
import { app } from "../lib/bridge";

// SettingsNetwork configures HTTP proxy for outgoing model and tool requests.
export function SettingsNetwork({ s, busy: _busy, apply }: SectionProps) {
  const [mode, setMode] = useState(s.network?.proxyMode || "off");
  const [url, setUrl] = useState(s.network?.proxyUrl || "");
  const [noProxy, setNoProxy] = useState(s.network?.noProxy || "");

  const save = () => apply(() => app.SetNetwork(mode, url, noProxy));

  return (
    <SettingsPageShell title={<span className="flex items-center gap-1.5"><Globe size={15} />网络代理</span>} desc="配置 HTTP/HTTPS 代理，影响模型 API 和 web_fetch 等工具的出站流量。">
      <SettingsSection title="代理模式">
        <SettingsField label="代理模式" hint="选择出站流量的代理策略。">
          <SegmentedButton
            options={[
              { value: "off", label: "关闭" },
              { value: "auto", label: "系统代理" },
              { value: "env", label: "环境变量" },
              { value: "custom", label: "自定义" },
            ]}
            value={mode}
            onChange={setMode}
          />
        </SettingsField>

        {mode === "custom" && (
          <>
            <SettingsField label="代理 URL" hint="http://host:port 或 socks5://host:port">
              <input
                className="bg-bg-soft border border-border-soft rounded-md text-fg text-[12px] px-2.5 py-1.5 w-[260px] outline-none focus:border-accent transition-colors"
                placeholder="http://127.0.0.1:7890"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
              />
            </SettingsField>

            <SettingsField label="NoProxy" hint="逗号分隔的直连域名/主机，绕过代理">
              <input
                className="bg-bg-soft border border-border-soft rounded-md text-fg text-[12px] px-2.5 py-1.5 w-[260px] outline-none focus:border-accent transition-colors"
                placeholder="localhost,127.0.0.1,.local"
                value={noProxy}
                onChange={(e) => setNoProxy(e.target.value)}
              />
            </SettingsField>
          </>
        )}

        <div className="pt-2">
          <button
            className="px-4 py-1.5 text-[12px] rounded-md bg-accent text-white border-0 cursor-pointer hover:opacity-90 transition-opacity"
            onClick={save}
          >
            保存代理设置
          </button>
        </div>
      </SettingsSection>
    </SettingsPageShell>
  );
}
