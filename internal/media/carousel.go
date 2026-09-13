package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/maskedsyntax/goggles/internal/apperr"
)

const (
	MinCarouselItems    = 2
	MaxCarouselItems    = 10
	DefaultSlideSeconds = 3.0
	MaxSlideSeconds     = 60.0
)

type Item struct {
	Path string
	Kind string
	Info Info
}

func ListCarouselFiles(dir string) ([]string, error) {
	st, err := os.Stat(dir)
	if err != nil {
		return nil, mapOpenError(dir, err)
	}
	if !st.IsDir() {
		return nil, apperr.Invalid(dir + " is not a directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, apperr.Wrap(apperr.FileUnreadable, "cannot read directory", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if kindForExt(e.Name()) == "" {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, filepath.Join(dir, name))
	}
	if len(out) == 0 {
		return nil, apperr.Invalid("no images or videos in " + dir)
	}
	return out, nil
}

func CollectDir(ctx context.Context, dir string) ([]Item, error) {
	paths, err := ListCarouselFiles(dir)
	if err != nil {
		return nil, err
	}
	return Collect(ctx, paths)
}

func Collect(ctx context.Context, paths []string) ([]Item, error) {
	if len(paths) == 0 {
		return nil, apperr.Invalid("no carousel items")
	}
	items := make([]Item, 0, len(paths))
	seen := map[string]struct{}{}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		info, err := Probe(ctx, abs)
		if err != nil {
			return nil, err
		}
		items = append(items, Item{Path: abs, Kind: info.Kind, Info: info})
	}
	if len(items) == 0 {
		return nil, apperr.Invalid("no carousel items")
	}
	return items, nil
}

func HashCarousel(paths []string, audio string, slideSeconds float64, replaceAudio bool) (string, error) {
	h := sha256.New()
	for _, p := range paths {
		sum, err := HashFile(p)
		if err != nil {
			return "", err
		}
		_, _ = io.WriteString(h, sum)
		_, _ = io.WriteString(h, "\n")
	}
	if strings.TrimSpace(audio) != "" {
		sum, err := HashFile(audio)
		if err != nil {
			return "", err
		}
		_, _ = io.WriteString(h, "audio:")
		_, _ = io.WriteString(h, sum)
		_, _ = io.WriteString(h, "\n")
		_, _ = io.WriteString(h, fmt.Sprintf("slide=%.3f replace=%t\n", slideSeconds, replaceAudio))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func kindForExt(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png":
		return KindImage
	case ".mp4", ".mov":
		return KindVideo
	default:
		return ""
	}
}

func IsCarousel(items []Item) bool {
	if len(items) >= MinCarouselItems {
		return true
	}
	return len(items) == 1 && items[0].Kind == KindImage
}
