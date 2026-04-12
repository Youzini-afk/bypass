package windsurf

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/golang/protobuf/proto"
)

type streamEvent struct {
	kind    string
	payload []byte
}

func newStreamReader(body io.Reader) *bufio.Reader {
	return bufio.NewReaderSize(body, 256*1024)
}

func readStreamEvent(reader *bufio.Reader) (event streamEvent, err error) {
	var header [5]byte
	if _, err = io.ReadFull(reader, header[:]); err != nil {
		return event, err
	}

	chunkLen := bytesToInt32(header[1:])
	if chunkLen < 0 {
		return event, fmt.Errorf("invalid windsurf frame length %d", chunkLen)
	}

	payload := make([]byte, chunkLen)
	if _, err = io.ReadFull(reader, payload); err != nil {
		return event, err
	}

	if header[0] == 3 {
		event.kind = "error"
	} else {
		event.kind = "message"
	}
	event.payload, err = decodeStreamPayload(header[0], payload)
	return
}

func decodeStreamPayload(magic byte, payload []byte) ([]byte, error) {
	if isGzipFrame(payload) {
		reader, err := gzip.NewReader(bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		defer reader.Close()

		payload, err = io.ReadAll(reader)
		if err != nil {
			return nil, err
		}
	}

	if magic == 3 {
		return payload, nil
	}

	var message ResMessage
	if err := proto.Unmarshal(payload, &message); err != nil {
		return nil, err
	}

	if message.Think != "" {
		return append([]byte(thinkTag), []byte(message.Think)...), nil
	}
	return []byte(message.Message), nil
}

func isGzipFrame(payload []byte) bool {
	return len(payload) >= 2 && payload[0] == 0x1f && payload[1] == 0x8b
}
