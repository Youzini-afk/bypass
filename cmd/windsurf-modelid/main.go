package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	windsurfpb "chatgpt-adapter/relay/llm/windsurf"
	"github.com/golang/protobuf/proto"
)

type frameSummary struct {
	Index        int      `json:"index"`
	Compressed   bool     `json:"compressed"`
	ModelID      uint32   `json:"modelId"`
	SchemaID     string   `json:"schemaId,omitempty"`
	Name         string   `json:"name,omitempty"`
	Title        string   `json:"title,omitempty"`
	Lang         string   `json:"lang,omitempty"`
	Version1     string   `json:"version1,omitempty"`
	Version2     string   `json:"version2,omitempty"`
	MessageCount int      `json:"messageCount"`
	Stop         []string `json:"stop,omitempty"`
}

func main() {
	var (
		inputPath string
		hexInput  string
	)

	flag.StringVar(&inputPath, "input", "", "Path to a raw Windsurf request body file captured from GetChatMessage")
	flag.StringVar(&hexInput, "hex", "", "Hex-encoded Windsurf request body")
	flag.Parse()

	if strings.TrimSpace(inputPath) == "" && strings.TrimSpace(hexInput) == "" {
		exitf("usage: go run ./cmd/windsurf-modelid -input request.bin")
	}

	data, err := loadInput(inputPath, hexInput)
	if err != nil {
		exitf("read input: %v", err)
	}

	summaries, err := decodeFrames(data)
	if err != nil {
		exitf("decode frames: %v", err)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summaries); err != nil {
		exitf("write output: %v", err)
	}
}

func loadInput(path string, hexInput string) ([]byte, error) {
	if strings.TrimSpace(hexInput) != "" {
		compact := strings.ReplaceAll(strings.TrimSpace(hexInput), " ", "")
		compact = strings.ReplaceAll(compact, "\r", "")
		compact = strings.ReplaceAll(compact, "\n", "")
		return hex.DecodeString(compact)
	}
	return os.ReadFile(path)
}

func decodeFrames(data []byte) ([]frameSummary, error) {
	summaries := make([]frameSummary, 0, 1)
	index := 0

	for len(data) > 0 {
		if len(data) < 5 {
			return nil, fmt.Errorf("truncated connect frame header at frame %d", index)
		}

		flags := data[0]
		size := int(binary.BigEndian.Uint32(data[1:5]))
		if len(data) < 5+size {
			return nil, fmt.Errorf("truncated connect frame payload at frame %d", index)
		}

		payload := data[5 : 5+size]
		data = data[5+size:]

		decoded, err := decodePayload(payload, flags&0x01 == 0x01)
		if err != nil {
			return nil, fmt.Errorf("decode frame %d payload: %w", index, err)
		}

		var message windsurfpb.ChatMessage
		if err := proto.Unmarshal(decoded, &message); err != nil {
			return nil, fmt.Errorf("unmarshal frame %d protobuf: %w", index, err)
		}

		summaries = append(summaries, frameSummary{
			Index:        index,
			Compressed:   flags&0x01 == 0x01,
			ModelID:      message.GetModel(),
			SchemaID:     message.GetSchema().GetId(),
			Name:         message.GetSchema().GetName(),
			Title:        message.GetSchema().GetTitle(),
			Lang:         message.GetSchema().GetLang(),
			Version1:     message.GetSchema().GetVersion1(),
			Version2:     message.GetSchema().GetVersion2(),
			MessageCount: len(message.GetMessages()),
			Stop:         message.GetConfig().GetStop(),
		})
		index++
	}

	return summaries, nil
}

func decodePayload(payload []byte, compressed bool) ([]byte, error) {
	if !compressed {
		return payload, nil
	}

	reader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

func exitf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
