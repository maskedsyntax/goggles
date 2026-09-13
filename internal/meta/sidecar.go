package meta

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/maskedsyntax/goggles/internal/platform"
	"gopkg.in/yaml.v3"
)

type File struct {
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
	Caption     string   `yaml:"caption"`
	ShareToFeed *bool    `yaml:"share_to_feed"`
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
	Privacy     string   `yaml:"privacy"`
	CategoryID  string   `yaml:"category_id"`
	MadeForKids *bool    `yaml:"made_for_kids"`
}

func LoadFor(mediaPath string) (File, error) {
	base := strings.TrimSuffix(mediaPath, filepath.Ext(mediaPath))
	for _, ext := range []string{".yaml", ".yml"} {
		p := base + ext
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
		return f, nil
	}
	return File{}, nil
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
