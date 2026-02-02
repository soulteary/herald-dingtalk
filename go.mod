module github.com/soulteary/herald-dingtalk

go 1.25.4

replace (
	github.com/soulteary/health-kit => ../kits/health-kit
	github.com/soulteary/logger-kit => ../kits/logger-kit
	github.com/soulteary/provider-kit => ../kits/provider-kit
)

require (
	github.com/gofiber/fiber/v2 v2.52.11
	github.com/pterm/pterm v0.12.82
	github.com/soulteary/cli-kit v1.2.1
	github.com/soulteary/health-kit v0.0.0
	github.com/soulteary/logger-kit v0.0.0
	github.com/soulteary/provider-kit v0.0.0
	github.com/soulteary/version-kit v1.0.1
)

require (
	atomicgo.dev/cursor v0.2.0 // indirect
	atomicgo.dev/keyboard v0.2.9 // indirect
	atomicgo.dev/schedule v0.1.0 // indirect
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/containerd/console v1.0.5 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gookit/color v1.5.4 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/lithammer/fuzzysearch v1.1.8 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/redis/go-redis/v9 v9.7.3 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.51.0 // indirect
	github.com/valyala/tcplisten v1.0.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	golang.org/x/sys v0.37.0 // indirect
	golang.org/x/term v0.32.0 // indirect
	golang.org/x/text v0.26.0 // indirect
)
