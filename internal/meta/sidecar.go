package meta

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/maskedsyntax/goggles/internal/platform"
	"gopkg.in/yaml.v3"
)

type File struct {
	Dir          string            `yaml:"-"`
	Profile      string            `yaml:"profile"`
	Instagram    PlatformFields    `yaml:"instagram"`
	YouTube      PlatformFields    `yaml:"youtube"`
	Destinations map[string]Target `yaml:"destinations"`
}

type Target struct {
	Instagram PlatformFields `yaml:"instagram"`
	YouTube   PlatformFields `yaml:"youtube"`
}

type PlatformFields struct {
	Caption      string   `yaml:"caption"`
	ShareToFeed  *bool    `yaml:"share_to_feed"`
	Title        string   `yaml:"title"`
	Description  string   `yaml:"description"`
	Tags         []string `yaml:"tags"`
	Privacy      string   `yaml:"privacy"`
	CategoryID   string   `yaml:"category_id"`
	MadeForKids  *bool    `yaml:"made_for_kids"`
	Carousel     []string `yaml:"carousel"`
	Audio        string   `yaml:"audio"`
	SlideSeconds *float64 `yaml:"slide_seconds"`
	ReplaceAudio *bool    `yaml:"replace_audio"`
}

func LoadFor(mediaPath string) (File, error) {
	candidates := sidecarCandidates(mediaPath)
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return File{}, err
		}
		var f File
		if err := yaml.Unmarshal(data, &f); err != nil {
			return File{}, err
		}
		f.Dir = sidecarDir(mediaPath, p)
		return f, nil
	}
	st, err := os.Stat(mediaPath)
	if err == nil && st.IsDir() {
		return File{Dir: mediaPath}, nil
	}
	if mediaPath != "" {
		return File{Dir: filepath.Dir(mediaPath)}, nil
	}
	return File{}, nil
}

func sidecarCandidates(mediaPath string) []string {
	st, err := os.Stat(mediaPath)
	if err == nil && st.IsDir() {
		return []string{
			mediaPath + ".yaml",
			mediaPath + ".yml",
			filepath.Join(mediaPath, "carousel.yaml"),
			filepath.Join(mediaPath, "carousel.yml"),
			filepath.Join(mediaPath, "goggles.yaml"),
			filepath.Join(mediaPath, "goggles.yml"),
		}
	}
	base := strings.TrimSuffix(mediaPath, filepath.Ext(mediaPath))
	return []string{base + ".yaml", base + ".yml"}
}

func sidecarDir(mediaPath, sidecarPath string) string {
	st, err := os.Stat(mediaPath)
	if err == nil && st.IsDir() {
		return mediaPath
	}
	return filepath.Dir(sidecarPath)
}

func (f File) For(destAlias string, p platform.Platform) (dest, plat PlatformFields) {
	plat = f.YouTube
	if p == platform.Instagram {
		plat = f.Instagram
	}
	if f.Destinations != nil {
		if t, ok := f.Destinations[destAlias]; ok {
			if p == platform.Instagram {
				dest = t.Instagram
			} else {
				dest = t.YouTube
			}
		}
	}
	return dest, plat
}
