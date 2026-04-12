package windsurf

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/iocgo/sdk/env"
	"net/http"
	"strings"
	"sync"
	"time"

	"chatgpt-adapter/core/common"
	"chatgpt-adapter/core/common/vars"
	"chatgpt-adapter/core/gin/response"
	"chatgpt-adapter/core/logger"
	"github.com/gin-gonic/gin"
	"io"
)

const (
	ginTokens = "__tokens__"
	thinkTag  = "think: "
)

type ChunkErrorWrapper struct {
	Cause struct {
		Wrapper *ChunkErrorWrapper `json:"wrapper"`
		Leaf    struct {
			Message string `json:"message"`
			Details struct {
				OriginalTypeName string `json:"originalTypeName"`
				ErrorTypeMark    struct {
					FamilyName string `json:"familyName"`
				} `json:"errorTypeMark"`
			} `json:"details"`
		} `json:"leaf"`
	} `json:"cause"`
	Message string `json:"message"`
	Details struct {
		OriginalTypeName string `json:"originalTypeName"`
		ErrorTypeMark    struct {
			FamilyName string `json:"familyName"`
		} `json:"errorTypeMark"`
		ReportablePayload []string `json:"reportablePayload"`
		FullDetails       struct {
			Type string `json:"@type"`
			Msg  string `json:"msg"`
		} `json:"fullDetails"`
	} `json:"details"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details []struct {
		Type  string `json:"type"`
		Value string `json:"value"`
		Debug struct {
			Wrapper *ChunkErrorWrapper `json:"wrapper"`
		} `json:"debug"`
	} `json:"details"`
}

type chunkError struct {
	E Error `json:"error"`
}

func (ce chunkError) Error() string {
	return ce.E.Error()
}

func (e Error) Error() string {
	message := e.Message
	if len(e.Details) > 0 {
		wrapper := e.Details[0].Debug.Wrapper
		for {
			if wrapper == nil {
				break
			}
			if wrapper.Cause.Wrapper == nil {
				break
			}
			wrapper = wrapper.Cause.Wrapper
		}
		if wrapper != nil {
			message = wrapper.Cause.Leaf.Message
		}
	}
	return fmt.Sprintf("[%s] %s", e.Code, message)
}

func waitMessage(r *http.Response, cancel func(str string) bool) (content string, err error) {
	defer r.Body.Close()
	reader := newStreamReader(r.Body)
	for {
		event, readErr := readStreamEvent(reader)
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return content, readErr
		}

		chunk := event.payload
		if len(chunk) == 0 {
			continue
		}

		if event.kind == "error" {
			if bytes.Equal(chunk, []byte("{}")) {
				break
			}
			var chunkErr chunkError
			err = json.Unmarshal(chunk, &chunkErr)
			if err == nil {
				err = &chunkErr
			}
			return
		}

		raw := string(chunk)
		logger.Debug("----- raw -----")
		logger.Debug(raw)
		if len(raw) > 0 {
			content += raw
			if cancel != nil && cancel(content) {
				return content, nil
			}
		}
	}

	return content, nil
}

func waitResponse(ctx *gin.Context, r *http.Response, sse bool) (content string, err error) {
	defer r.Body.Close()
	created := time.Now().Unix()
	logger.Info("waitResponse ...")
	completion := common.GetGinCompletion(ctx)
	matchers := common.GetGinMatchers(ctx)
	tokens := ctx.GetInt(ginTokens)
	thinkReason := env.Env.GetBool("server.think_reason")
	thinkReason = thinkReason && completion.Model[9:] == "deepseek-reasoner"
	reasoningContent := ""
	think := 0

	onceExec := sync.OnceFunc(func() {
		if !sse {
			ctx.Writer.WriteHeader(http.StatusOK)
		}
	})

	reader := newStreamReader(r.Body)
	for {
		event, readErr := readStreamEvent(reader)
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				if response.NotResponse(ctx) && !ctx.Writer.Written() {
					response.Error(ctx, -1, readErr)
				}
				return content, readErr
			}

			raw := response.ExecMatchers(matchers, "", true)
			if raw != "" && sse {
				response.SSEResponse(ctx, Model, raw, created)
			}
			content += raw
			break
		}

		chunk := event.payload
		if len(chunk) == 0 {
			continue
		}

		if event.kind == "error" {
			if bytes.Equal(chunk, []byte("{}")) {
				break
			}
			var chunkErr chunkError
			err := json.Unmarshal(chunk, &chunkErr)
			if err == nil {
				err = &chunkErr
			}

			if response.NotSSEHeader(ctx) && !ctx.Writer.Written() {
				logger.Error(err)
				response.Error(ctx, -1, err)
			}
			return content, err
		}

		raw := string(chunk)
		reasonContent := ""
		if strings.HasPrefix(raw, thinkTag) {
			reasonContent = raw[len(thinkTag):]
			raw = ""
			think = 2
			logger.Debug("----- think raw -----")
			logger.Debug(reasonContent)
			reasoningContent += reasonContent
			goto label

		} else if thinkReason && think == 0 {
			if strings.HasPrefix(raw, "<think>") {
				reasonContent = raw[7:]
				raw = ""
				think = 1
				goto label
			}
		}

		if thinkReason && think == 1 {
			reasonContent = raw
			if strings.HasPrefix(raw, "</think>") {
				reasonContent = ""
				think = 2
			}

			raw = ""
			logger.Debug("----- think raw -----")
			logger.Debug(reasonContent)
			reasoningContent += reasonContent
			goto label
		}

		logger.Debug("----- raw -----")
		logger.Debug(raw)
		onceExec()

		raw = response.ExecMatchers(matchers, raw, false)
		if len(raw) == 0 {
			continue
		}

		if raw == response.EOF {
			break
		}

	label:
		if sse {
			response.ReasonSSEResponse(ctx, Model, raw, reasonContent, created)
		}
		content += raw
	}

	if content == "" && response.NotSSEHeader(ctx) {
		return content, nil
	}

	ctx.Set(vars.GinCompletionUsage, response.CalcUsageTokens(content, tokens))
	if !sse {
		response.ReasonResponse(ctx, Model, content, reasoningContent)
	} else {
		response.SSEResponse(ctx, Model, "[DONE]", created)
	}
	return content, nil
}
