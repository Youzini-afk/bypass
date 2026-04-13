import { FormEvent, useEffect, useMemo, useState } from "react";
import {
  BingAccount,
  BootstrapResponse,
  ConfigResponse,
  CozeAccount,
  ModelsResponse,
  PlaygroundResponse,
  RuntimeConfig,
  SecretAccount,
  StorageStatus,
} from "./types";

type TabKey = "overview" | "providers" | "windsurf" | "playground";
type Flash = { kind: "success" | "error" | "info"; text: string } | null;
type RequestResult<T> = { ok: boolean; status: number; data?: T; error?: string };
type ProviderTestDraft = { model: string; accountId: string; prompt: string };
type ProviderTestResult = { pending: boolean; error?: string; response?: PlaygroundResponse };
type WindsurfAction = "validate" | "jwt" | "smoke";

const TAB_ITEMS: Array<{ key: TabKey; label: string; subtitle: string }> = [
  { key: "overview", label: "概览", subtitle: "状态与存储" },
  { key: "providers", label: "Provider 配置", subtitle: "统一配置台" },
  { key: "windsurf", label: "Windsurf 专区", subtitle: "账号与模型映射" },
  { key: "playground", label: "调试 / 测试台", subtitle: "即时验证" },
];

const PROVIDER_ORDER = [
  "windsurf",
  "cursor",
  "deepseek",
  "qodo",
  "lmsys",
  "blackbox",
  "you",
  "grok",
  "bing",
  "coze",
] as const;

type ProviderName = (typeof PROVIDER_ORDER)[number];
type ModelProvider = "cursor" | "deepseek" | "qodo" | "lmsys" | "blackbox" | "you" | "grok";

const PROVIDER_LABELS: Record<string, string> = {
  windsurf: "Windsurf",
  cursor: "Cursor",
  deepseek: "DeepSeek",
  qodo: "Qodo",
  lmsys: "LMSYS",
  blackbox: "Blackbox",
  you: "You",
  grok: "Grok",
  bing: "Bing",
  coze: "Coze",
  "lmsys-chat": "LMSYS Chat",
};

const PROVIDER_NOTES: Record<string, string> = {
  cursor: "支持配置模型、checksum 与后台测试专用 token。",
  deepseek: "模型集保持兼容接口原样，后台可保存测试 token。",
  qodo: "通用 key + 模型配置；测试会直接走当前保存配置。",
  lmsys: "可写入 LMSYS 相关 token/config 字符串并测试标准模型。",
  blackbox: "Validated token 保存在服务端，支持快速联通性验证。",
  you: "Cookie 池支持热更新，可选开启定时任务保活。",
  grok: "Cookie 池热更新，当前模型固定为 grok-2 / grok-3。",
  bing: "账号池包含 ScopeID / IDToken / Cookie，保存后可立即重载。",
  coze: "支持 WebSDK 账号池、bot/model/system 参数与后台重建。",
};

const PLAYGROUND_FALLBACK_PROMPT = "Reply with READY.";

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

function createId(prefix: string) {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return `${prefix}-${crypto.randomUUID()}`;
  }
  return `${prefix}-${Math.random().toString(36).slice(2, 12)}`;
}

function appError(result: RequestResult<unknown>, fallback: string) {
  return result.error || fallback;
}

function accountHint(hasSecret?: boolean, suffix?: string) {
  if (!hasSecret) {
    return "未保存密钥。";
  }
  return suffix ? `已保存，后四位 ${suffix}，留空则保持。` : "已保存，留空则保持。";
}

function findDefaultAccount(accounts: Array<{ id: string; default: boolean }>) {
  return accounts.find((item) => item.default)?.id || accounts[0]?.id || "";
}

function normalizeSecretField<T extends SecretAccount>(account: T, secret: string): T {
  const next = { ...account, secret };
  if (secret.trim()) {
    next.keepSecret = false;
    next.hasSecret = false;
    next.secretSuffix = "";
    return next;
  }
  next.keepSecret = !!account.hasSecret;
  return next;
}

function normalizeMaskedValue<T extends Record<string, unknown>>(
  item: T,
  field: keyof T,
  keepField: keyof T,
  hasField: keyof T,
  suffixField: keyof T,
  value: string,
) {
  const next = { ...item, [field]: value } as T;
  if (value.trim()) {
    next[keepField] = false as T[keyof T];
    next[hasField] = false as T[keyof T];
    next[suffixField] = "" as T[keyof T];
    return next;
  }
  next[keepField] = Boolean(item[hasField]) as T[keyof T];
  return next;
}

function ensureSingleDefault<T extends { default: boolean }>(items: T[], index: number) {
  return items.map((item, current) => ({ ...item, default: current === index }));
}

function providerEnabled(config: RuntimeConfig | null, provider: ProviderName) {
  if (!config) {
    return false;
  }
  if (provider === "windsurf") {
    return config.windsurf.enabled;
  }
  return config.providers[provider].enabled;
}

function providerAccounts(config: RuntimeConfig | null, provider: ProviderName): SecretAccount[] {
  if (!config) {
    return [];
  }
  switch (provider) {
    case "windsurf":
      return config.windsurf.accounts;
    case "cursor":
      return config.providers.cursor.accounts;
    case "deepseek":
      return config.providers.deepseek.accounts;
    case "lmsys":
      return config.providers.lmsys.accounts;
    case "you":
      return config.providers.you.accounts;
    case "grok":
      return config.providers.grok.accounts;
    default:
      return [];
  }
}

function providerModels(models: Record<string, string[]>, provider: string) {
  return models[provider] || [];
}

function prettyProvider(name: string) {
  return PROVIDER_LABELS[name] || name;
}

function prettyPath(path: string) {
  return path || "未检测到";
}

function prettyStorage(status?: StorageStatus) {
  if (!status) {
    return "未检测";
  }
  if (!status.writable) {
    return `不可写: ${prettyPath(status.path)}`;
  }
  if (status.exists) {
    return `已持久化: ${prettyPath(status.path)}`;
  }
  return `待首次保存创建: ${prettyPath(status.path)}`;
}

function badgeClass(enabled: boolean) {
  return enabled ? "status-badge status-badge--ok" : "status-badge status-badge--muted";
}

function SectionTitle(props: { eyebrow?: string; title: string; description?: string; actions?: JSX.Element | null }) {
  const { eyebrow, title, description, actions } = props;
  return (
    <div className="section-title">
      <div>
        {eyebrow ? <div className="section-title__eyebrow">{eyebrow}</div> : null}
        <h2>{title}</h2>
        {description ? <p>{description}</p> : null}
      </div>
      {actions ? <div className="section-title__actions">{actions}</div> : null}
    </div>
  );
}

function Field(props: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  hint?: string;
  type?: string;
  textarea?: boolean;
  rows?: number;
  monospace?: boolean;
}) {
  const { label, value, onChange, placeholder, hint, type = "text", textarea, rows = 4, monospace } = props;
  return (
    <label className="field">
      <span>{label}</span>
      {textarea ? (
        <textarea
          rows={rows}
          value={value}
          placeholder={placeholder}
          onChange={(event) => onChange(event.target.value)}
          className={monospace ? "is-monospace" : undefined}
        />
      ) : (
        <input
          type={type}
          value={value}
          placeholder={placeholder}
          onChange={(event) => onChange(event.target.value)}
          className={monospace ? "is-monospace" : undefined}
        />
      )}
      {hint ? <small>{hint}</small> : null}
    </label>
  );
}

function ToggleField(props: { label: string; checked: boolean; onChange: (value: boolean) => void; hint?: string }) {
  const { label, checked, onChange, hint } = props;
  return (
    <label className="toggle-field">
      <span className="toggle-field__copy">
        <strong>{label}</strong>
        {hint ? <small>{hint}</small> : null}
      </span>
      <button
        type="button"
        className={`switch ${checked ? "switch--on" : ""}`}
        onClick={() => onChange(!checked)}
        aria-pressed={checked}
      >
        <span />
      </button>
    </label>
  );
}

function StringListEditor(props: {
  label: string;
  items: string[];
  onChange: (items: string[]) => void;
  placeholder?: string;
  hint?: string;
}) {
  const { label, items, onChange, placeholder, hint } = props;
  const [draft, setDraft] = useState("");

  function addValue() {
    const value = draft.trim();
    if (!value) {
      return;
    }
    if (!items.includes(value)) {
      onChange([...items, value]);
    }
    setDraft("");
  }

  return (
    <div className="field">
      <span>{label}</span>
      <div className="list-editor">
        <div className="list-editor__input">
          <input
            value={draft}
            placeholder={placeholder}
            onChange={(event) => setDraft(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                addValue();
              }
            }}
          />
          <button type="button" className="ghost-button" onClick={addValue}>
            添加
          </button>
        </div>
        {items.length ? (
          <div className="chip-row">
            {items.map((item) => (
              <span key={item} className="chip">
                {item}
                <button type="button" onClick={() => onChange(items.filter((current) => current !== item))}>
                  ×
                </button>
              </span>
            ))}
          </div>
        ) : (
          <div className="empty-inline">还没有自定义条目</div>
        )}
      </div>
      {hint ? <small>{hint}</small> : null}
    </div>
  );
}

function SecretAccountsEditor(props: {
  title: string;
  description: string;
  accounts: SecretAccount[];
  secretLabel: string;
  secretPlaceholder?: string;
  addLabel: string;
  onChange: (accounts: SecretAccount[]) => void;
}) {
  const { title, description, accounts, secretLabel, secretPlaceholder, addLabel, onChange } = props;

  function update(index: number, patch: Partial<SecretAccount>) {
    onChange(accounts.map((item, current) => (current === index ? { ...item, ...patch } : item)));
  }

  return (
    <div className="editor-card">
      <div className="editor-card__header">
        <div>
          <h4>{title}</h4>
          <p>{description}</p>
        </div>
        <button
          type="button"
          className="ghost-button"
          onClick={() =>
            onChange([
              ...accounts,
              {
                id: createId("account"),
                name: `${addLabel} ${accounts.length + 1}`,
                secret: "",
                note: "",
                default: accounts.length === 0,
                keepSecret: false,
              },
            ])
          }
        >
          新增
        </button>
      </div>
      <div className="stack-list">
        {accounts.length ? (
          accounts.map((account, index) => (
            <div key={account.id} className="account-card">
              <div className="account-card__toolbar">
                <button type="button" className="pill-button" onClick={() => onChange(ensureSingleDefault(accounts, index))}>
                  {account.default ? "默认账号" : "设为默认"}
                </button>
                <button type="button" className="pill-button pill-button--danger" onClick={() => onChange(accounts.filter((item) => item.id !== account.id))}>
                  删除
                </button>
              </div>
              <div className="grid cols-2">
                <Field label="别名" value={account.name} onChange={(value) => update(index, { name: value })} placeholder={`${addLabel} ${index + 1}`} />
                <Field label="备注" value={account.note || ""} onChange={(value) => update(index, { note: value })} placeholder="用途、来源、归属人" />
                <Field
                  label={secretLabel}
                  value={account.secret || ""}
                  type="password"
                  placeholder={secretPlaceholder || "留空保持当前值"}
                  onChange={(value) => update(index, normalizeSecretField(account, value))}
                  hint={accountHint(account.hasSecret, account.secretSuffix)}
                />
              </div>
            </div>
          ))
        ) : (
          <div className="empty-state small">还没有可用账号。</div>
        )}
      </div>
    </div>
  );
}

function BingAccountsEditor(props: { accounts: BingAccount[]; onChange: (accounts: BingAccount[]) => void }) {
  const { accounts, onChange } = props;

  function update(index: number, patch: Partial<BingAccount>) {
    onChange(accounts.map((item, current) => (current === index ? { ...item, ...patch } : item)));
  }

  return (
    <div className="editor-card">
      <div className="editor-card__header">
        <div>
          <h4>Bing 账号池</h4>
          <p>ScopeID、IDToken、Cookie 保存后会立即参与池化与轮询。</p>
        </div>
        <button
          type="button"
          className="ghost-button"
          onClick={() =>
            onChange([
              ...accounts,
              {
                id: createId("bing"),
                name: `Bing ${accounts.length + 1}`,
                scopeId: "",
                idToken: "",
                cookie: "",
                note: "",
                default: accounts.length === 0,
              },
            ])
          }
        >
          新增
        </button>
      </div>
      <div className="stack-list">
        {accounts.length ? (
          accounts.map((account, index) => (
            <div key={account.id} className="account-card">
              <div className="account-card__toolbar">
                <button type="button" className="pill-button" onClick={() => onChange(ensureSingleDefault(accounts, index))}>
                  {account.default ? "默认账号" : "设为默认"}
                </button>
                <button type="button" className="pill-button pill-button--danger" onClick={() => onChange(accounts.filter((item) => item.id !== account.id))}>
                  删除
                </button>
              </div>
              <div className="grid cols-2">
                <Field label="别名" value={account.name} onChange={(value) => update(index, { name: value })} placeholder={`Bing ${index + 1}`} />
                <Field label="ScopeID" value={account.scopeId} onChange={(value) => update(index, { scopeId: value })} placeholder="scope id" />
                <Field label="备注" value={account.note || ""} onChange={(value) => update(index, { note: value })} placeholder="可填写来源或归属" />
                <Field
                  label="IDToken"
                  type="password"
                  value={account.idToken || ""}
                  onChange={(value) =>
                    update(index, normalizeMaskedValue(account, "idToken", "keepIdToken", "hasIdToken", "idTokenSuffix", value))
                  }
                  placeholder="留空保持当前值"
                  hint={accountHint(account.hasIdToken, account.idTokenSuffix)}
                />
                <Field
                  label="Cookie"
                  type="password"
                  value={account.cookie || ""}
                  onChange={(value) =>
                    update(index, normalizeMaskedValue(account, "cookie", "keepCookie", "hasCookie", "cookieSuffix", value))
                  }
                  placeholder="留空保持当前值"
                  hint={accountHint(account.hasCookie, account.cookieSuffix)}
                />
              </div>
            </div>
          ))
        ) : (
          <div className="empty-state small">还没有 Bing 账号。</div>
        )}
      </div>
    </div>
  );
}

function CozeAccountsEditor(props: { accounts: CozeAccount[]; onChange: (accounts: CozeAccount[]) => void }) {
  const { accounts, onChange } = props;

  function update(index: number, patch: Partial<CozeAccount>) {
    onChange(accounts.map((item, current) => (current === index ? { ...item, ...patch } : item)));
  }

  return (
    <div className="editor-card">
      <div className="editor-card__header">
        <div>
          <h4>Coze WebSDK 账号池</h4>
          <p>支持邮箱密码、validate 或 cookies 方式保存；后台保存后会触发重建。</p>
        </div>
        <button
          type="button"
          className="ghost-button"
          onClick={() =>
            onChange([
              ...accounts,
              {
                id: createId("coze"),
                name: `Coze ${accounts.length + 1}`,
                email: "",
                password: "",
                validate: "",
                cookies: "",
                note: "",
                default: accounts.length === 0,
              },
            ])
          }
        >
          新增
        </button>
      </div>
      <div className="stack-list">
        {accounts.length ? (
          accounts.map((account, index) => (
            <div key={account.id} className="account-card">
              <div className="account-card__toolbar">
                <button type="button" className="pill-button" onClick={() => onChange(ensureSingleDefault(accounts, index))}>
                  {account.default ? "默认账号" : "设为默认"}
                </button>
                <button type="button" className="pill-button pill-button--danger" onClick={() => onChange(accounts.filter((item) => item.id !== account.id))}>
                  删除
                </button>
              </div>
              <div className="grid cols-2">
                <Field label="别名" value={account.name} onChange={(value) => update(index, { name: value })} placeholder={`Coze ${index + 1}`} />
                <Field label="邮箱" value={account.email} onChange={(value) => update(index, { email: value })} placeholder="email@example.com" />
                <Field label="备注" value={account.note || ""} onChange={(value) => update(index, { note: value })} placeholder="账号说明" />
                <Field
                  label="密码"
                  type="password"
                  value={account.password || ""}
                  onChange={(value) =>
                    update(index, normalizeMaskedValue(account, "password", "keepPassword", "hasPassword", "passwordSuffix", value))
                  }
                  placeholder="留空保持当前值"
                  hint={accountHint(account.hasPassword, account.passwordSuffix)}
                />
                <Field
                  label="Validate"
                  type="password"
                  value={account.validate || ""}
                  onChange={(value) =>
                    update(index, normalizeMaskedValue(account, "validate", "keepValidate", "hasValidate", "validateSuffix", value))
                  }
                  placeholder="留空保持当前值"
                  hint={accountHint(account.hasValidate, account.validateSuffix)}
                />
                <Field
                  label="Cookies"
                  type="password"
                  value={account.cookies || ""}
                  onChange={(value) =>
                    update(index, normalizeMaskedValue(account, "cookies", "keepCookies", "hasCookies", "cookiesSuffix", value))
                  }
                  placeholder="留空保持当前值"
                  hint={accountHint(account.hasCookies, account.cookiesSuffix)}
                />
              </div>
            </div>
          ))
        ) : (
          <div className="empty-state small">还没有 Coze 账号。</div>
        )}
      </div>
    </div>
  );
}

export default function App() {
  const [bootstrap, setBootstrap] = useState<BootstrapResponse | null>(null);
  const [config, setConfig] = useState<RuntimeConfig | null>(null);
  const [draft, setDraft] = useState<RuntimeConfig | null>(null);
  const [models, setModels] = useState<Record<string, string[]>>({});
  const [activeTab, setActiveTab] = useState<TabKey>("overview");
  const [flash, setFlash] = useState<Flash>(null);
  const [bootPending, setBootPending] = useState(true);
  const [loginPending, setLoginPending] = useState(false);
  const [savePending, setSavePending] = useState(false);
  const [loginPassword, setLoginPassword] = useState("");
  const [providerTests, setProviderTests] = useState<Record<string, ProviderTestDraft>>({});
  const [providerTestResults, setProviderTestResults] = useState<Record<string, ProviderTestResult>>({});
  const [windsurfActionPending, setWindsurfActionPending] = useState<WindsurfAction | "">("");
  const [windsurfResponse, setWindsurfResponse] = useState<Record<string, unknown> | null>(null);
  const [windsurfError, setWindsurfError] = useState("");
  const [playgroundPending, setPlaygroundPending] = useState(false);
  const [playgroundError, setPlaygroundError] = useState("");
  const [playgroundResponse, setPlaygroundResponse] = useState<PlaygroundResponse | null>(null);
  const [playgroundDraft, setPlaygroundDraft] = useState({
    provider: "windsurf",
    model: "",
    accountId: "",
    system: "",
    prompt: "Write one short sentence confirming the request path is working.",
    stream: false,
  });

  const dirty = useMemo(() => {
    if (!config || !draft) {
      return false;
    }
    return JSON.stringify(config) !== JSON.stringify(draft);
  }, [config, draft]);

  async function requestJson<T>(path: string, init?: RequestInit): Promise<RequestResult<T>> {
    const headers = new Headers(init?.headers || {});
    if (init?.body && !headers.has("Content-Type")) {
      headers.set("Content-Type", "application/json");
    }

    const response = await fetch(path, {
      credentials: "include",
      ...init,
      headers,
    });

    let payload: T | undefined;
    try {
      payload = (await response.json()) as T;
    } catch {
      payload = undefined;
    }

    const payloadError = payload && typeof payload === "object" ? (payload as { error?: string }).error : undefined;
    const okFlag = payload && typeof payload === "object" && "ok" in (payload as object)
      ? Boolean((payload as { ok?: boolean }).ok)
      : response.ok;

    if (response.status === 401) {
      setBootstrap((current) => (current ? { ...current, authenticated: false, requiresLogin: true } : current));
    }

    return {
      ok: response.ok && okFlag,
      status: response.status,
      data: payload,
      error: payloadError || (!response.ok ? `${response.status} ${response.statusText}` : undefined),
    };
  }

  async function loadWorkspace() {
    const [configResult, modelResult] = await Promise.all([
      requestJson<ConfigResponse>("/api/admin/config"),
      requestJson<ModelsResponse>("/api/admin/models"),
    ]);

    if (!configResult.ok || !configResult.data) {
      throw new Error(appError(configResult, "读取配置失败"));
    }
    if (!modelResult.ok || !modelResult.data) {
      throw new Error(appError(modelResult, "读取模型列表失败"));
    }

    setConfig(configResult.data.config);
    setDraft(clone(configResult.data.config));
    setModels(modelResult.data.models || {});
  }

  async function loadBootstrap(withWorkspace = true) {
    setBootPending(true);
    try {
      const result = await requestJson<BootstrapResponse>("/api/admin/bootstrap");
      if (!result.ok || !result.data) {
        throw new Error(appError(result, "读取启动信息失败"));
      }

      setBootstrap(result.data);
      if (withWorkspace && (result.data.authenticated || !result.data.requiresLogin)) {
        await loadWorkspace();
      } else if (result.data.requiresLogin && !result.data.authenticated) {
        setConfig(null);
        setDraft(null);
        setModels({});
      }
    } catch (error) {
      setFlash({ kind: "error", text: error instanceof Error ? error.message : "加载失败" });
    } finally {
      setBootPending(false);
    }
  }

  useEffect(() => {
    void loadBootstrap(true);
  }, []);

  function mutateDraft(mutator: (current: RuntimeConfig) => void) {
    setDraft((current) => {
      if (!current) {
        return current;
      }
      const next = clone(current);
      mutator(next);
      return next;
    });
  }

  function defaultProviderTest(name: string): ProviderTestDraft {
    return {
      model: providerModels(models, name)[0] || "",
      accountId: findDefaultAccount(providerAccounts(draft, name as ProviderName)),
      prompt: PLAYGROUND_FALLBACK_PROMPT,
    };
  }

  function providerTestState(name: string): ProviderTestDraft {
    return providerTests[name] || defaultProviderTest(name);
  }

  function updateProviderTest(name: string, patch: Partial<ProviderTestDraft>) {
    setProviderTests((current) => ({
      ...current,
      [name]: {
        ...providerTestState(name),
        ...patch,
      },
    }));
  }

  async function handleLogin(event: FormEvent) {
    event.preventDefault();
    if (!loginPassword.trim()) {
      setFlash({ kind: "error", text: "请输入管理员密码。" });
      return;
    }
    setLoginPending(true);
    try {
      const result = await requestJson<{ ok: boolean; error?: string }>("/api/admin/auth/login", {
        method: "POST",
        body: JSON.stringify({ password: loginPassword }),
      });
      if (!result.ok) {
        throw new Error(appError(result, "登录失败"));
      }
      setLoginPassword("");
      setFlash({ kind: "success", text: "登录成功，正在加载控制台。" });
      await loadBootstrap(true);
    } catch (error) {
      setFlash({ kind: "error", text: error instanceof Error ? error.message : "登录失败" });
    } finally {
      setLoginPending(false);
    }
  }

  async function handleLogout() {
    const result = await requestJson<{ ok: boolean; error?: string }>("/api/admin/auth/logout", {
      method: "POST",
    });
    if (!result.ok) {
      setFlash({ kind: "error", text: appError(result, "退出登录失败") });
      return;
    }
    setFlash({ kind: "info", text: "已退出登录。" });
    await loadBootstrap(false);
  }

  async function handleSave() {
    if (!draft) {
      return;
    }
    setSavePending(true);
    try {
      const result = await requestJson<ConfigResponse & { models?: Record<string, string[]> }>("/api/admin/config", {
        method: "PUT",
        body: JSON.stringify(draft),
      });
      if (!result.ok || !result.data) {
        throw new Error(appError(result, "保存配置失败"));
      }

      setConfig(result.data.config);
      setDraft(clone(result.data.config));
      if (result.data.models) {
        setModels(result.data.models);
      }
      await loadBootstrap(false);
      setFlash({ kind: "success", text: "配置已保存并刷新到运行时。" });
    } catch (error) {
      setFlash({ kind: "error", text: error instanceof Error ? error.message : "保存失败" });
    } finally {
      setSavePending(false);
    }
  }

  async function runProviderTest(name: string) {
    const state = providerTestState(name);
    setProviderTestResults((current) => ({
      ...current,
      [name]: { pending: true },
    }));

    const result = await requestJson<PlaygroundResponse>("/api/admin/test/provider", {
      method: "POST",
      body: JSON.stringify({
        provider: name,
        model: state.model,
        accountId: state.accountId || undefined,
        prompt: state.prompt || PLAYGROUND_FALLBACK_PROMPT,
      }),
    });

    setProviderTestResults((current) => ({
      ...current,
      [name]: {
        pending: false,
        error: result.ok ? "" : appError(result, "测试失败"),
        response: result.data,
      },
    }));
  }

  async function runWindsurfAction(action: WindsurfAction) {
    const accounts = draft?.windsurf.accounts || [];
    const model = providerModels(models, "windsurf")[0] || "";
    const accountId = findDefaultAccount(accounts);
    setWindsurfActionPending(action);
    setWindsurfError("");
    setWindsurfResponse(null);

    const result = await requestJson<Record<string, unknown>>("/api/admin/test/windsurf", {
      method: "POST",
      body: JSON.stringify({
        action,
        accountId,
        model,
        prompt: PLAYGROUND_FALLBACK_PROMPT,
      }),
    });

    if (!result.ok) {
      setWindsurfError(appError(result, "Windsurf 测试失败"));
    } else {
      setWindsurfResponse(result.data || null);
    }
    setWindsurfActionPending("");
  }

  async function runPlayground() {
    setPlaygroundPending(true);
    setPlaygroundError("");
    setPlaygroundResponse(null);
    const result = await requestJson<PlaygroundResponse>("/api/admin/playground/chat", {
      method: "POST",
      body: JSON.stringify({
        provider: playgroundDraft.provider,
        model: playgroundDraft.model,
        accountId: playgroundDraft.accountId || undefined,
        system: playgroundDraft.system,
        prompt: playgroundDraft.prompt,
        stream: playgroundDraft.stream,
      }),
    });
    if (!result.ok) {
      setPlaygroundError(appError(result, "Playground 请求失败"));
    } else {
      setPlaygroundResponse(result.data || null);
    }
    setPlaygroundPending(false);
  }

  function setProviderEnabled(provider: ProviderName, value: boolean) {
    mutateDraft((current) => {
      if (provider === "windsurf") {
        current.windsurf.enabled = value;
        return;
      }
      current.providers[provider].enabled = value;
    });
  }

  function setProviderModelsValue(provider: ModelProvider, value: string[]) {
    mutateDraft((current) => {
      current.providers[provider].models = value;
    });
  }

  function viewOverview() {
    return (
      <div className="stack-page">
        <SectionTitle
          eyebrow="Admin Console"
          title="运行时概览"
          description="这里展示当前管理台状态、持久化路径和各 provider 的即时摘要。"
        />

        <div className="hero-panel">
          <div>
            <div className="hero-panel__eyebrow">Bypass Admin</div>
            <h1>同服务内置管理后台</h1>
            <p>
              这套控制台现在和 Go 服务同进程部署，配置写入独立运行时文件，适合在 Zeabur 上持续自动部署与热更新。
            </p>
          </div>
          <div className="hero-panel__meta">
            <div className="metric-card">
              <span>版本</span>
              <strong>{bootstrap?.version || "admin-v1"}</strong>
            </div>
            <div className="metric-card">
              <span>配置存储</span>
              <strong>{bootstrap?.storage?.writable ? "可写" : "只读 / 未挂载"}</strong>
            </div>
            <div className="metric-card">
              <span>当前路径</span>
              <strong>{prettyPath(bootstrap?.storage?.path || "")}</strong>
            </div>
          </div>
        </div>

        <div className="stats-grid">
          {(bootstrap?.providers || []).map((provider) => (
            <div className="summary-card" key={provider.name}>
              <div className="summary-card__top">
                <strong>{prettyProvider(provider.name)}</strong>
                <span className={badgeClass(provider.enabled)}>{provider.enabled ? "启用" : "停用"}</span>
              </div>
              <div className="summary-card__numbers">
                <div>
                  <span>模型</span>
                  <strong>{provider.modelCount}</strong>
                </div>
                <div>
                  <span>账号</span>
                  <strong>{provider.accountCount}</strong>
                </div>
              </div>
              <div className="summary-card__tags">
                {(provider.models || []).slice(0, 4).map((model) => (
                  <code key={model}>{model}</code>
                ))}
                {provider.models.length > 4 ? <code>+{provider.models.length - 4}</code> : null}
              </div>
            </div>
          ))}
        </div>

        <div className="panel-grid">
          <div className="panel">
            <SectionTitle title="持久化状态" description="Zeabur 推荐挂载 `/app/data`，这样配置在重启和新版本部署后都能保留。" />
            <div className="grid cols-2">
              <DataPair label="运行时文件" value={bootstrap?.storage?.path || "未检测"} />
              <DataPair label="目录" value={bootstrap?.storage?.dir || "未检测"} />
              <DataPair label="可写" value={bootstrap?.storage?.writable ? "是" : "否"} />
              <DataPair label="已存在" value={bootstrap?.storage?.exists ? "是" : "否"} />
            </div>
            <div className="inline-note">挂载建议: {bootstrap?.storage?.mountHint || "/app/data"}</div>
          </div>

          <div className="panel">
            <SectionTitle title="Windsurf 快照" description="第一优先级 provider，建议优先把账号、profile 与模型映射都维护在后台。" />
            <div className="grid cols-2">
              <DataPair label="启用状态" value={providerEnabled(draft, "windsurf") ? "已启用" : "已停用"} />
              <DataPair label="账号数" value={String(draft?.windsurf.accounts.length || 0)} />
              <DataPair label="模型映射数" value={String(draft?.windsurf.models.length || 0)} />
              <DataPair label="走代理" value={draft?.windsurf.proxied ? "是" : "否"} />
            </div>
          </div>
        </div>
      </div>
    );
  }

  function viewProviders() {
    if (!draft) {
      return null;
    }

    const cursor = draft.providers.cursor;
    const deepseek = draft.providers.deepseek;
    const qodo = draft.providers.qodo;
    const lmsys = draft.providers.lmsys;
    const blackbox = draft.providers.blackbox;
    const you = draft.providers.you;
    const grok = draft.providers.grok;
    const bing = draft.providers.bing;
    const coze = draft.providers.coze;

    return (
      <div className="stack-page">
        <SectionTitle
          eyebrow="Provider Console"
          title="统一配置"
          description="非 Windsurf provider 先提供统一的配置、启用开关与基础连通性测试。"
        />

        <div className="panel">
          <div className="grid cols-2">
            <Field
              label="全局代理"
              value={draft.server.proxied}
              onChange={(value) => mutateDraft((current) => { current.server.proxied = value; })}
              placeholder="http://127.0.0.1:7890"
              hint="保存后会写入运行时快照并参与各 provider 热更新。"
            />
            <ToggleField
              label="Think Reason"
              checked={draft.server.thinkReason}
              onChange={(value) => mutateDraft((current) => { current.server.thinkReason = value; })}
              hint="保留原有服务级思维输出开关。"
            />
          </div>
        </div>

        <div className="provider-grid">
          <div className="provider-card">
            <div className="provider-card__header">
              <div>
                <h3>Cursor</h3>
                <p>{PROVIDER_NOTES.cursor}</p>
              </div>
              <span className={badgeClass(cursor.enabled)}>{cursor.enabled ? "已启用" : "已停用"}</span>
            </div>
            <ToggleField label="启用 Cursor" checked={cursor.enabled} onChange={(value) => setProviderEnabled("cursor", value)} />
            <StringListEditor
              label="附加模型"
              items={cursor.models}
              onChange={(items) => setProviderModelsValue("cursor", items)}
              placeholder="例如 claude-3.7-sonnet"
            />
            <Field
              label="Checksum"
              value={cursor.settings?.checksum || ""}
              onChange={(value) => mutateDraft((current) => { current.providers.cursor.settings = { ...(current.providers.cursor.settings || {}), checksum: value }; })}
              placeholder="可填固定值或远程 checksum 地址"
            />
            <SecretAccountsEditor
              title="测试账号"
              description="仅供后台测试 / Playground 使用，不影响外部调用协议。"
              accounts={cursor.accounts}
              secretLabel="Access Token"
              addLabel="Cursor"
              onChange={(accounts) => mutateDraft((current) => { current.providers.cursor.accounts = accounts; })}
            />
            <ProviderTestPanel
              provider="cursor"
              models={providerModels(models, "cursor")}
              accounts={cursor.accounts}
              state={providerTestState("cursor")}
              result={providerTestResults.cursor}
              onChange={updateProviderTest}
              onRun={runProviderTest}
            />
          </div>

          <div className="provider-card">
            <div className="provider-card__header">
              <div>
                <h3>DeepSeek</h3>
                <p>{PROVIDER_NOTES.deepseek}</p>
              </div>
              <span className={badgeClass(deepseek.enabled)}>{deepseek.enabled ? "已启用" : "已停用"}</span>
            </div>
            <ToggleField label="启用 DeepSeek" checked={deepseek.enabled} onChange={(value) => setProviderEnabled("deepseek", value)} />
            <div className="inline-note">当前对外模型保持 `deepseek-chat / deepseek-reasoner`，此处重点是维护后台测试凭据。</div>
            <SecretAccountsEditor
              title="测试账号"
              description="保存 Bearer token 后，可直接在后台跑最小聊天测试。"
              accounts={deepseek.accounts}
              secretLabel="Bearer Token"
              addLabel="DeepSeek"
              onChange={(accounts) => mutateDraft((current) => { current.providers.deepseek.accounts = accounts; })}
            />
            <ProviderTestPanel
              provider="deepseek"
              models={providerModels(models, "deepseek")}
              accounts={deepseek.accounts}
              state={providerTestState("deepseek")}
              result={providerTestResults.deepseek}
              onChange={updateProviderTest}
              onRun={runProviderTest}
            />
          </div>

          <div className="provider-card">
            <div className="provider-card__header">
              <div>
                <h3>Qodo</h3>
                <p>{PROVIDER_NOTES.qodo}</p>
              </div>
              <span className={badgeClass(qodo.enabled)}>{qodo.enabled ? "已启用" : "已停用"}</span>
            </div>
            <ToggleField label="启用 Qodo" checked={qodo.enabled} onChange={(value) => setProviderEnabled("qodo", value)} />
            <StringListEditor
              label="附加模型"
              items={qodo.models}
              onChange={(items) => setProviderModelsValue("qodo", items)}
              placeholder="例如 claude-3-7-sonnet"
            />
            <Field
              label="Qodo Key"
              type="password"
              value={qodo.settings?.key || ""}
              onChange={(value) => mutateDraft((current) => { current.providers.qodo.settings = { ...(current.providers.qodo.settings || {}), key: value }; })}
              placeholder="留空保持当前值"
              hint="保存时如果保持为空，服务端会沿用现有值。"
            />
            <ProviderTestPanel
              provider="qodo"
              models={providerModels(models, "qodo")}
              state={providerTestState("qodo")}
              result={providerTestResults.qodo}
              onChange={updateProviderTest}
              onRun={runProviderTest}
            />
          </div>

          <div className="provider-card">
            <div className="provider-card__header">
              <div>
                <h3>LMSYS</h3>
                <p>{PROVIDER_NOTES.lmsys}</p>
              </div>
              <span className={badgeClass(lmsys.enabled)}>{lmsys.enabled ? "已启用" : "已停用"}</span>
            </div>
            <ToggleField label="启用 LMSYS" checked={lmsys.enabled} onChange={(value) => setProviderEnabled("lmsys", value)} />
            <StringListEditor
              label="附加模型"
              items={lmsys.models}
              onChange={(items) => setProviderModelsValue("lmsys", items)}
              placeholder="例如 llama-3.1-70b-instruct"
            />
            <SecretAccountsEditor
              title="配置字符串"
              description="当前会写入 `lmsys.token`，可保存一条默认配置字符串。"
              accounts={lmsys.accounts}
              secretLabel="Token / Config"
              addLabel="LMSYS"
              onChange={(accounts) => mutateDraft((current) => { current.providers.lmsys.accounts = accounts; })}
            />
            <ProviderTestPanel
              provider="lmsys"
              models={providerModels(models, "lmsys")}
              state={providerTestState("lmsys")}
              result={providerTestResults.lmsys}
              onChange={updateProviderTest}
              onRun={runProviderTest}
            />
          </div>

          <div className="provider-card">
            <div className="provider-card__header">
              <div>
                <h3>Blackbox</h3>
                <p>{PROVIDER_NOTES.blackbox}</p>
              </div>
              <span className={badgeClass(blackbox.enabled)}>{blackbox.enabled ? "已启用" : "已停用"}</span>
            </div>
            <ToggleField label="启用 Blackbox" checked={blackbox.enabled} onChange={(value) => setProviderEnabled("blackbox", value)} />
            <StringListEditor
              label="附加模型"
              items={blackbox.models}
              onChange={(items) => setProviderModelsValue("blackbox", items)}
              placeholder="例如 Claude-Sonnet-3.7"
            />
            <Field
              label="Validated Token"
              type="password"
              value={blackbox.settings?.validatedToken || ""}
              onChange={(value) => mutateDraft((current) => { current.providers.blackbox.settings = { ...(current.providers.blackbox.settings || {}), validatedToken: value }; })}
              placeholder="留空保持当前值"
              hint="该值用于实际请求参数，不依赖额外账号列表。"
            />
            <ProviderTestPanel
              provider="blackbox"
              models={providerModels(models, "blackbox")}
              state={providerTestState("blackbox")}
              result={providerTestResults.blackbox}
              onChange={updateProviderTest}
              onRun={runProviderTest}
            />
          </div>

          <div className="provider-card">
            <div className="provider-card__header">
              <div>
                <h3>You</h3>
                <p>{PROVIDER_NOTES.you}</p>
              </div>
              <span className={badgeClass(you.enabled)}>{you.enabled ? "已启用" : "已停用"}</span>
            </div>
            <ToggleField label="启用 You" checked={you.enabled} onChange={(value) => setProviderEnabled("you", value)} />
            <ToggleField
              label="启用 Task 保活"
              checked={you.settings?.task === "true"}
              onChange={(value) => mutateDraft((current) => { current.providers.you.settings = { ...(current.providers.you.settings || {}), task: value ? "true" : "false" }; })}
            />
            <StringListEditor
              label="附加模型"
              items={you.models}
              onChange={(items) => setProviderModelsValue("you", items)}
              placeholder="例如 gpt_4o"
            />
            <SecretAccountsEditor
              title="Cookie 池"
              description="这些 Cookie 会写入运行时池并支持热更新。"
              accounts={you.accounts}
              secretLabel="Cookie"
              addLabel="You"
              onChange={(accounts) => mutateDraft((current) => { current.providers.you.accounts = accounts; })}
            />
            <ProviderTestPanel
              provider="you"
              models={providerModels(models, "you")}
              state={providerTestState("you")}
              result={providerTestResults.you}
              onChange={updateProviderTest}
              onRun={runProviderTest}
            />
          </div>

          <div className="provider-card">
            <div className="provider-card__header">
              <div>
                <h3>Grok</h3>
                <p>{PROVIDER_NOTES.grok}</p>
              </div>
              <span className={badgeClass(grok.enabled)}>{grok.enabled ? "已启用" : "已停用"}</span>
            </div>
            <ToggleField label="启用 Grok" checked={grok.enabled} onChange={(value) => setProviderEnabled("grok", value)} />
            <div className="inline-note">当前模型为固定集合，重点是 Cookie 池是否可用。</div>
            <SecretAccountsEditor
              title="Cookie 池"
              description="Cookie 保存后会立刻刷新轮询池。"
              accounts={grok.accounts}
              secretLabel="Cookie"
              addLabel="Grok"
              onChange={(accounts) => mutateDraft((current) => { current.providers.grok.accounts = accounts; })}
            />
            <ProviderTestPanel
              provider="grok"
              models={providerModels(models, "grok")}
              state={providerTestState("grok")}
              result={providerTestResults.grok}
              onChange={updateProviderTest}
              onRun={runProviderTest}
            />
          </div>

          <div className="provider-card">
            <div className="provider-card__header">
              <div>
                <h3>Bing</h3>
                <p>{PROVIDER_NOTES.bing}</p>
              </div>
              <span className={badgeClass(bing.enabled)}>{bing.enabled ? "已启用" : "已停用"}</span>
            </div>
            <ToggleField label="启用 Bing" checked={bing.enabled} onChange={(value) => setProviderEnabled("bing", value)} />
            <BingAccountsEditor accounts={bing.accounts} onChange={(accounts) => mutateDraft((current) => { current.providers.bing.accounts = accounts; })} />
            <ProviderTestPanel
              provider="bing"
              models={providerModels(models, "bing")}
              state={providerTestState("bing")}
              result={providerTestResults.bing}
              onChange={updateProviderTest}
              onRun={runProviderTest}
            />
          </div>

          <div className="provider-card">
            <div className="provider-card__header">
              <div>
                <h3>Coze</h3>
                <p>{PROVIDER_NOTES.coze}</p>
              </div>
              <span className={badgeClass(coze.enabled)}>{coze.enabled ? "已启用" : "已停用"}</span>
            </div>
            <ToggleField label="启用 Coze" checked={coze.enabled} onChange={(value) => setProviderEnabled("coze", value)} />
            <div className="grid cols-2">
              <Field
                label="Bot"
                value={coze.settings?.bot || ""}
                onChange={(value) => mutateDraft((current) => { current.providers.coze.settings = { ...(current.providers.coze.settings || {}), bot: value }; })}
                placeholder="custom-128k"
              />
              <Field
                label="默认模型"
                value={coze.settings?.model || ""}
                onChange={(value) => mutateDraft((current) => { current.providers.coze.settings = { ...(current.providers.coze.settings || {}), model: value }; })}
                placeholder="gpt-4o-128k"
              />
            </div>
            <Field
              label="System"
              value={coze.settings?.system || ""}
              onChange={(value) => mutateDraft((current) => { current.providers.coze.settings = { ...(current.providers.coze.settings || {}), system: value }; })}
              textarea
              rows={5}
              placeholder="用于 coze bot draft / publish 的 system prompt"
            />
            <CozeAccountsEditor accounts={coze.accounts} onChange={(accounts) => mutateDraft((current) => { current.providers.coze.accounts = accounts; })} />
            <ProviderTestPanel
              provider="coze"
              models={providerModels(models, "coze")}
              state={providerTestState("coze")}
              result={providerTestResults.coze}
              onChange={updateProviderTest}
              onRun={runProviderTest}
            />
          </div>
        </div>
      </div>
    );
  }

  function viewWindsurf() {
    if (!draft) {
      return null;
    }

    const windsurf = draft.windsurf;
    const mappedModels = providerModels(models, "windsurf");

    return (
      <div className="stack-page">
        <SectionTitle
          eyebrow="Windsurf First"
          title="Windsurf 专区"
          description="账号、profile、模型映射、测试与 Playground 优先围绕 Windsurf 做完整闭环。"
        />

        <div className="provider-card">
          <div className="provider-card__header">
            <div>
              <h3>基础开关</h3>
              <p>这里的改动会直接刷新运行时快照，并影响 `/v1/models` 暴露模型。</p>
            </div>
            <span className={badgeClass(windsurf.enabled)}>{windsurf.enabled ? "已启用" : "已停用"}</span>
          </div>
          <div className="grid cols-2">
            <ToggleField
              label="启用 Windsurf"
              checked={windsurf.enabled}
              onChange={(value) => setProviderEnabled("windsurf", value)}
            />
            <ToggleField
              label="Windsurf 走代理"
              checked={windsurf.proxied}
              onChange={(value) => mutateDraft((current) => { current.windsurf.proxied = value; })}
            />
          </div>
        </div>

        <SecretAccountsEditor
          title="账号 / Token 列表"
          description="列表页默认只保留别名、备注与后四位提示，保存时留空即可保持原有 token。"
          accounts={windsurf.accounts}
          secretLabel="Windsurf Token"
          addLabel="Windsurf"
          onChange={(accounts) => mutateDraft((current) => { current.windsurf.accounts = accounts; })}
        />
        <div className="provider-card">
          <SectionTitle title="Profile 配置" description="这些字段会参与请求头与客户端画像生成。" />
          <div className="grid cols-2">
            <Field label="Name" value={windsurf.profile.name} onChange={(value) => mutateDraft((current) => { current.windsurf.profile.name = value; })} />
            <Field label="Title" value={windsurf.profile.title} onChange={(value) => mutateDraft((current) => { current.windsurf.profile.title = value; })} />
            <Field label="Lang" value={windsurf.profile.lang} onChange={(value) => mutateDraft((current) => { current.windsurf.profile.lang = value; })} />
            <Field label="Version1" value={windsurf.profile.version1} onChange={(value) => mutateDraft((current) => { current.windsurf.profile.version1 = value; })} />
            <Field label="Version2" value={windsurf.profile.version2} onChange={(value) => mutateDraft((current) => { current.windsurf.profile.version2 = value; })} />
            <Field label="User-Agent" value={windsurf.profile.userAgent} onChange={(value) => mutateDraft((current) => { current.windsurf.profile.userAgent = value; })} />
            <Field label="OS" value={windsurf.profile.os} onChange={(value) => mutateDraft((current) => { current.windsurf.profile.os = value; })} textarea rows={4} monospace />
            <Field label="Equi" value={windsurf.profile.equi} onChange={(value) => mutateDraft((current) => { current.windsurf.profile.equi = value; })} textarea rows={4} monospace />
          </div>
          <Field
            label="Instructions"
            value={windsurf.profile.instructions}
            onChange={(value) => mutateDraft((current) => { current.windsurf.profile.instructions = value; })}
            textarea
            rows={6}
          />
          <Field
            label="Instructions Suffix"
            value={windsurf.profile.instructionsSuffix}
            onChange={(value) => mutateDraft((current) => { current.windsurf.profile.instructionsSuffix = value; })}
            textarea
            rows={5}
          />
        </div>

        <div className="provider-card">
          <SectionTitle title="模型映射" description="修改后保存即可影响 Windsurf 模型列表和 `/v1/models` 输出。" />
          <div className="stack-list">
            {windsurf.models.map((item, index) => (
              <div key={`${item.name}-${index}`} className="mapping-row">
                <input
                  value={item.name}
                  placeholder="模型名，例如 claude-3-7-sonnet"
                  onChange={(event) => mutateDraft((current) => { current.windsurf.models[index].name = event.target.value; })}
                />
                <input
                  value={item.value}
                  placeholder="numeric id"
                  onChange={(event) => mutateDraft((current) => { current.windsurf.models[index].value = event.target.value; })}
                />
                <button
                  type="button"
                  className="pill-button pill-button--danger"
                  onClick={() => mutateDraft((current) => { current.windsurf.models = current.windsurf.models.filter((_, currentIndex) => currentIndex !== index); })}
                >
                  删除
                </button>
              </div>
            ))}
          </div>
          <div className="toolbar">
            <button type="button" className="ghost-button" onClick={() => mutateDraft((current) => { current.windsurf.models.push({ name: "", value: "" }); })}>
              新增映射
            </button>
          </div>
          <div className="summary-card__tags">
            {mappedModels.slice(0, 8).map((model) => (
              <code key={model}>{model}</code>
            ))}
          </div>
        </div>

        <div className="panel-grid">
          <div className="panel">
            <SectionTitle title="Windsurf 专项测试" description="验证 token 格式、JWT 获取与最小聊天 smoke test。" />
            <div className="toolbar">
              <button type="button" className="primary-button" disabled={windsurfActionPending === "validate"} onClick={() => void runWindsurfAction("validate")}>
                {windsurfActionPending === "validate" ? "验证中..." : "Token 校验"}
              </button>
              <button type="button" className="ghost-button" disabled={windsurfActionPending === "jwt"} onClick={() => void runWindsurfAction("jwt")}>
                {windsurfActionPending === "jwt" ? "获取中..." : "JWT 测试"}
              </button>
              <button type="button" className="ghost-button" disabled={windsurfActionPending === "smoke"} onClick={() => void runWindsurfAction("smoke")}>
                {windsurfActionPending === "smoke" ? "请求中..." : "Smoke Test"}
              </button>
            </div>
            {windsurfError ? <div className="result-box result-box--error">{windsurfError}</div> : null}
            {windsurfResponse ? <pre className="result-box">{JSON.stringify(windsurfResponse, null, 2)}</pre> : null}
          </div>

          <div className="panel">
            <SectionTitle title="即时预览" description="当前默认账号与模型可直接被 Playground 复用。" />
            <div className="grid cols-2">
              <DataPair label="默认账号" value={findDefaultAccount(windsurf.accounts) || "未设置"} />
              <DataPair label="默认模型" value={mappedModels[0] || "未暴露"} />
              <DataPair label="模型映射" value={String(windsurf.models.length)} />
              <DataPair label="后端状态" value={windsurf.enabled ? "活跃" : "已禁用"} />
            </div>
          </div>
        </div>
      </div>
    );
  }

  function viewPlayground() {
    const provider = playgroundDraft.provider;
    const availableModels = providerModels(models, provider);
    const availableAccounts = providerAccounts(draft, provider as ProviderName);

    return (
      <div className="stack-page">
        <SectionTitle
          eyebrow="Playground"
          title="调试 / 测试台"
          description="直接复用现有后端链路，返回原始响应、提取内容、状态码与耗时。"
        />

        <div className="playground-layout">
          <div className="panel">
            <div className="grid cols-2">
              <label className="field">
                <span>Provider</span>
                <select
                  value={provider}
                  onChange={(event) => {
                    const nextProvider = event.target.value;
                    setPlaygroundDraft((current) => ({
                      ...current,
                      provider: nextProvider,
                      model: providerModels(models, nextProvider)[0] || "",
                      accountId: findDefaultAccount(providerAccounts(draft, nextProvider as ProviderName)),
                    }));
                  }}
                >
                  {PROVIDER_ORDER.filter((name) => providerModels(models, name).length || providerEnabled(draft, name)).map((name) => (
                    <option key={name} value={name}>{prettyProvider(name)}</option>
                  ))}
                </select>
              </label>
              <label className="field">
                <span>模型</span>
                <select value={playgroundDraft.model} onChange={(event) => setPlaygroundDraft((current) => ({ ...current, model: event.target.value }))}>
                  <option value="">使用默认模型</option>
                  {availableModels.map((model) => (
                    <option key={model} value={model}>{model}</option>
                  ))}
                </select>
              </label>
            </div>

            {availableAccounts.length ? (
              <label className="field">
                <span>账号</span>
                <select value={playgroundDraft.accountId} onChange={(event) => setPlaygroundDraft((current) => ({ ...current, accountId: event.target.value }))}>
                  <option value="">自动选默认账号</option>
                  {availableAccounts.map((account) => (
                    <option key={account.id} value={account.id}>{account.name || account.id}</option>
                  ))}
                </select>
              </label>
            ) : null}

            <Field label="System Prompt" value={playgroundDraft.system} onChange={(value) => setPlaygroundDraft((current) => ({ ...current, system: value }))} textarea rows={5} placeholder="可选 system prompt" />
            <Field label="用户请求" value={playgroundDraft.prompt} onChange={(value) => setPlaygroundDraft((current) => ({ ...current, prompt: value }))} textarea rows={7} placeholder="写一段最小验证请求" />
            <ToggleField label="Stream 模式" checked={playgroundDraft.stream} onChange={(value) => setPlaygroundDraft((current) => ({ ...current, stream: value }))} />
            <div className="toolbar">
              <button type="button" className="primary-button" disabled={playgroundPending} onClick={() => void runPlayground()}>
                {playgroundPending ? "请求中..." : "运行 Playground"}
              </button>
            </div>
          </div>

          <div className="panel">
            <SectionTitle title="结果" description="原样显示服务端返回，便于排查 provider 链路问题。" />
            {playgroundError ? <div className="result-box result-box--error">{playgroundError}</div> : null}
            {playgroundResponse?.details ? (
              <>
                <div className="grid cols-2">
                  <DataPair label="Provider" value={playgroundResponse.details.provider} />
                  <DataPair label="模型" value={playgroundResponse.details.model} />
                  <DataPair label="状态码" value={String(playgroundResponse.details.status)} />
                  <DataPair label="耗时" value={`${playgroundResponse.details.durationMs} ms`} />
                </div>
                <Field label="提取内容" value={playgroundResponse.details.content || ""} onChange={() => undefined} textarea rows={6} />
                <pre className="result-box">{playgroundResponse.details.raw}</pre>
              </>
            ) : (
              <div className="empty-state small">还没有运行结果。</div>
            )}
          </div>
        </div>
      </div>
    );
  }

  if (bootPending && !bootstrap) {
    return <div className="loading-screen">正在启动管理控制台...</div>;
  }

  if (bootstrap?.requiresLogin && !bootstrap.authenticated) {
    return (
      <div className="login-shell">
        <div className="login-card">
          <div className="section-title__eyebrow">Bypass Admin</div>
          <h1>管理员登录</h1>
          <p>优先使用 `server.password`；如果配置里写的是环境变量占位符，会自动回退到 `PASSWORD`。</p>
          <form onSubmit={handleLogin} className="login-form">
            <Field label="管理员密码" type="password" value={loginPassword} onChange={setLoginPassword} placeholder="输入 server.password 或 PASSWORD" />
            <button type="submit" className="primary-button" disabled={loginPending}>
              {loginPending ? "登录中..." : "进入控制台"}
            </button>
          </form>
          {flash ? <div className={`flash flash--${flash.kind}`}>{flash.text}</div> : null}
          <div className="login-note">运行时配置路径: {prettyStorage(bootstrap.storage)}</div>
        </div>
      </div>
    );
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand__badge">B</div>
          <div>
            <strong>Bypass Admin</strong>
            <span>{prettyStorage(bootstrap?.storage)}</span>
          </div>
        </div>
        <nav className="nav-list">
          {TAB_ITEMS.map((item) => (
            <button key={item.key} type="button" className={`nav-item ${activeTab === item.key ? "nav-item--active" : ""}`} onClick={() => setActiveTab(item.key)}>
              <strong>{item.label}</strong>
              <span>{item.subtitle}</span>
            </button>
          ))}
        </nav>
        <div className="sidebar__footer">
          <div className="sidebar__meta">
            <span>保存路径</span>
            <strong>{bootstrap?.storage?.path || "/app/data/runtime-config.json"}</strong>
          </div>
        </div>
      </aside>

      <main className="main">
        <header className="topbar">
          <div>
            <small>Zeabur Friendly</small>
            <h1>{TAB_ITEMS.find((item) => item.key === activeTab)?.label}</h1>
          </div>
          <div className="topbar__actions">
            <span className={`status-badge ${dirty ? "status-badge--warn" : "status-badge--ok"}`}>
              {dirty ? "有未保存改动" : "运行时已同步"}
            </span>
            <button type="button" className="ghost-button" onClick={() => void loadBootstrap(true)}>刷新</button>
            <button type="button" className="primary-button" disabled={!dirty || savePending} onClick={() => void handleSave()}>
              {savePending ? "保存中..." : "保存配置"}
            </button>
            {bootstrap?.requiresLogin ? <button type="button" className="ghost-button" onClick={() => void handleLogout()}>退出</button> : null}
          </div>
        </header>

        {flash ? <div className={`flash flash--${flash.kind}`}>{flash.text}</div> : null}
        {activeTab === "overview" ? viewOverview() : null}
        {activeTab === "providers" ? viewProviders() : null}
        {activeTab === "windsurf" ? viewWindsurf() : null}
        {activeTab === "playground" ? viewPlayground() : null}
      </main>
    </div>
  );
}

function ProviderTestPanel(props: {
  provider: string;
  models: string[];
  accounts?: SecretAccount[];
  state: ProviderTestDraft;
  result?: ProviderTestResult;
  onChange: (provider: string, patch: Partial<ProviderTestDraft>) => void;
  onRun: (provider: string) => void;
}) {
  const { provider, models, accounts = [], state, result, onChange, onRun } = props;

  return (
    <div className="test-panel">
      <h4>基础测试</h4>
      <div className="grid cols-2">
        <label className="field">
          <span>模型</span>
          <select value={state.model} onChange={(event) => onChange(provider, { model: event.target.value })}>
            <option value="">自动选择默认模型</option>
            {models.map((model) => (
              <option key={model} value={model}>{model}</option>
            ))}
          </select>
        </label>
        {accounts.length ? (
          <label className="field">
            <span>账号</span>
            <select value={state.accountId} onChange={(event) => onChange(provider, { accountId: event.target.value })}>
              <option value="">自动选择默认账号</option>
              {accounts.map((account) => (
                <option key={account.id} value={account.id}>{account.name || account.id}</option>
              ))}
            </select>
          </label>
        ) : null}
      </div>
      <Field label="测试提示词" value={state.prompt} onChange={(value) => onChange(provider, { prompt: value })} placeholder={PLAYGROUND_FALLBACK_PROMPT} />
      <div className="toolbar">
        <button type="button" className="ghost-button" disabled={!!result?.pending} onClick={() => onRun(provider)}>
          {result?.pending ? "测试中..." : "运行测试"}
        </button>
      </div>
      {result?.error ? <div className="result-box result-box--error">{result.error}</div> : null}
      {result?.response?.details ? (
        <div className="result-mini">
          <div className="result-mini__meta">
            <span>状态 {result.response.details.status}</span>
            <span>{result.response.details.durationMs} ms</span>
          </div>
          <pre>{result.response.details.content || result.response.details.raw}</pre>
        </div>
      ) : null}
    </div>
  );
}

function DataPair(props: { label: string; value: string }) {
  return (
    <div className="data-pair">
      <span>{props.label}</span>
      <strong>{props.value}</strong>
    </div>
  );
}
