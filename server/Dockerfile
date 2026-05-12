# ── ステージ1: ビルド ─────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app

# go.mod / go.sum をコピー（外部依存ゼロなのでダウンロード不要）
COPY go.mod go.sum ./

# ソースをコピーしてビルド（標準ライブラリのみ → 数秒で完了）
COPY *.go pyrunner.py ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o fileserver .

# ── ステージ2: 実行イメージ ───────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache docker-cli ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/fileserver .

EXPOSE 4450
CMD ["./fileserver"]
