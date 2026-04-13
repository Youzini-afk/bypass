package gin

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"time"

	"chatgpt-adapter/core/adminui"
	"chatgpt-adapter/core/gin/inter"
	"chatgpt-adapter/core/runtimecfg"
	windsurfadmin "chatgpt-adapter/relay/llm/windsurf"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/iocgo/sdk"
	"github.com/iocgo/sdk/env"
)

const adminCookieName = "bypass_admin_session"

type AdminHandler struct {
	env        *env.Environment
	extensions []inter.Adapter
	core       *Handler
}

type adminClaims struct {
	Sub string `json:"sub"`
	jwt.RegisteredClaims
}

type providerSummary struct {
	Name         string   `json:"name"`
	Enabled      bool     `json:"enabled"`
	ModelCount   int      `json:"modelCount"`
	AccountCount int      `json:"accountCount"`
	Models       []string `json:"models"`
}

type playgroundRequest struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	AccountID string `json:"accountId"`
	System    string `json:"system"`
	Prompt    string `json:"prompt"`
	Stream    bool   `json:"stream"`
}

type playgroundResult struct {
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Status      int    `json:"status"`
	DurationMs  int64  `json:"durationMs"`
	ContentType string `json:"contentType"`
	Content     string `json:"content,omitempty"`
	Raw         string `json:"raw"`
}

// @Inject()
func NewAdmin(container *sdk.Container, env *env.Environment) *AdminHandler {
	extensions := sdk.ListInvokeAs[inter.Adapter](container)
	core := &Handler{extensions: extensions}
	return &AdminHandler{
		env:        env,
		extensions: extensions,
		core:       core,
	}
}

func (h *AdminHandler) Mount(engine *gin.Engine) {
	adminui.Mount(engine)

	admin := engine.Group("/api/admin")
	admin.GET("/bootstrap", h.bootstrap)
	admin.POST("/auth/login", h.login)
	admin.POST("/auth/logout", h.logout)
	admin.GET("/auth/me", h.withAuth(h.me))
	admin.GET("/config", h.withAuth(h.getConfig))
	admin.PUT("/config", h.withAuth(h.putConfig))
	admin.GET("/models", h.withAuth(h.getModels))
	admin.POST("/test/provider", h.withAuth(h.testProvider))
	admin.POST("/test/windsurf", h.withAuth(h.testWindsurf))
	admin.POST("/playground/chat", h.withAuth(h.playgroundChat))
}

func (h *AdminHandler) withAuth(next gin.HandlerFunc) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !h.requireAuth(ctx) {
			return
		}
		next(ctx)
	}
}

func (h *AdminHandler) requireAuth(ctx *gin.Context) bool {
	ok, err := h.authenticated(ctx)
	if err != nil || !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"ok":    false,
			"error": "unauthorized",
		})
		return false
	}
	return true
}

func (h *AdminHandler) authenticated(ctx *gin.Context) (bool, error) {
	if h.adminPassword() == "" {
		return true, nil
	}

	cookie, err := ctx.Cookie(adminCookieName)
	if err != nil || cookie == "" {
		return false, err
	}

	token, err := jwt.ParseWithClaims(cookie, &adminClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(h.sessionSecret()), nil
	})
	if err != nil || !token.Valid {
		return false, err
	}

	claims, ok := token.Claims.(*adminClaims)
	if !ok {
		return false, errors.New("invalid claims")
	}
	return claims.Sub == "admin", nil
}

func (h *AdminHandler) bootstrap(ctx *gin.Context) {
	authenticated, _ := h.authenticated(ctx)
	ctx.JSON(http.StatusOK, gin.H{
		"ok":            true,
		"authenticated": authenticated,
		"requiresLogin": h.adminPassword() != "",
		"storage":       runtimecfg.Storage(),
		"providers":     h.providerSummaries(),
		"version":       "admin-v1",
	})
}

func (h *AdminHandler) login(ctx *gin.Context) {
	var request struct {
		Password string `json:"password"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	password := h.adminPassword()
	if password == "" {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{
			"ok":    false,
			"error": "server.password is not configured",
		})
		return
	}
	if strings.TrimSpace(request.Password) != password {
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"ok":    false,
			"error": "invalid password",
		})
		return
	}

	signed, err := h.signSession()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	h.writeSessionCookie(ctx, signed, time.Now().Add(24*time.Hour))
	ctx.JSON(http.StatusOK, gin.H{
		"ok": true,
	})
}

func (h *AdminHandler) logout(ctx *gin.Context) {
	http.SetCookie(ctx.Writer, &http.Cookie{
		Name:     adminCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.cookieSecure(ctx),
	})
	ctx.JSON(http.StatusOK, gin.H{
		"ok": true,
	})
}

func (h *AdminHandler) me(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"ok":            true,
		"authenticated": true,
		"user": gin.H{
			"role": "admin",
		},
	})
}

func (h *AdminHandler) getConfig(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"config":  runtimecfg.Current(true),
		"storage": runtimecfg.Storage(),
	})
}

func (h *AdminHandler) putConfig(ctx *gin.Context) {
	var cfg runtimecfg.RuntimeConfig
	if err := ctx.ShouldBindJSON(&cfg); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	if err := runtimecfg.Save(cfg); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"config":  runtimecfg.Current(true),
		"storage": runtimecfg.Storage(),
		"models":  h.modelsByProvider(),
	})
}

func (h *AdminHandler) getModels(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"ok":     true,
		"models": h.modelsByProvider(),
	})
}

func (h *AdminHandler) testProvider(ctx *gin.Context) {
	var request playgroundRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	if strings.TrimSpace(request.Prompt) == "" {
		request.Prompt = "Reply with READY."
	}

	result, err := h.executePlayground(request)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"ok":      false,
			"error":   err.Error(),
			"details": result,
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"details": result,
	})
}

func (h *AdminHandler) testWindsurf(ctx *gin.Context) {
	var request struct {
		Action    string `json:"action"`
		AccountID string `json:"accountId"`
		Token     string `json:"token"`
		Model     string `json:"model"`
		Prompt    string `json:"prompt"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	token := strings.TrimSpace(request.Token)
	if token == "" {
		var err error
		token, err = h.resolveProviderToken("windsurf", request.AccountID)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}
	}

	action := strings.ToLower(strings.TrimSpace(request.Action))
	switch action {
	case "", "smoke", "model":
		playground := playgroundRequest{
			Provider:  "windsurf",
			Model:     request.Model,
			Prompt:    requestPrompt(request.Prompt, "Reply with READY."),
			AccountID: request.AccountID,
		}
		result, err := h.executePlaygroundWithToken(playground, token)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"ok":      false,
				"error":   err.Error(),
				"details": result,
			})
			return
		}
		ctx.JSON(http.StatusOK, gin.H{
			"ok":      true,
			"details": result,
		})
	case "validate":
		if err := windsurfadmin.AdminValidateToken(token); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}
		ctx.JSON(http.StatusOK, gin.H{
			"ok": true,
			"details": gin.H{
				"message": "token format is valid",
			},
		})
	case "jwt":
		value, err := windsurfadmin.AdminFetchJWT(ctx.Request.Context(), h.env, token)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}
		preview := value
		if len(preview) > 12 {
			preview = preview[:12] + "..."
		}
		ctx.JSON(http.StatusOK, gin.H{
			"ok": true,
			"details": gin.H{
				"length":  len(value),
				"preview": preview,
			},
		})
	default:
		ctx.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": "unsupported action",
		})
	}
}

func (h *AdminHandler) playgroundChat(ctx *gin.Context) {
	var request playgroundRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	result, err := h.executePlayground(request)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"ok":      false,
			"error":   err.Error(),
			"details": result,
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"details": result,
	})
}

func (h *AdminHandler) executePlayground(request playgroundRequest) (playgroundResult, error) {
	provider := providerName(request.Provider, request.Model)
	token, err := h.resolveProviderToken(provider, request.AccountID)
	if err != nil {
		return playgroundResult{}, err
	}
	return h.executePlaygroundWithToken(request, token)
}

func (h *AdminHandler) executePlaygroundWithToken(request playgroundRequest, authToken string) (playgroundResult, error) {
	provider := providerName(request.Provider, request.Model)
	model := strings.TrimSpace(request.Model)
	if model == "" {
		model = h.defaultModelFor(provider)
	}
	if model == "" {
		return playgroundResult{}, errors.New("no model available for provider")
	}

	payload := map[string]interface{}{
		"model":  model,
		"stream": request.Stream,
		"messages": buildMessages(
			request.System,
			requestPrompt(request.Prompt, "Hello"),
		),
	}

	recorder := httptest.NewRecorder()
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = req
	token(ctx)
	ctx.Set("proxies", h.env.GetString("server.proxied"))

	started := time.Now()
	h.core.completions(ctx)

	resp := recorder.Result()
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	result := playgroundResult{
		Provider:    provider,
		Model:       model,
		Status:      resp.StatusCode,
		DurationMs:  time.Since(started).Milliseconds(),
		ContentType: resp.Header.Get("Content-Type"),
		Raw:         string(raw),
		Content:     extractContent(string(raw)),
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return result, errors.New(resp.Status)
	}
	return result, nil
}

func (h *AdminHandler) resolveProviderToken(provider string, accountID string) (string, error) {
	cfg := runtimecfg.Current(false)

	switch provider {
	case "windsurf":
		return selectSecret(cfg.Windsurf.Accounts, accountID)
	case "cursor":
		return selectSecret(cfg.Providers.Cursor.Accounts, accountID)
	case "deepseek":
		return selectSecret(cfg.Providers.Deepseek.Accounts, accountID)
	case "qodo":
		return "", nil
	case "blackbox":
		return "", nil
	case "you", "bing", "coze", "lmsys", "grok":
		return strings.TrimSpace(h.adminPassword()), nil
	default:
		if provider == "" {
			return "", nil
		}
		return "", errors.New("unsupported provider token source")
	}
}

func (h *AdminHandler) defaultModelFor(provider string) string {
	models := h.modelsByProvider()[provider]
	if len(models) == 0 {
		return ""
	}
	return models[0]
}

func (h *AdminHandler) providerSummaries() []providerSummary {
	cfg := runtimecfg.Current(false)
	modelMap := h.modelsByProvider()
	summaries := []providerSummary{
		{Name: "windsurf", Enabled: cfg.Windsurf.Enabled, AccountCount: len(cfg.Windsurf.Accounts), Models: modelMap["windsurf"]},
		{Name: "cursor", Enabled: cfg.Providers.Cursor.Enabled, AccountCount: len(cfg.Providers.Cursor.Accounts), Models: modelMap["cursor"]},
		{Name: "deepseek", Enabled: cfg.Providers.Deepseek.Enabled, AccountCount: len(cfg.Providers.Deepseek.Accounts), Models: modelMap["deepseek"]},
		{Name: "qodo", Enabled: cfg.Providers.Qodo.Enabled, AccountCount: len(cfg.Providers.Qodo.Accounts), Models: modelMap["qodo"]},
		{Name: "lmsys", Enabled: cfg.Providers.Lmsys.Enabled, AccountCount: len(cfg.Providers.Lmsys.Accounts), Models: modelMap["lmsys"]},
		{Name: "blackbox", Enabled: cfg.Providers.Blackbox.Enabled, AccountCount: len(cfg.Providers.Blackbox.Accounts), Models: modelMap["blackbox"]},
		{Name: "you", Enabled: cfg.Providers.You.Enabled, AccountCount: len(cfg.Providers.You.Accounts), Models: modelMap["you"]},
		{Name: "grok", Enabled: cfg.Providers.Grok.Enabled, AccountCount: len(cfg.Providers.Grok.Accounts), Models: modelMap["grok"]},
		{Name: "bing", Enabled: cfg.Providers.Bing.Enabled, AccountCount: len(cfg.Providers.Bing.Accounts), Models: modelMap["bing"]},
		{Name: "coze", Enabled: cfg.Providers.Coze.Enabled, AccountCount: len(cfg.Providers.Coze.Accounts), Models: modelMap["coze"]},
	}

	result := make([]providerSummary, 0, len(summaries))
	for _, item := range summaries {
		item.ModelCount = len(item.Models)
		result = append(result, item)
	}
	return result
}

func (h *AdminHandler) modelsByProvider() map[string][]string {
	result := make(map[string][]string)
	for _, extension := range h.extensions {
		for _, item := range extension.Models() {
			provider := providerName("", item.Id)
			if provider == "" {
				continue
			}
			result[provider] = append(result[provider], item.Id)
		}
	}

	for provider := range result {
		sort.Strings(result[provider])
	}
	return result
}

func (h *AdminHandler) adminPassword() string {
	value := strings.TrimSpace(h.env.GetString("server.password"))
	if value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("PASSWORD"))
}

func (h *AdminHandler) signSession() (string, error) {
	claims := adminClaims{
		Sub: "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.sessionSecret()))
}

func (h *AdminHandler) sessionSecret() string {
	secret := h.adminPassword()
	if secret == "" {
		secret = "bypass-admin"
	}
	return "bypass-admin:" + secret
}

func (h *AdminHandler) writeSessionCookie(ctx *gin.Context, value string, expires time.Time) {
	http.SetCookie(ctx.Writer, &http.Cookie{
		Name:     adminCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.cookieSecure(ctx),
	})
}

func (h *AdminHandler) cookieSecure(ctx *gin.Context) bool {
	if ctx.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(ctx.GetHeader("X-Forwarded-Proto"), "https")
}

func buildMessages(system string, prompt string) []map[string]string {
	messages := make([]map[string]string, 0, 2)
	if strings.TrimSpace(system) != "" {
		messages = append(messages, map[string]string{
			"role":    "system",
			"content": strings.TrimSpace(system),
		})
	}
	messages = append(messages, map[string]string{
		"role":    "user",
		"content": strings.TrimSpace(prompt),
	})
	return messages
}

func requestPrompt(prompt string, fallback string) string {
	if strings.TrimSpace(prompt) == "" {
		return fallback
	}
	return prompt
}

func selectSecret(accounts []runtimecfg.SecretAccount, accountID string) (string, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID != "" {
		for _, item := range accounts {
			if item.ID == accountID && strings.TrimSpace(item.Secret) != "" {
				return strings.TrimSpace(item.Secret), nil
			}
		}
	}
	for _, item := range accounts {
		if item.Default && strings.TrimSpace(item.Secret) != "" {
			return strings.TrimSpace(item.Secret), nil
		}
	}
	for _, item := range accounts {
		if strings.TrimSpace(item.Secret) != "" {
			return strings.TrimSpace(item.Secret), nil
		}
	}
	return "", errors.New("no credential configured")
}

func providerName(provider string, model string) string {
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider != "" {
		return provider
	}
	model = strings.TrimSpace(strings.ToLower(model))
	switch {
	case strings.HasPrefix(model, "windsurf/"):
		return "windsurf"
	case strings.HasPrefix(model, "cursor/"):
		return "cursor"
	case strings.HasPrefix(model, "qodo/"):
		return "qodo"
	case strings.HasPrefix(model, "coze"):
		return "coze"
	case strings.HasPrefix(model, "you/"):
		return "you"
	case strings.HasPrefix(model, "lmsys-chat/"):
		return "lmsys-chat"
	case strings.HasPrefix(model, "lmsys/"):
		return "lmsys"
	case strings.HasPrefix(model, "blackbox/"):
		return "blackbox"
	case strings.HasPrefix(model, "deepseek-"):
		return "deepseek"
	case strings.HasPrefix(model, "grok-"):
		return "grok"
	case strings.HasPrefix(model, "bing"):
		return "bing"
	default:
		return provider
	}
}

func extractContent(raw string) string {
	var payload map[string]interface{}
	if json.Unmarshal([]byte(raw), &payload) != nil {
		return ""
	}

	if errorValue, ok := payload["error"]; ok {
		buffer, _ := json.Marshal(errorValue)
		return string(buffer)
	}

	choices, ok := payload["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return ""
	}

	first, ok := choices[0].(map[string]interface{})
	if !ok {
		return ""
	}

	if message, ok := first["message"].(map[string]interface{}); ok {
		if content, ok := message["content"].(string); ok {
			return content
		}
	}

	if delta, ok := first["delta"].(map[string]interface{}); ok {
		if content, ok := delta["content"].(string); ok {
			return content
		}
	}
	return ""
}
