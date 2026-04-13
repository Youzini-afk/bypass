package windsurf

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"testing"

	"chatgpt-adapter/core/gin/model"
	"chatgpt-adapter/core/gin/response"
	"chatgpt-adapter/core/windsurfmeta"
	"github.com/golang/protobuf/proto"
)

func TestMergeModelRegistryAppliesCustomOverrides(t *testing.T) {
	registry := mergeModelRegistry(map[string]string{
		"new-model":         "999",
		"gpt4o":             "110",
		"claude-3-7-sonnet": "226",
		"broken":            "abc",
	})

	if got := registry["new-model"]; got != 999 {
		t.Fatalf("expected custom model id 999, got %d", got)
	}
	if got := registry["gpt-4o"]; got != 110 {
		t.Fatalf("expected canonical override model id 110, got %d", got)
	}
	if _, ok := registry["gpt4o"]; ok {
		t.Fatalf("legacy alias should not be listed in registry")
	}
	if _, ok := registry["broken"]; ok {
		t.Fatalf("invalid model mapping should be ignored")
	}
}

func TestResolveModelIDSupportsLegacyAlias(t *testing.T) {
	if got, err := resolveModelID(nil, "gpt4o"); err != nil || got != 109 {
		t.Fatalf("expected legacy alias to resolve to builtin id 109, got %d err=%v", got, err)
	}
	if got, err := resolveModelID(nil, windsurfmeta.CanonicalName("claude-3-7-sonnet-think")); err != nil || got != 227 {
		t.Fatalf("expected canonical thinking alias to resolve to builtin id 227, got %d err=%v", got, err)
	}
}

func TestNormalizeCredentialRejectsEmpty(t *testing.T) {
	_, err := normalizeCredential("   ")
	if err == nil {
		t.Fatalf("expected empty token to fail")
	}
	if !errors.Is(err, response.UnauthorizedError) {
		t.Fatalf("expected unauthorized error, got %v", err)
	}
}

func TestMessageTextFlattensOpenAIContentArray(t *testing.T) {
	message := model.Keyv[interface{}]{
		"content": []interface{}{
			map[string]interface{}{"type": "text", "text": "alpha"},
			map[string]interface{}{"type": "image_url"},
			map[string]interface{}{"type": "text", "text": "beta"},
		},
	}

	if got := messageText(message); got != "alpha\n\nbeta" {
		t.Fatalf("unexpected message text: %q", got)
	}
}

func TestReadStreamEventDecodesCompressedMessage(t *testing.T) {
	frame := mustFrame(t, 1, mustProto(t, &ResMessage{Message: "hello"}), true)

	event, err := readStreamEvent(newStreamReader(bytes.NewReader(frame)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.kind != "message" {
		t.Fatalf("expected message event, got %q", event.kind)
	}
	if got := string(event.payload); got != "hello" {
		t.Fatalf("unexpected payload: %q", got)
	}
}

func TestReadStreamEventDecodesThinkMessage(t *testing.T) {
	frame := mustFrame(t, 1, mustProto(t, &ResMessage{Think: "plan"}), true)

	event, err := readStreamEvent(newStreamReader(bytes.NewReader(frame)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := string(event.payload); got != thinkTag+"plan" {
		t.Fatalf("unexpected think payload: %q", got)
	}
}

func TestReadStreamEventReturnsErrorPayload(t *testing.T) {
	frame := mustFrame(t, 3, []byte(`{"error":"boom"}`), false)

	event, err := readStreamEvent(newStreamReader(bytes.NewReader(frame)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.kind != "error" {
		t.Fatalf("expected error event, got %q", event.kind)
	}
	if got := string(event.payload); got != `{"error":"boom"}` {
		t.Fatalf("unexpected error payload: %q", got)
	}
}

func TestReadStreamEventDetectsTruncatedPayload(t *testing.T) {
	frame := append(int32ToBytes(1, 10), []byte("short")...)

	_, err := readStreamEvent(newStreamReader(bytes.NewReader(frame)))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected unexpected EOF, got %v", err)
	}
}

func mustProto(t *testing.T, message proto.Message) []byte {
	t.Helper()

	payload, err := proto.Marshal(message)
	if err != nil {
		t.Fatalf("marshal proto: %v", err)
	}
	return payload
}

func mustFrame(t *testing.T, magic byte, payload []byte, compress bool) []byte {
	t.Helper()

	if compress {
		var buf bytes.Buffer
		writer := gzip.NewWriter(&buf)
		if _, err := writer.Write(payload); err != nil {
			t.Fatalf("gzip write: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("gzip close: %v", err)
		}
		payload = buf.Bytes()
	}

	return append(int32ToBytes(magic, len(payload)), payload...)
}
