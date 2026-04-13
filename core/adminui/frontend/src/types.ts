export type StorageStatus = {
  path: string;
  dir: string;
  writable: boolean;
  exists: boolean;
  mountHint: string;
};

export type SecretAccount = {
  id: string;
  name: string;
  secret?: string;
  note?: string;
  default: boolean;
  hasSecret?: boolean;
  keepSecret?: boolean;
  secretSuffix?: string;
};

export type NamedValue = {
  name: string;
  value: string;
};

export type WindsurfProfile = {
  name: string;
  title: string;
  lang: string;
  version1: string;
  version2: string;
  os: string;
  equi: string;
  userAgent: string;
  instructions: string;
  instructionsSuffix: string;
};

export type WindsurfConfig = {
  enabled: boolean;
  proxied: boolean;
  accounts: SecretAccount[];
  profile: WindsurfProfile;
  models: NamedValue[];
};

export type ProviderConfig = {
  enabled: boolean;
  models: string[];
  accounts: SecretAccount[];
  settings?: Record<string, string>;
};

export type BingAccount = {
  id: string;
  name: string;
  scopeId: string;
  idToken?: string;
  cookie?: string;
  note?: string;
  default: boolean;
  hasIdToken?: boolean;
  keepIdToken?: boolean;
  hasCookie?: boolean;
  keepCookie?: boolean;
  idTokenSuffix?: string;
  cookieSuffix?: string;
};

export type BingConfig = {
  enabled: boolean;
  accounts: BingAccount[];
};

export type CozeAccount = {
  id: string;
  name: string;
  email: string;
  password?: string;
  validate?: string;
  cookies?: string;
  note?: string;
  default: boolean;
  hasPassword?: boolean;
  keepPassword?: boolean;
  hasValidate?: boolean;
  keepValidate?: boolean;
  hasCookies?: boolean;
  keepCookies?: boolean;
  passwordSuffix?: string;
  validateSuffix?: string;
  cookiesSuffix?: string;
};

export type CozeConfig = {
  enabled: boolean;
  accounts: CozeAccount[];
  settings?: Record<string, string>;
};

export type RuntimeConfig = {
  version: number;
  updatedAt?: string;
  server: {
    proxied: string;
    thinkReason: boolean;
  };
  windsurf: WindsurfConfig;
  providers: {
    cursor: ProviderConfig;
    deepseek: ProviderConfig;
    qodo: ProviderConfig;
    lmsys: ProviderConfig;
    blackbox: ProviderConfig;
    you: ProviderConfig;
    grok: ProviderConfig;
    bing: BingConfig;
    coze: CozeConfig;
  };
};

export type ProviderSummary = {
  name: string;
  enabled: boolean;
  modelCount: number;
  accountCount: number;
  models: string[];
};

export type WindsurfOfficialModel = {
  name: string;
  availability: string;
  source: string;
  sourceDate: string;
  note?: string;
  builtinMapping: boolean;
};

export type BootstrapResponse = {
  ok: boolean;
  authenticated: boolean;
  requiresLogin: boolean;
  storage: StorageStatus;
  providers: ProviderSummary[];
  windsurfCatalog: WindsurfOfficialModel[];
  version: string;
};

export type ConfigResponse = {
  ok: boolean;
  config: RuntimeConfig;
  storage: StorageStatus;
};

export type ModelsResponse = {
  ok: boolean;
  models: Record<string, string[]>;
};

export type PlaygroundResult = {
  provider: string;
  model: string;
  status: number;
  durationMs: number;
  contentType: string;
  content?: string;
  raw: string;
};

export type PlaygroundResponse = {
  ok: boolean;
  details: PlaygroundResult;
  error?: string;
};
