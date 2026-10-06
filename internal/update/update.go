// Package update проверяет наличие новой версии в GitHub Releases.
package update

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Repo — репозиторий проекта на GitHub.
const Repo = "nailspb/langswitch"

// Release — опубликованная версия.
type Release struct {
	Version string `json:"tag_name"` // например "v1.2.0"
	URL     string `json:"html_url"` // страница релиза
}

// Latest возвращает последний опубликованный релиз.
func Latest(ctx context.Context) (Release, error) {
	url := "https://api.github.com/repos/" + Repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "langswitch")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub: %s", resp.Status)
	}
	var r Release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return Release{}, fmt.Errorf("GitHub: %w", err)
	}
	return r, nil
}

// Newer сообщает, что версия latest новее current. Версии вида v1.2.3;
// если current не такая (например, "dev"), обновление не предлагается.
func Newer(latest, current string) bool {
	l, ok1 := parse(latest)
	c, ok2 := parse(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := range l {
		if r := cmp.Compare(l[i], c[i]); r != 0 {
			return r > 0
		}
	}
	return false
}

// parse разбирает "v1.2.3" (суффикс вроде "-rc1" или "-5-gabc" отбрасывается).
func parse(v string) ([3]int, bool) {
	var out [3]int
	v, _, _ = strings.Cut(strings.TrimPrefix(v, "v"), "-")
	parts := strings.Split(v, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
