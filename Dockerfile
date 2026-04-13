FROM golang:1.23-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go install -ldflags="-s -w" -trimpath ./cmd/iocgo
RUN CGO_ENABLED=0 go build -toolexec /go/bin/iocgo -ldflags="-s -w" -trimpath -o /out/server ./main.go
RUN if [ -f config.yaml ]; then cp config.yaml /out/config.yaml; else printf 'server:\n  port: ${PORT}\n  password: ${PASSWORD}\n' > /out/config.yaml; fi

FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata \
    && mkdir -p /app/log /app/tmp /app/relay/llm/deepseek

COPY --from=builder /out/server ./server
COPY --from=builder /out/config.yaml ./config.yaml
COPY relay/llm/deepseek/sha3_wasm_bg.wasm ./relay/llm/deepseek/sha3_wasm_bg.wasm

ENV PORT=8080

EXPOSE 8080

CMD ["./server"]
