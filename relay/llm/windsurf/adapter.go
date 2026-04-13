package windsurf

import (
	"chatgpt-adapter/core/common"
	"chatgpt-adapter/core/common/vars"
	"chatgpt-adapter/core/gin/inter"
	"chatgpt-adapter/core/gin/model"
	"chatgpt-adapter/core/gin/response"
	"chatgpt-adapter/core/logger"
	"chatgpt-adapter/core/runtimecfg"
	"github.com/gin-gonic/gin"
	"github.com/iocgo/sdk/env"
	"strings"
)

var (
	Model = "windsurf"
)

type api struct {
	inter.BaseAdapter

	env *env.Environment
}

func (api *api) Match(ctx *gin.Context, model string) (ok bool, err error) {
	if !runtimecfg.Enabled(Model) {
		return
	}
	if len(model) <= 9 || Model+"/" != model[:9] {
		return
	}
	for _, mod := range listModelNames(api.env) {
		if model[9:] == mod {
			if strings.HasPrefix(mod, "deepseek") {
				completion := common.GetGinCompletion(ctx)
				completion.StopSequences = append(completion.StopSequences, "<codebase_search>", "<write_to_file>", "<open_link>")
				ctx.Set(vars.GinCompletion, completion)
			}
			ok = true
			return
		}
	}
	return
}

func (api *api) Models() (slice []model.Model) {
	if !runtimecfg.Enabled(Model) {
		return nil
	}
	for _, mod := range listModelNames(api.env) {
		slice = append(slice, model.Model{
			Id:      Model + "/" + mod,
			Object:  "model",
			Created: 1686935002,
			By:      Model + "-adapter",
		})
	}
	return
}

func (api *api) ToolChoice(ctx *gin.Context) (ok bool, err error) {
	var (
		completion = common.GetGinCompletion(ctx)
	)
	cookie, err := normalizeCredential(ctx.GetString("token"))
	if err != nil {
		return false, err
	}

	if toolChoice(ctx, api.env, cookie, completion) {
		ok = true
	}
	return
}

func (api *api) Completion(ctx *gin.Context) (err error) {
	var (
		completion = common.GetGinCompletion(ctx)
	)
	cookie, err := normalizeCredential(ctx.GetString("token"))
	if err != nil {
		return err
	}

	ctx.Set(ginTokens, promptTokenCount(completion))

	token, err := genToken(ctx.Request.Context(), api.env, api.env.GetString("server.proxied"), cookie)
	if err != nil {
		return
	}

	buffer, err := convertRequest(api.env, completion, cookie, token)
	if err != nil {
		return
	}

	r, err := fetch(ctx.Request.Context(), api.env, buffer)
	if err != nil {
		logger.Error(err)
		return
	}

	content, err := waitResponse(ctx, r, completion.Stream)
	if err != nil {
		logger.Error(err)
		if response.NotResponse(ctx) && !ctx.Writer.Written() {
			response.Error(ctx, -1, err)
		}
		return nil
	}
	if content == "" && response.NotResponse(ctx) && !ctx.Writer.Written() {
		response.Error(ctx, -1, "EMPTY RESPONSE")
	}
	return
}
