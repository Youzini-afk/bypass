package runtimecfg

type RuntimeConfig struct {
	Version   int             `json:"version"`
	UpdatedAt string          `json:"updatedAt,omitempty"`
	Server    ServerConfig    `json:"server"`
	Windsurf  WindsurfConfig  `json:"windsurf"`
	Providers ProvidersConfig `json:"providers"`
}

type ServerConfig struct {
	Proxied     string `json:"proxied"`
	ThinkReason bool   `json:"thinkReason"`
}

type SecretAccount struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Secret       string `json:"secret,omitempty"`
	Note         string `json:"note,omitempty"`
	Default      bool   `json:"default"`
	HasSecret    bool   `json:"hasSecret,omitempty"`
	KeepSecret   bool   `json:"keepSecret,omitempty"`
	SecretSuffix string `json:"secretSuffix,omitempty"`
}

type NamedValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type WindsurfProfile struct {
	Name               string `json:"name"`
	Title              string `json:"title"`
	Lang               string `json:"lang"`
	Version1           string `json:"version1"`
	Version2           string `json:"version2"`
	OS                 string `json:"os"`
	Equi               string `json:"equi"`
	UserAgent          string `json:"userAgent"`
	Instructions       string `json:"instructions"`
	InstructionsSuffix string `json:"instructionsSuffix"`
}

type WindsurfConfig struct {
	Enabled  bool            `json:"enabled"`
	Proxied  bool            `json:"proxied"`
	Accounts []SecretAccount `json:"accounts"`
	Profile  WindsurfProfile `json:"profile"`
	Models   []NamedValue    `json:"models"`
}

type ProviderConfig struct {
	Enabled  bool              `json:"enabled"`
	Models   []string          `json:"models"`
	Accounts []SecretAccount   `json:"accounts"`
	Settings map[string]string `json:"settings,omitempty"`
}

type BingAccount struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ScopeID       string `json:"scopeId"`
	IDToken       string `json:"idToken,omitempty"`
	Cookie        string `json:"cookie,omitempty"`
	Note          string `json:"note,omitempty"`
	Default       bool   `json:"default"`
	HasIDToken    bool   `json:"hasIdToken,omitempty"`
	KeepIDToken   bool   `json:"keepIdToken,omitempty"`
	HasCookie     bool   `json:"hasCookie,omitempty"`
	KeepCookie    bool   `json:"keepCookie,omitempty"`
	IDTokenSuffix string `json:"idTokenSuffix,omitempty"`
	CookieSuffix  string `json:"cookieSuffix,omitempty"`
}

type BingConfig struct {
	Enabled  bool          `json:"enabled"`
	Accounts []BingAccount `json:"accounts"`
}

type CozeAccount struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	Password       string `json:"password,omitempty"`
	Validate       string `json:"validate,omitempty"`
	Cookies        string `json:"cookies,omitempty"`
	Note           string `json:"note,omitempty"`
	Default        bool   `json:"default"`
	HasPassword    bool   `json:"hasPassword,omitempty"`
	KeepPassword   bool   `json:"keepPassword,omitempty"`
	HasValidate    bool   `json:"hasValidate,omitempty"`
	KeepValidate   bool   `json:"keepValidate,omitempty"`
	HasCookies     bool   `json:"hasCookies,omitempty"`
	KeepCookies    bool   `json:"keepCookies,omitempty"`
	PasswordSuffix string `json:"passwordSuffix,omitempty"`
	ValidateSuffix string `json:"validateSuffix,omitempty"`
	CookiesSuffix  string `json:"cookiesSuffix,omitempty"`
}

type CozeConfig struct {
	Enabled  bool              `json:"enabled"`
	Accounts []CozeAccount     `json:"accounts"`
	Settings map[string]string `json:"settings,omitempty"`
}

type ProvidersConfig struct {
	Cursor   ProviderConfig `json:"cursor"`
	Deepseek ProviderConfig `json:"deepseek"`
	Qodo     ProviderConfig `json:"qodo"`
	Lmsys    ProviderConfig `json:"lmsys"`
	Blackbox ProviderConfig `json:"blackbox"`
	You      ProviderConfig `json:"you"`
	Grok     ProviderConfig `json:"grok"`
	Bing     BingConfig     `json:"bing"`
	Coze     CozeConfig     `json:"coze"`
}

type StorageStatus struct {
	Path      string `json:"path"`
	Dir       string `json:"dir"`
	Writable  bool   `json:"writable"`
	Exists    bool   `json:"exists"`
	MountHint string `json:"mountHint"`
}
