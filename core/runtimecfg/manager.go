package runtimecfg

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"chatgpt-adapter/core/windsurfmeta"
	"github.com/google/uuid"
	"github.com/iocgo/sdk/env"
)

const (
	configVersion          = 1
	defaultStorageFileName = "runtime-config.json"
)

var (
	manager   *Manager
	managerMu sync.RWMutex
	initOnce  sync.Once
	initErr   error

	reloaders   = map[string]func(*env.Environment) error{}
	reloadersMu sync.RWMutex
)

type Manager struct {
	env     *env.Environment
	path    string
	storage StorageStatus

	mu     sync.RWMutex
	config RuntimeConfig
}

func Init(environment *env.Environment) error {
	initOnce.Do(func() {
		if environment == nil {
			initErr = errors.New("runtime config environment is nil")
			return
		}

		mgr := &Manager{
			env:  environment,
			path: resolvePath(),
		}
		mgr.storage = statStorage(mgr.path)

		cfg := mgr.extractBootstrap()
		if loaded, ok := mgr.readFile(); ok {
			cfg = mgr.mergeBootstrap(cfg, loaded)
		}
		if err := mgr.applyRuntimeConfig(cfg, false); err != nil {
			initErr = err
			return
		}

		managerMu.Lock()
		manager = mgr
		managerMu.Unlock()
	})
	return initErr
}

func Current(redacted bool) RuntimeConfig {
	managerMu.RLock()
	mgr := manager
	managerMu.RUnlock()
	if mgr == nil {
		return RuntimeConfig{}
	}

	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	cfg := cloneConfig(mgr.config)
	if redacted {
		redactConfig(&cfg)
	}
	return cfg
}

func Storage() StorageStatus {
	managerMu.RLock()
	mgr := manager
	managerMu.RUnlock()
	if mgr == nil {
		return StorageStatus{}
	}
	return statStorage(mgr.path)
}

func Save(next RuntimeConfig) error {
	managerMu.RLock()
	mgr := manager
	managerMu.RUnlock()
	if mgr == nil {
		return errors.New("runtime config manager not initialized")
	}

	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	prev := cloneConfig(mgr.config)
	merged := mergeSecrets(prev, next)
	merged = mgr.mergeBootstrap(mgr.extractBootstrap(), merged)

	if err := mgr.writeFile(merged); err != nil {
		return err
	}
	if err := mgr.applyRuntimeConfig(merged, true); err != nil {
		_ = mgr.writeFile(prev)
		_ = mgr.applyRuntimeConfig(prev, true)
		return err
	}
	return nil
}

func Enabled(provider string) bool {
	managerMu.RLock()
	mgr := manager
	managerMu.RUnlock()
	if mgr == nil {
		return true
	}

	mgr.mu.RLock()
	defer mgr.mu.RUnlock()
	return mgr.providerEnabled(provider)
}

func RegisterReloader(name string, fn func(*env.Environment) error) {
	if fn == nil {
		return
	}
	reloadersMu.Lock()
	defer reloadersMu.Unlock()
	reloaders[strings.ToLower(strings.TrimSpace(name))] = fn
}

func runtimeConfigPath() string {
	managerMu.RLock()
	mgr := manager
	managerMu.RUnlock()
	if mgr == nil {
		return resolvePath()
	}
	return mgr.path
}

func (m *Manager) providerEnabled(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "windsurf":
		return m.config.Windsurf.Enabled
	case "cursor":
		return m.config.Providers.Cursor.Enabled
	case "deepseek":
		return m.config.Providers.Deepseek.Enabled
	case "qodo":
		return m.config.Providers.Qodo.Enabled
	case "lmsys":
		return m.config.Providers.Lmsys.Enabled
	case "blackbox":
		return m.config.Providers.Blackbox.Enabled
	case "you":
		return m.config.Providers.You.Enabled
	case "grok":
		return m.config.Providers.Grok.Enabled
	case "bing":
		return m.config.Providers.Bing.Enabled
	case "coze":
		return m.config.Providers.Coze.Enabled
	default:
		return true
	}
}

func (m *Manager) applyRuntimeConfig(cfg RuntimeConfig, runReload bool) error {
	cfg = normalizeConfig(cfg)
	m.applyToEnv(cfg)

	if runReload {
		for _, name := range []string{"you", "grok", "bing", "coze"} {
			if err := runReloader(name, m.env); err != nil {
				return err
			}
		}
	}

	m.config = cfg
	m.storage = statStorage(m.path)
	return nil
}

func (m *Manager) applyToEnv(cfg RuntimeConfig) {
	m.env.Set("server.proxied", strings.TrimSpace(cfg.Server.Proxied))
	m.env.Set("server.think_reason", cfg.Server.ThinkReason)

	m.env.Set("windsurf.proxied", cfg.Windsurf.Proxied)
	m.env.Set("windsurf.name", strings.TrimSpace(cfg.Windsurf.Profile.Name))
	m.env.Set("windsurf.title", strings.TrimSpace(cfg.Windsurf.Profile.Title))
	m.env.Set("windsurf.lang", strings.TrimSpace(cfg.Windsurf.Profile.Lang))
	m.env.Set("windsurf.version1", strings.TrimSpace(cfg.Windsurf.Profile.Version1))
	m.env.Set("windsurf.version2", strings.TrimSpace(cfg.Windsurf.Profile.Version2))
	m.env.Set("windsurf.os", strings.TrimSpace(cfg.Windsurf.Profile.OS))
	m.env.Set("windsurf.equi", strings.TrimSpace(cfg.Windsurf.Profile.Equi))
	m.env.Set("windsurf.user-agent", strings.TrimSpace(cfg.Windsurf.Profile.UserAgent))
	m.env.Set("windsurf.instructions", strings.TrimSpace(cfg.Windsurf.Profile.Instructions))
	m.env.Set("windsurf.instructions_suffix", strings.TrimSpace(cfg.Windsurf.Profile.InstructionsSuffix))
	m.env.Set("windsurf.model", namedValuesToMap(cfg.Windsurf.Models))

	m.env.Set("cursor.model", normalizeStringSlice(cfg.Providers.Cursor.Models))
	m.env.Set("cursor.checksum", strings.TrimSpace(cfg.Providers.Cursor.Settings["checksum"]))

	m.env.Set("qodo.model", normalizeStringSlice(cfg.Providers.Qodo.Models))
	m.env.Set("qodo.key", strings.TrimSpace(cfg.Providers.Qodo.Settings["key"]))

	m.env.Set("lmsys.model", normalizeStringSlice(cfg.Providers.Lmsys.Models))
	m.env.Set("lmsys.token", defaultSecret(cfg.Providers.Lmsys.Accounts))

	m.env.Set("blackbox.model", normalizeStringSlice(cfg.Providers.Blackbox.Models))
	m.env.Set("blackbox.token", strings.TrimSpace(cfg.Providers.Blackbox.Settings["validatedToken"]))

	m.env.Set("you.model", normalizeStringSlice(cfg.Providers.You.Models))
	m.env.Set("you.cookies", secretsFromAccounts(cfg.Providers.You.Accounts))
	m.env.Set("you.task", parseBoolString(cfg.Providers.You.Settings["task"]))

	m.env.Set("grok.cookies", secretsFromAccounts(cfg.Providers.Grok.Accounts))
	m.env.Set("bing.cookies", bingAccountsToValues(cfg.Providers.Bing.Accounts))
	m.env.Set("coze.websdk.accounts", cozeAccountsToValues(cfg.Providers.Coze.Accounts))
	m.env.Set("coze.websdk.bot", strings.TrimSpace(cfg.Providers.Coze.Settings["bot"]))
	m.env.Set("coze.websdk.model", strings.TrimSpace(cfg.Providers.Coze.Settings["model"]))
	m.env.Set("coze.websdk.system", strings.TrimSpace(cfg.Providers.Coze.Settings["system"]))
}

func (m *Manager) extractBootstrap() RuntimeConfig {
	cfg := RuntimeConfig{
		Version: configVersion,
		Server: ServerConfig{
			Proxied:     strings.TrimSpace(m.env.GetString("server.proxied")),
			ThinkReason: m.env.GetBool("server.think_reason"),
		},
		Windsurf: WindsurfConfig{
			Enabled: true,
			Proxied: m.env.GetBool("windsurf.proxied"),
			Profile: WindsurfProfile{
				Name:               strings.TrimSpace(m.env.GetString("windsurf.name")),
				Title:              strings.TrimSpace(m.env.GetString("windsurf.title")),
				Lang:               strings.TrimSpace(m.env.GetString("windsurf.lang")),
				Version1:           strings.TrimSpace(m.env.GetString("windsurf.version1")),
				Version2:           strings.TrimSpace(m.env.GetString("windsurf.version2")),
				OS:                 strings.TrimSpace(m.env.GetString("windsurf.os")),
				Equi:               strings.TrimSpace(m.env.GetString("windsurf.equi")),
				UserAgent:          strings.TrimSpace(m.env.GetString("windsurf.user-agent")),
				Instructions:       strings.TrimSpace(m.env.GetString("windsurf.instructions")),
				InstructionsSuffix: strings.TrimSpace(m.env.GetString("windsurf.instructions_suffix")),
			},
			Models: mapToNamedValues(mergeDefaultWindsurfModels(m.env.GetStringMapString("windsurf.model"))),
		},
		Providers: ProvidersConfig{
			Cursor: ProviderConfig{
				Enabled: true,
				Models:  normalizeStringSlice(m.env.GetStringSlice("cursor.model")),
				Settings: map[string]string{
					"checksum": strings.TrimSpace(m.env.GetString("cursor.checksum")),
				},
			},
			Deepseek: ProviderConfig{
				Enabled: true,
			},
			Qodo: ProviderConfig{
				Enabled: true,
				Models:  normalizeStringSlice(m.env.GetStringSlice("qodo.model")),
				Settings: map[string]string{
					"key": strings.TrimSpace(m.env.GetString("qodo.key")),
				},
			},
			Lmsys: ProviderConfig{
				Enabled: true,
				Models:  normalizeStringSlice(m.env.GetStringSlice("lmsys.model")),
				Accounts: secretFromSingleValue(
					"lmsys-default",
					"LMSYS Default",
					strings.TrimSpace(m.env.GetString("lmsys.token")),
				),
			},
			Blackbox: ProviderConfig{
				Enabled: true,
				Models:  normalizeStringSlice(m.env.GetStringSlice("blackbox.model")),
				Settings: map[string]string{
					"validatedToken": strings.TrimSpace(m.env.GetString("blackbox.token")),
				},
			},
			You: ProviderConfig{
				Enabled:  true,
				Models:   normalizeStringSlice(m.env.GetStringSlice("you.model")),
				Accounts: accountsFromValues("you", "Cookie", m.env.GetStringSlice("you.cookies")),
				Settings: map[string]string{"task": boolToString(m.env.GetBool("you.task"))},
			},
			Grok: ProviderConfig{
				Enabled:  true,
				Accounts: accountsFromValues("grok", "Cookie", m.env.GetStringSlice("grok.cookies")),
			},
			Bing: BingConfig{
				Enabled:  true,
				Accounts: extractBingAccounts(m.env),
			},
			Coze: CozeConfig{
				Enabled:  true,
				Accounts: extractCozeAccounts(m.env),
				Settings: map[string]string{
					"bot":    strings.TrimSpace(m.env.GetString("coze.websdk.bot")),
					"model":  strings.TrimSpace(m.env.GetString("coze.websdk.model")),
					"system": strings.TrimSpace(m.env.GetString("coze.websdk.system")),
				},
			},
		},
	}
	return normalizeConfig(cfg)
}

func (m *Manager) mergeBootstrap(base RuntimeConfig, loaded RuntimeConfig) RuntimeConfig {
	if loaded.Version == 0 {
		return base
	}

	base.Version = loaded.Version
	base.UpdatedAt = loaded.UpdatedAt

	base.Server = loaded.Server
	base.Windsurf = loaded.Windsurf
	base.Providers.Cursor = mergeProvider(base.Providers.Cursor, loaded.Providers.Cursor)
	base.Providers.Deepseek = mergeProvider(base.Providers.Deepseek, loaded.Providers.Deepseek)
	base.Providers.Qodo = mergeProvider(base.Providers.Qodo, loaded.Providers.Qodo)
	base.Providers.Lmsys = mergeProvider(base.Providers.Lmsys, loaded.Providers.Lmsys)
	base.Providers.Blackbox = mergeProvider(base.Providers.Blackbox, loaded.Providers.Blackbox)
	base.Providers.You = mergeProvider(base.Providers.You, loaded.Providers.You)
	base.Providers.Grok = mergeProvider(base.Providers.Grok, loaded.Providers.Grok)
	base.Providers.Bing = mergeBing(base.Providers.Bing, loaded.Providers.Bing)
	base.Providers.Coze = mergeCoze(base.Providers.Coze, loaded.Providers.Coze)
	return normalizeConfig(base)
}

func (m *Manager) readFile() (RuntimeConfig, bool) {
	var cfg RuntimeConfig
	buffer, err := os.ReadFile(m.path)
	if err != nil {
		return cfg, false
	}
	if json.Unmarshal(buffer, &cfg) != nil {
		return RuntimeConfig{}, false
	}
	return cfg, true
}

func (m *Manager) writeFile(cfg RuntimeConfig) error {
	cfg.Version = configVersion
	cfg.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	cfg = normalizeConfig(cfg)

	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}

	buffer, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, buffer, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, m.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func runReloader(name string, environment *env.Environment) error {
	reloadersMu.RLock()
	fn, ok := reloaders[strings.ToLower(strings.TrimSpace(name))]
	reloadersMu.RUnlock()
	if !ok {
		return nil
	}
	return fn(environment)
}

func normalizeConfig(cfg RuntimeConfig) RuntimeConfig {
	cfg.Version = configVersion
	cfg.Server.Proxied = strings.TrimSpace(cfg.Server.Proxied)

	cfg.Windsurf.Accounts = normalizeSecretAccounts(cfg.Windsurf.Accounts)
	cfg.Windsurf.Models = normalizeWindsurfNamedValues(cfg.Windsurf.Models)
	cfg.Windsurf.Profile = normalizeWindsurfProfile(cfg.Windsurf.Profile)

	cfg.Providers.Cursor = normalizeProvider(cfg.Providers.Cursor)
	cfg.Providers.Deepseek = normalizeProvider(cfg.Providers.Deepseek)
	cfg.Providers.Qodo = normalizeProvider(cfg.Providers.Qodo)
	cfg.Providers.Lmsys = normalizeProvider(cfg.Providers.Lmsys)
	cfg.Providers.Blackbox = normalizeProvider(cfg.Providers.Blackbox)
	cfg.Providers.You = normalizeProvider(cfg.Providers.You)
	cfg.Providers.Grok = normalizeProvider(cfg.Providers.Grok)
	cfg.Providers.Bing = normalizeBing(cfg.Providers.Bing)
	cfg.Providers.Coze = normalizeCoze(cfg.Providers.Coze)
	return cfg
}

func normalizeProvider(cfg ProviderConfig) ProviderConfig {
	cfg.Models = normalizeStringSlice(cfg.Models)
	cfg.Accounts = normalizeSecretAccounts(cfg.Accounts)
	cfg.Settings = normalizeSettings(cfg.Settings)
	return cfg
}

func normalizeBing(cfg BingConfig) BingConfig {
	cfg.Accounts = normalizeBingAccounts(cfg.Accounts)
	return cfg
}

func normalizeCoze(cfg CozeConfig) CozeConfig {
	cfg.Accounts = normalizeCozeAccounts(cfg.Accounts)
	cfg.Settings = normalizeSettings(cfg.Settings)
	return cfg
}

func normalizeWindsurfProfile(profile WindsurfProfile) WindsurfProfile {
	profile.Name = strings.TrimSpace(profile.Name)
	profile.Title = strings.TrimSpace(profile.Title)
	profile.Lang = strings.TrimSpace(profile.Lang)
	profile.Version1 = strings.TrimSpace(profile.Version1)
	profile.Version2 = strings.TrimSpace(profile.Version2)
	profile.OS = strings.TrimSpace(profile.OS)
	profile.Equi = strings.TrimSpace(profile.Equi)
	profile.UserAgent = strings.TrimSpace(profile.UserAgent)
	profile.Instructions = strings.TrimSpace(profile.Instructions)
	profile.InstructionsSuffix = strings.TrimSpace(profile.InstructionsSuffix)
	return profile
}

func normalizeWindsurfNamedValues(values []NamedValue) []NamedValue {
	merged := make(map[string]string, len(values))
	for _, item := range values {
		name := windsurfmeta.CanonicalName(item.Name)
		value := strings.TrimSpace(item.Value)
		if name == "" || value == "" {
			continue
		}
		merged[name] = value
	}

	result := make([]NamedValue, 0, len(merged))
	for name, value := range merged {
		result = append(result, NamedValue{Name: name, Value: value})
	}
	return normalizeNamedValues(result)
}

func normalizeSecretAccounts(accounts []SecretAccount) []SecretAccount {
	if accounts == nil {
		return []SecretAccount{}
	}
	for idx := range accounts {
		accounts[idx].ID = ensureID(accounts[idx].ID)
		accounts[idx].Name = strings.TrimSpace(accounts[idx].Name)
		accounts[idx].Secret = strings.TrimSpace(accounts[idx].Secret)
		accounts[idx].Note = strings.TrimSpace(accounts[idx].Note)
	}
	ensureSingleDefault(accounts)
	return accounts
}

func normalizeNamedValues(values []NamedValue) []NamedValue {
	result := make([]NamedValue, 0, len(values))
	for _, item := range values {
		name := strings.TrimSpace(item.Name)
		value := strings.TrimSpace(item.Value)
		if name == "" || value == "" {
			continue
		}
		result = append(result, NamedValue{Name: name, Value: value})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func normalizeStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizeSettings(settings map[string]string) map[string]string {
	if settings == nil {
		return map[string]string{}
	}
	result := make(map[string]string, len(settings))
	for key, value := range settings {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		result[key] = value
	}
	return result
}

func normalizeBingAccounts(accounts []BingAccount) []BingAccount {
	if accounts == nil {
		return []BingAccount{}
	}
	for idx := range accounts {
		accounts[idx].ID = ensureID(accounts[idx].ID)
		accounts[idx].Name = strings.TrimSpace(accounts[idx].Name)
		accounts[idx].ScopeID = strings.TrimSpace(accounts[idx].ScopeID)
		accounts[idx].IDToken = strings.TrimSpace(accounts[idx].IDToken)
		accounts[idx].Cookie = strings.TrimSpace(accounts[idx].Cookie)
		accounts[idx].Note = strings.TrimSpace(accounts[idx].Note)
	}
	ensureSingleDefaultBing(accounts)
	return accounts
}

func normalizeCozeAccounts(accounts []CozeAccount) []CozeAccount {
	if accounts == nil {
		return []CozeAccount{}
	}
	for idx := range accounts {
		accounts[idx].ID = ensureID(accounts[idx].ID)
		accounts[idx].Name = strings.TrimSpace(accounts[idx].Name)
		accounts[idx].Email = strings.TrimSpace(accounts[idx].Email)
		accounts[idx].Password = strings.TrimSpace(accounts[idx].Password)
		accounts[idx].Validate = strings.TrimSpace(accounts[idx].Validate)
		accounts[idx].Cookies = strings.TrimSpace(accounts[idx].Cookies)
		accounts[idx].Note = strings.TrimSpace(accounts[idx].Note)
	}
	ensureSingleDefaultCoze(accounts)
	return accounts
}

func ensureSingleDefault(accounts []SecretAccount) {
	hasDefault := false
	for idx := range accounts {
		if accounts[idx].Default && !hasDefault {
			hasDefault = true
			continue
		}
		if hasDefault {
			accounts[idx].Default = false
		}
	}
	if !hasDefault && len(accounts) > 0 {
		accounts[0].Default = true
	}
}

func ensureSingleDefaultBing(accounts []BingAccount) {
	hasDefault := false
	for idx := range accounts {
		if accounts[idx].Default && !hasDefault {
			hasDefault = true
			continue
		}
		if hasDefault {
			accounts[idx].Default = false
		}
	}
	if !hasDefault && len(accounts) > 0 {
		accounts[0].Default = true
	}
}

func ensureSingleDefaultCoze(accounts []CozeAccount) {
	hasDefault := false
	for idx := range accounts {
		if accounts[idx].Default && !hasDefault {
			hasDefault = true
			continue
		}
		if hasDefault {
			accounts[idx].Default = false
		}
	}
	if !hasDefault && len(accounts) > 0 {
		accounts[0].Default = true
	}
}

func ensureID(id string) string {
	id = strings.TrimSpace(id)
	if id != "" {
		return id
	}
	return uuid.NewString()
}

func cloneConfig(cfg RuntimeConfig) RuntimeConfig {
	buffer, err := json.Marshal(cfg)
	if err != nil {
		return cfg
	}
	var cloned RuntimeConfig
	if json.Unmarshal(buffer, &cloned) != nil {
		return cfg
	}
	return cloned
}

func redactConfig(cfg *RuntimeConfig) {
	for idx := range cfg.Windsurf.Accounts {
		if cfg.Windsurf.Accounts[idx].Secret != "" {
			cfg.Windsurf.Accounts[idx].HasSecret = true
			cfg.Windsurf.Accounts[idx].KeepSecret = true
			cfg.Windsurf.Accounts[idx].SecretSuffix = secretSuffix(cfg.Windsurf.Accounts[idx].Secret)
			cfg.Windsurf.Accounts[idx].Secret = ""
		}
	}

	redactProvider := func(provider *ProviderConfig) {
		for idx := range provider.Accounts {
			if provider.Accounts[idx].Secret != "" {
				provider.Accounts[idx].HasSecret = true
				provider.Accounts[idx].KeepSecret = true
				provider.Accounts[idx].SecretSuffix = secretSuffix(provider.Accounts[idx].Secret)
				provider.Accounts[idx].Secret = ""
			}
		}
	}

	redactProvider(&cfg.Providers.Cursor)
	redactProvider(&cfg.Providers.Deepseek)
	redactProvider(&cfg.Providers.Qodo)
	redactProvider(&cfg.Providers.Lmsys)
	redactProvider(&cfg.Providers.Blackbox)
	redactProvider(&cfg.Providers.You)
	redactProvider(&cfg.Providers.Grok)

	for idx := range cfg.Providers.Bing.Accounts {
		if cfg.Providers.Bing.Accounts[idx].IDToken != "" {
			cfg.Providers.Bing.Accounts[idx].HasIDToken = true
			cfg.Providers.Bing.Accounts[idx].KeepIDToken = true
			cfg.Providers.Bing.Accounts[idx].IDTokenSuffix = secretSuffix(cfg.Providers.Bing.Accounts[idx].IDToken)
			cfg.Providers.Bing.Accounts[idx].IDToken = ""
		}
		if cfg.Providers.Bing.Accounts[idx].Cookie != "" {
			cfg.Providers.Bing.Accounts[idx].HasCookie = true
			cfg.Providers.Bing.Accounts[idx].KeepCookie = true
			cfg.Providers.Bing.Accounts[idx].CookieSuffix = secretSuffix(cfg.Providers.Bing.Accounts[idx].Cookie)
			cfg.Providers.Bing.Accounts[idx].Cookie = ""
		}
	}

	for idx := range cfg.Providers.Coze.Accounts {
		if cfg.Providers.Coze.Accounts[idx].Password != "" {
			cfg.Providers.Coze.Accounts[idx].HasPassword = true
			cfg.Providers.Coze.Accounts[idx].KeepPassword = true
			cfg.Providers.Coze.Accounts[idx].PasswordSuffix = secretSuffix(cfg.Providers.Coze.Accounts[idx].Password)
			cfg.Providers.Coze.Accounts[idx].Password = ""
		}
		if cfg.Providers.Coze.Accounts[idx].Validate != "" {
			cfg.Providers.Coze.Accounts[idx].HasValidate = true
			cfg.Providers.Coze.Accounts[idx].KeepValidate = true
			cfg.Providers.Coze.Accounts[idx].ValidateSuffix = secretSuffix(cfg.Providers.Coze.Accounts[idx].Validate)
			cfg.Providers.Coze.Accounts[idx].Validate = ""
		}
		if cfg.Providers.Coze.Accounts[idx].Cookies != "" {
			cfg.Providers.Coze.Accounts[idx].HasCookies = true
			cfg.Providers.Coze.Accounts[idx].KeepCookies = true
			cfg.Providers.Coze.Accounts[idx].CookiesSuffix = secretSuffix(cfg.Providers.Coze.Accounts[idx].Cookies)
			cfg.Providers.Coze.Accounts[idx].Cookies = ""
		}
	}

	if token := cfg.Providers.Blackbox.Settings["validatedToken"]; token != "" {
		cfg.Providers.Blackbox.Settings["validatedToken"] = ""
	}
	if token := cfg.Providers.Qodo.Settings["key"]; token != "" {
		cfg.Providers.Qodo.Settings["key"] = ""
	}
}

func mergeSecrets(prev RuntimeConfig, next RuntimeConfig) RuntimeConfig {
	next.Providers.Blackbox.Settings = normalizeSettings(next.Providers.Blackbox.Settings)
	next.Providers.Qodo.Settings = normalizeSettings(next.Providers.Qodo.Settings)
	next.Windsurf.Accounts = mergeSecretAccountList(prev.Windsurf.Accounts, next.Windsurf.Accounts)
	next.Providers.Cursor.Accounts = mergeSecretAccountList(prev.Providers.Cursor.Accounts, next.Providers.Cursor.Accounts)
	next.Providers.Deepseek.Accounts = mergeSecretAccountList(prev.Providers.Deepseek.Accounts, next.Providers.Deepseek.Accounts)
	next.Providers.Qodo.Accounts = mergeSecretAccountList(prev.Providers.Qodo.Accounts, next.Providers.Qodo.Accounts)
	next.Providers.Lmsys.Accounts = mergeSecretAccountList(prev.Providers.Lmsys.Accounts, next.Providers.Lmsys.Accounts)
	next.Providers.Blackbox.Accounts = mergeSecretAccountList(prev.Providers.Blackbox.Accounts, next.Providers.Blackbox.Accounts)
	next.Providers.You.Accounts = mergeSecretAccountList(prev.Providers.You.Accounts, next.Providers.You.Accounts)
	next.Providers.Grok.Accounts = mergeSecretAccountList(prev.Providers.Grok.Accounts, next.Providers.Grok.Accounts)
	next.Providers.Bing.Accounts = mergeBingAccountList(prev.Providers.Bing.Accounts, next.Providers.Bing.Accounts)
	next.Providers.Coze.Accounts = mergeCozeAccountList(prev.Providers.Coze.Accounts, next.Providers.Coze.Accounts)

	if strings.TrimSpace(next.Providers.Blackbox.Settings["validatedToken"]) == "" {
		next.Providers.Blackbox.Settings["validatedToken"] = prev.Providers.Blackbox.Settings["validatedToken"]
	}
	if strings.TrimSpace(next.Providers.Qodo.Settings["key"]) == "" {
		next.Providers.Qodo.Settings["key"] = prev.Providers.Qodo.Settings["key"]
	}
	return next
}

func mergeSecretAccountList(prev, next []SecretAccount) []SecretAccount {
	prevMap := map[string]SecretAccount{}
	for _, item := range prev {
		prevMap[item.ID] = item
	}
	for idx := range next {
		next[idx].ID = ensureID(next[idx].ID)
		if next[idx].KeepSecret && strings.TrimSpace(next[idx].Secret) == "" {
			next[idx].Secret = prevMap[next[idx].ID].Secret
		}
	}
	return next
}

func mergeBingAccountList(prev, next []BingAccount) []BingAccount {
	prevMap := map[string]BingAccount{}
	for _, item := range prev {
		prevMap[item.ID] = item
	}
	for idx := range next {
		next[idx].ID = ensureID(next[idx].ID)
		if next[idx].KeepIDToken && strings.TrimSpace(next[idx].IDToken) == "" {
			next[idx].IDToken = prevMap[next[idx].ID].IDToken
		}
		if next[idx].KeepCookie && strings.TrimSpace(next[idx].Cookie) == "" {
			next[idx].Cookie = prevMap[next[idx].ID].Cookie
		}
	}
	return next
}

func mergeCozeAccountList(prev, next []CozeAccount) []CozeAccount {
	prevMap := map[string]CozeAccount{}
	for _, item := range prev {
		prevMap[item.ID] = item
	}
	for idx := range next {
		next[idx].ID = ensureID(next[idx].ID)
		if next[idx].KeepPassword && strings.TrimSpace(next[idx].Password) == "" {
			next[idx].Password = prevMap[next[idx].ID].Password
		}
		if next[idx].KeepValidate && strings.TrimSpace(next[idx].Validate) == "" {
			next[idx].Validate = prevMap[next[idx].ID].Validate
		}
		if next[idx].KeepCookies && strings.TrimSpace(next[idx].Cookies) == "" {
			next[idx].Cookies = prevMap[next[idx].ID].Cookies
		}
	}
	return next
}

func mergeProvider(base, loaded ProviderConfig) ProviderConfig {
	base.Enabled = loaded.Enabled
	if loaded.Models != nil {
		base.Models = loaded.Models
	}
	if loaded.Accounts != nil {
		base.Accounts = loaded.Accounts
	}
	base.Settings = mergeStringMap(base.Settings, loaded.Settings)
	return base
}

func mergeBing(base, loaded BingConfig) BingConfig {
	base.Enabled = loaded.Enabled
	if loaded.Accounts != nil {
		base.Accounts = loaded.Accounts
	}
	return base
}

func mergeCoze(base, loaded CozeConfig) CozeConfig {
	base.Enabled = loaded.Enabled
	if loaded.Accounts != nil {
		base.Accounts = loaded.Accounts
	}
	base.Settings = mergeStringMap(base.Settings, loaded.Settings)
	return base
}

func mergeStringMap(base, loaded map[string]string) map[string]string {
	if loaded != nil {
		return normalizeSettings(loaded)
	}
	if base == nil {
		return map[string]string{}
	}
	return normalizeSettings(base)
}

func resolvePath() string {
	if value := strings.TrimSpace(os.Getenv("ADMIN_RUNTIME_CONFIG_PATH")); value != "" {
		return value
	}
	if _, err := os.Stat("/app"); err == nil {
		return filepath.Join("/app", "data", defaultStorageFileName)
	}
	return filepath.Join("data", defaultStorageFileName)
}

func statStorage(path string) StorageStatus {
	dir := filepath.Dir(path)
	status := StorageStatus{
		Path:      path,
		Dir:       dir,
		MountHint: "/app/data",
	}

	if _, err := os.Stat(path); err == nil {
		status.Exists = true
	}
	if err := os.MkdirAll(dir, 0o755); err == nil {
		testFile := filepath.Join(dir, ".runtimecfg-write-test")
		if writeErr := os.WriteFile(testFile, []byte("ok"), 0o644); writeErr == nil {
			status.Writable = true
			_ = os.Remove(testFile)
		}
	}
	return status
}

func mergeDefaultWindsurfModels(custom map[string]string) map[string]string {
	return windsurfmeta.NormalizeRegistry(custom)
}

func mapToNamedValues(values map[string]string) []NamedValue {
	result := make([]NamedValue, 0, len(values))
	for key, value := range values {
		result = append(result, NamedValue{Name: key, Value: value})
	}
	return normalizeNamedValues(result)
}

func namedValuesToMap(values []NamedValue) map[string]string {
	result := make(map[string]string, len(values))
	for _, item := range values {
		name := windsurfmeta.CanonicalName(item.Name)
		value := strings.TrimSpace(item.Value)
		if name == "" || value == "" {
			continue
		}
		result[name] = value
	}
	return result
}

func accountsFromValues(prefix string, label string, values []string) []SecretAccount {
	result := make([]SecretAccount, 0, len(values))
	for idx, value := range normalizeStringSlice(values) {
		result = append(result, SecretAccount{
			ID:      prefix + "-" + strings.ReplaceAll(uuid.NewString(), "-", ""),
			Name:    label + " " + intToString(idx+1),
			Secret:  value,
			Default: idx == 0,
		})
	}
	return result
}

func secretsFromAccounts(accounts []SecretAccount) []string {
	result := make([]string, 0, len(accounts))
	for _, item := range normalizeSecretAccounts(accounts) {
		if item.Secret == "" {
			continue
		}
		result = append(result, item.Secret)
	}
	return result
}

func defaultSecret(accounts []SecretAccount) string {
	accounts = normalizeSecretAccounts(accounts)
	for _, item := range accounts {
		if item.Default && item.Secret != "" {
			return item.Secret
		}
	}
	for _, item := range accounts {
		if item.Secret != "" {
			return item.Secret
		}
	}
	return ""
}

func secretFromSingleValue(id string, name string, value string) []SecretAccount {
	value = strings.TrimSpace(value)
	if value == "" {
		return []SecretAccount{}
	}
	return []SecretAccount{{
		ID:      id,
		Name:    name,
		Secret:  value,
		Default: true,
	}}
}

func extractBingAccounts(environment *env.Environment) []BingAccount {
	var raw []struct {
		ScopeID string `mapstructure:"scopeid"`
		IDToken string `mapstructure:"idtoken"`
		Cookie  string `mapstructure:"cookie"`
	}
	if err := environment.UnmarshalKey("bing.cookies", &raw); err != nil {
		return []BingAccount{}
	}

	result := make([]BingAccount, 0, len(raw))
	for idx, item := range raw {
		result = append(result, BingAccount{
			ID:      "bing-" + strings.ReplaceAll(uuid.NewString(), "-", ""),
			Name:    "Bing " + intToString(idx+1),
			ScopeID: strings.TrimSpace(item.ScopeID),
			IDToken: strings.TrimSpace(item.IDToken),
			Cookie:  strings.TrimSpace(item.Cookie),
			Default: idx == 0,
		})
	}
	return normalizeBingAccounts(result)
}

func bingAccountsToValues(accounts []BingAccount) []map[string]string {
	result := make([]map[string]string, 0, len(accounts))
	for _, item := range normalizeBingAccounts(accounts) {
		if item.ScopeID == "" || item.IDToken == "" || item.Cookie == "" {
			continue
		}
		result = append(result, map[string]string{
			"scopeid": item.ScopeID,
			"idtoken": item.IDToken,
			"cookie":  item.Cookie,
		})
	}
	return result
}

func extractCozeAccounts(environment *env.Environment) []CozeAccount {
	type bootstrapAccount struct {
		Email    string `mapstructure:"email"`
		Password string `mapstructure:"password"`
		Validate string `mapstructure:"validate"`
		Cookies  string `mapstructure:"cookies"`
	}

	values := make([]bootstrapAccount, 0)
	if err := environment.UnmarshalKey("coze.websdk.accounts", &values); err != nil {
		return []CozeAccount{}
	}

	result := make([]CozeAccount, 0, len(values))
	for idx, item := range values {
		result = append(result, CozeAccount{
			ID:       "coze-" + strings.ReplaceAll(uuid.NewString(), "-", ""),
			Name:     "Coze " + intToString(idx+1),
			Email:    strings.TrimSpace(item.Email),
			Password: strings.TrimSpace(item.Password),
			Validate: strings.TrimSpace(item.Validate),
			Cookies:  strings.TrimSpace(item.Cookies),
			Default:  idx == 0,
		})
	}
	return normalizeCozeAccounts(result)
}

func cozeAccountsToValues(accounts []CozeAccount) []map[string]string {
	result := make([]map[string]string, 0, len(accounts))
	for _, item := range normalizeCozeAccounts(accounts) {
		if item.Email == "" && item.Cookies == "" {
			continue
		}
		result = append(result, map[string]string{
			"email":    item.Email,
			"password": item.Password,
			"validate": item.Validate,
			"cookies":  item.Cookies,
		})
	}
	return result
}

func boolToString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func parseBoolString(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func stringify(value interface{}) string {
	switch current := value.(type) {
	case string:
		return strings.TrimSpace(current)
	default:
		return ""
	}
}

func intToString(value int) string {
	return strconv.Itoa(value)
}

func secretSuffix(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= 4 {
		return string(runes)
	}
	return string(runes[len(runes)-4:])
}
