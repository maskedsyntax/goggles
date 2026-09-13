package cli

import (
	"strings"
	"time"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/platform"
	"github.com/maskedsyntax/goggles/internal/publish"
	"github.com/spf13/cobra"
)

func newPublishCmd(app *App) *cobra.Command {
	var (
		profileName, dest, platforms, caption, at     string
		title, description, tags, privacy, categoryID string
		carouselDir, audio                            string
		slideSeconds                                  float64
		allowDup, shareToFeed, madeForKids            bool
		replaceAudio                                  bool
	)
	cmd := &cobra.Command{
		Use:   "publish [file...]",
		Short: "Publish a Reel, Short, or Instagram carousel",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if carouselDir == "" && len(args) == 0 {
				return apperr.Invalid("pass a file, files, or --carousel <dir>")
			}
			publishAt := ""
			if at != "" {
				ts, err := time.Parse(time.RFC3339, at)
				if err != nil {
					return apperr.Invalid("--at must be RFC3339, e.g. 2026-09-14T19:30:00+05:30")
				}
				publishAt = ts.UTC().Format(time.RFC3339)
			}
			if err := app.openDB(); err != nil {
				return err
			}
			host, err := app.openHost(cmd.Context())
			if err != nil {
				return err
			}
			var plats []platform.Platform
			if platforms != "" {
				parts, err := parsePlatformList(platforms)
				if err != nil {
					return apperr.Invalid(err.Error())
				}
				for _, p := range parts {
					parsed, err := platform.Parse(p)
					if err != nil {
						return err
					}
					plats = append(plats, parsed)
				}
			}
			runner := app.runner(host)
			var tagList []string
			for _, t := range strings.Split(tags, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					tagList = append(tagList, t)
				}
			}
			path := ""
			if len(args) > 0 {
				path = args[0]
			} else {
				path = carouselDir
			}
			res, err := runner.Run(cmd.Context(), publish.Request{
				Path:            path,
				Paths:           args,
				CarouselDir:     carouselDir,
				Audio:           audio,
				SlideSeconds:    slideSeconds,
				SlideSecondsSet: cmd.Flags().Changed("slide-seconds"),
				ReplaceAudio:    replaceAudio,
				ReplaceAudioSet: cmd.Flags().Changed("replace-audio"),
				Profile:         profileName,
				Destination:     dest,
				Platforms:       plats,
				AllowDuplicate:  allowDup,
				DryRun:          app.DryRun,
				Caption:         caption,
				ShareToFeed:     shareToFeed,
				ShareToFeedSet:  cmd.Flags().Changed("share-to-feed"),
				Title:           title,
				Description:     description,
				Tags:            tagList,
				Privacy:         privacy,
				CategoryID:      categoryID,
				MadeForKids:     madeForKids,
				MadeForKidsSet:  cmd.Flags().Changed("made-for-kids"),
				PublishAt:       publishAt,
			})
			if err != nil {
				return err
			}
			if err := app.Out.Result(res); err != nil {
				return err
			}
			if res.Partial {
				return apperr.Printed(apperr.New(apperr.Partial, "published to some destinations"))
			}
			if !res.Success {
				code := apperr.MetaRequestFailed
				msg := "publish failed"
				if len(res.Jobs) > 0 && res.Jobs[0].ErrorCode != "" {
					code = apperr.Code(res.Jobs[0].ErrorCode)
					msg = res.Jobs[0].ErrorMessage
				}
				return apperr.Printed(apperr.New(code, msg))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "publish to every destination in a profile")
	cmd.Flags().StringVar(&dest, "destination", "", "publish to one destination")
	cmd.Flags().StringVar(&platforms, "platform", "", "limit a profile publish to platforms")
	cmd.Flags().StringVar(&at, "at", "", "RFC3339 timestamp (YouTube publishAt; requires private)")
	cmd.Flags().StringVar(&caption, "caption", "", "Instagram caption")
	cmd.Flags().StringVar(&title, "title", "", "YouTube title")
	cmd.Flags().StringVar(&description, "description", "", "YouTube description")
	cmd.Flags().StringVar(&tags, "tags", "", "comma-separated YouTube tags")
	cmd.Flags().StringVar(&privacy, "privacy", "", "YouTube privacy: public, unlisted, or private")
	cmd.Flags().StringVar(&categoryID, "category-id", "", "YouTube category id")
	cmd.Flags().BoolVar(&madeForKids, "made-for-kids", false, "YouTube made for kids")
	cmd.Flags().BoolVar(&shareToFeed, "share-to-feed", false, "also share the Reel to the IG feed")
	cmd.Flags().BoolVar(&allowDup, "allow-duplicate", false, "allow posting the same hash to a destination again")
	cmd.Flags().StringVar(&carouselDir, "carousel", "", "directory of 2–10 images/videos for an Instagram carousel")
	cmd.Flags().StringVar(&audio, "audio", "", "soundtrack to bake into carousel slides (and silent videos)")
	cmd.Flags().Float64Var(&slideSeconds, "slide-seconds", 3, "duration of each still when baking --audio")
	cmd.Flags().BoolVar(&replaceAudio, "replace-audio", false, "replace audio on carousel videos that already have a soundtrack")
	return cmd
}
