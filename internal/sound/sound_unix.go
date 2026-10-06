//go:build !windows

package sound

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// command — плеер и путь к WAV-файлу; nil, если проиграть звук нечем.
var command = sync.OnceValue(func() []string {
	var player string
	for _, p := range []string{"afplay", "paplay", "aplay"} {
		if path, err := exec.LookPath(p); err == nil {
			player = path
			break
		}
	}
	if player == "" {
		return nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil
	}
	file := filepath.Join(dir, "langswitch", "switch.wav")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return nil
	}
	if err := os.WriteFile(file, click, 0o644); err != nil {
		return nil
	}
	return []string{player, file}
})

// Play проигрывает сигнал через системный плеер (afplay на macOS, paplay/aplay в Linux),
// не задерживая вызывающего.
func Play() {
	go func() {
		cmd := command()
		if cmd == nil {
			return
		}
		_ = exec.Command(cmd[0], cmd[1:]...).Run()
	}()
}
