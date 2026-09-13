package cli

import (
	"strings"

	"github.com/maskedsyntax/goggles/internal/account"
	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/platform"
	ig "github.com/maskedsyntax/goggles/internal/platform/instagram"
	yt "github.com/maskedsyntax/goggles/internal/platform/youtube"
	"github.com/spf13/cobra"
)

func newAuthCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Authenticate with Meta and Google"}
	var alias, token, userID, username, channelID, channelTitle string
	login := &cobra.Command{
		Use:   "login <platform>",
		Short: "Store platform credentials (browser OAuth comes later)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := platform.Parse(args[0])
			if err != nil {
				return err
			}
			if strings.TrimSpace(alias) == "" {
				return apperr.Invalid("--alias is required")
			}
			if strings.TrimSpace(token) == "" {
				return apperr.Invalid("--access-token is required until browser OAuth is implemented")
			}
			if err := app.openDB(); err != nil {
				return err
			}
			in := account.AddInput{
				Platform: p,
				Alias:    alias,
				Timezone: app.Config.General.Timezone,
				Enabled:  true,
			}
			switch p {
			case platform.Instagram:
				if userID == "" {
					client := ig.NewClient(ig.GraphBase(app.Config.Meta.GraphHost, app.Config.Meta.GraphVersion), token, nil)
					id, name, err := client.Me(cmd.Context())
					if err != nil {
						return err
					}
					userID = id
					if username == "" {
						username = name
					}
				}
				in.Username = username
				in.UserID = userID
				in.DisplayName = username
			case platform.YouTube:
				if strings.TrimSpace(channelID) == "" {
					return apperr.New(apperr.YouTubeChannelRequired, "pass --channel-id; goggles never selects a YouTube channel silently")
				}
				client := yt.NewClient("", token, nil)
				chs, err := client.ListChannels(cmd.Context())
				if err != nil {
					return err
				}
				ch, ok := yt.FindChannel(chs, channelID)
				if !ok {
					return apperr.New(apperr.YouTubeChannelNotFound, channelID)
				}
				if channelTitle == "" {
					channelTitle = ch.Title
				}
				in.ChannelID = channelID
				in.ChannelTitle = channelTitle
				in.DisplayName = channelTitle
			default:
				return apperr.NotImpl(string(p) + " login")
			}
			if app.DryRun {
				return app.Out.Success(map[string]any{"dry_run": true, "alias": alias, "platform": p})
			}
			pair, err := account.Add(cmd.Context(), app.DB, in)
			if err != nil {
				return err
			}
			ref := string(p) + "/" + pair.Account.ID
			if err := app.Keychain.Set(ref, token); err != nil {
				return err
			}
			if err := account.SetCredentialRef(cmd.Context(), app.DB, pair.Account.ID, "keychain:"+ref); err != nil {
				return err
			}
			pair.Account.CredentialRef = "keychain:" + ref
			return app.Out.Success(map[string]any{
				"account":     pair.Account,
				"destination": pair.Destination,
			})
		},
	}
	login.Flags().StringVar(&alias, "alias", "", "destination alias")
	login.Flags().StringVar(&token, "access-token", "", "platform access token")
	login.Flags().StringVar(&userID, "user-id", "", "Instagram professional account id")
	login.Flags().StringVar(&username, "username", "", "Instagram username")
	login.Flags().StringVar(&channelID, "channel-id", "", "YouTube channel ID")
	login.Flags().StringVar(&channelTitle, "channel-title", "", "YouTube channel title")

	cmd.AddCommand(
		login,
		&cobra.Command{
			Use:   "status",
			Short: "Show authentication state",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := app.openDB(); err != nil {
					return err
				}
				pairs, err := account.List(cmd.Context(), app.DB)
				if err != nil {
					return err
				}
				igCount, ytCount := 0, 0
				for _, p := range pairs {
					switch p.Account.Platform {
					case platform.Instagram:
						if p.Account.CredentialRef != "" {
							igCount++
						}
					case platform.YouTube:
						if p.Account.CredentialRef != "" {
							ytCount++
						}
					}
				}
				return app.Out.Success(map[string]any{
					"instagram_accounts": igCount,
					"youtube_accounts":   ytCount,
				})
			},
		},
		&cobra.Command{Use: "logout", Short: "Remove stored credentials", Args: cobra.NoArgs, RunE: notImplemented("auth logout")},
	)
	return cmd
}
