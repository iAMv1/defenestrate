package hud

import "embed"

// icon.ico is a 16×16 solid teal square, generated once and embedded —
// no external asset file ships with the repo.
//
//go:embed icon.ico
var iconFS embed.FS

func iconBytes() []byte {
	b, err := iconFS.ReadFile("icon.ico")
	if err != nil {
		return nil
	}
	return b
}
