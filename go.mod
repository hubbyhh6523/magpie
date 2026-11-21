module github.com/yetone/magpie

go 1.26.8

require (
	github.com/MoeForge/moejs v0.1.0
	github.com/charmbracelet/lipgloss v1.1.1-0.20250404203927-76690c660834
	github.com/clipperhouse/uax29/v2 v2.5.0
	github.com/coder/websocket v1.8.14
	github.com/dlclark/regexp2 v1.11.5
	github.com/dop251/goja v0.0.0-20261118204651-8dfece3d1e1d
	github.com/fsnotify/fsnotify v1.9.0
	github.com/gen2brain/h265 v0.2.4
	github.com/go-ole/go-ole v1.3.0
	github.com/godbus/dbus/v5 v5.2.2
	github.com/google/uuid v1.6.0
	github.com/klauspost/compress v1.18.3
	github.com/pelletier/go-toml/v2 v2.2.4
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3
	github.com/tidwall/gjson v1.19.0
	github.com/tidwall/jsonc v0.3.3
	github.com/wailsapp/wails/v3 v3.0.0-beta.24
	golang.org/x/crypto v0.53.0
	golang.org/x/image v0.43.0
	golang.org/x/net v0.56.0
	golang.org/x/sys v0.49.0
	golang.org/x/term v0.44.0
	golang.org/x/text v0.43.0
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/libc v1.79.0
	modernc.org/sqlite v1.61.0
)

require (
	git.sr.ht/~jackmordaunt/go-toast/v2 v2.0.3 // indirect
	github.com/MoeForge/moego v0.1.0-alpha.1 // indirect
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/charmbracelet/colorprofile v0.4.1 // indirect
	github.com/charmbracelet/x/ansi v0.11.5 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.15 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.9.0 // indirect
	github.com/clipperhouse/stringish v0.1.1 // indirect
	github.com/dlclark/regexp2/v2 v2.11.0 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/google/pprof v0.0.0-20261016150608-1facf8f91b50 // indirect
	github.com/lucasb-eyer/go-colorful v1.3.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mattn/go-runewidth v0.0.19 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

replace github.com/santhosh-tekuri/jsonschema/v6 => ./third_party/jsonschema

replace gopkg.in/yaml.v3 => ./third_party/yaml.v3
