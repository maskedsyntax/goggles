package cli

import (
	"fmt"
	"strings"

	"github.com/maskedsyntax/goggles/internal/account"
	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/platform"
	yt "github.com/maskedsyntax/goggles/internal/platform/youtube"
	"github.com/spf13/cobra"
)

func newAccountCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "account", Short: "Connect Instagram accounts and YouTube channels"}

	var (
		alias, username, channelID, channelTitle, timezone, displayName, accessToken, userID string
		disabled                                                                             bool
	)
	add := &cobra.Command{
		Use:   "add <platform>",
		Short: "Add a local account and destination (OAuth comes in a later phase)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := platform.Parse(args[0])
			if err != nil {
				return err
			}
			if alias == "" {
				return apperr.Invalid("--alias is required")
			}
			if err := app.openDB(); err != nil {
				return err
			}
			if app.DryRun {
				return app.Out.Success(map[string]any{
					"dry_run":    true,
					"platform":   p,
					"alias":      alias,
					"channel_id": channelID,
					"username":   username,
				})
			}
			pair, err := account.Add(cmd.Context(), app.DB, account.AddInput{
				Platform:     p,
				Alias:        alias,
				DisplayName:  displayName,
				Username:     username,
				ChannelID:    channelID,
				ChannelTitle: channelTitle,
				UserID:       userID,
				Timezone:     firstNonEmpty(timezone, app.Config.General.Timezone),
				Enabled:      !disabled,
			})
			if err != nil {
				return err
			}
			if accessToken != "" {
				ref := string(p) + "/" + pair.Account.ID
				if err := app.Keychain.Set(ref, accessToken); err != nil {
					return err
				}
				if err := account.SetCredentialRef(cmd.Context(), app.DB, pair.Account.ID, "keychain:"+ref); err != nil {
					return err
				}
				pair.Account.CredentialRef = "keychain:" + ref
			}
			return app.Out.Success(map[string]any{
				"account":     pair.Account,
				"destination": pair.Destination,
			})
		},
	}
	add.Flags().StringVar(&alias, "alias", "", "destination alias")
	add.Flags().StringVar(&username, "username", "", "Instagram username")
	add.Flags().StringVar(&channelID, "channel-id", "", "YouTube channel ID")
	add.Flags().StringVar(&channelTitle, "channel-title", "", "YouTube channel title")
	add.Flags().StringVar(&displayName, "display-name", "", "human-readable name")
	add.Flags().StringVar(&timezone, "timezone", "", "destination timezone")
	add.Flags().BoolVar(&disabled, "disabled", false, "create disabled")
	add.Flags().StringVar(&accessToken, "access-token", "", "store a platform access token in the keychain")
	add.Flags().StringVar(&userID, "user-id", "", "Instagram professional account id")

	cmd.AddCommand(
		add,
		&cobra.Command{
			Use:   "list",
			Short: "List connected accounts",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := app.openDB(); err != nil {
					return err
				}
				pairs, err := account.List(cmd.Context(), app.DB)
				if err != nil {
					return err
				}
				if app.JSON {
					return app.Out.Success(map[string]any{"accounts": pairs})
				}
				rows := make([][]string, 0, len(pairs))
				for _, p := range pairs {
					rows = append(rows, []string{
						p.Account.Alias,
						string(p.Account.Platform),
						fmt.Sprintf("%v", p.Account.Enabled),
						externalLabel(p.Destination),
					})
				}
				return app.Out.Table([]string{"ALIAS", "PLATFORM", "ENABLED", "EXTERNAL"}, rows)
			},
		},
		&cobra.Command{
			Use:   "info <alias>",
			Short: "Show one account and its destination",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := app.openDB(); err != nil {
					return err
				}
				pair, err := account.GetByAlias(cmd.Context(), app.DB, args[0])
				if err != nil {
					return err
				}
				return app.Out.Success(map[string]any{"account": pair.Account, "destination": pair.Destination})
			},
		},
		&cobra.Command{
			Use:   "remove <alias>",
			Short: "Remove an account and its destination",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := app.openDB(); err != nil {
					return err
				}
				if app.DryRun {
					return app.Out.Success(map[string]any{"dry_run": true, "alias": args[0]})
				}
				if err := account.Remove(cmd.Context(), app.DB, args[0]); err != nil {
					return err
				}
				return app.Out.Success(map[string]any{"removed": args[0]})
			},
		},
		&cobra.Command{
			Use:   "test <alias>",
			Short: "Test live credentials for an account",
			Args:  cobra.ExactArgs(1),
			RunE:  notImplemented("account test"),
		},
		newAccountChannelsCmd(app),
	)
	return cmd
}

func externalLabel(d account.Destination) string {
	if d.ExternalUsername != "" {
		return "@" + d.ExternalUsername
	}
	if d.ExternalTitle != "" && d.ExternalID != "" {
		return d.ExternalTitle + " (" + d.ExternalID + ")"
	}
	if d.ExternalID != "" {
		return d.ExternalID
	}
	return "-"
}

func newAccountChannelsCmd(app *App) *cobra.Command {
	var token, alias string
	cmd := &cobra.Command{
		Use:   "channels <platform>",
		Short: "List YouTube channels for a Google identity",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := platform.Parse(args[0])
			if err != nil {
				return err
			}
			if p != platform.YouTube {
				return apperr.Invalid("channel listing is only supported for youtube")
			}
			if token == "" && alias != "" {
				if err := app.openDB(); err != nil {
					return err
				}
				pair, err := account.GetByAlias(cmd.Context(), app.DB, alias)
				if err != nil {
					return err
				}
				token, err = app.Keychain.Get("youtube/" + pair.Account.ID)
				if err != nil {
					return apperr.New(apperr.AuthRequired, "YouTube token missing for "+alias)
				}
			}
			if strings.TrimSpace(token) == "" {
				return apperr.Invalid("pass --access-token or --alias")
			}
			client := yt.NewClient("", token, nil)
			chs, err := client.ListChannels(cmd.Context())
			if err != nil {
				return err
			}
			if app.JSON {
				return app.Out.Success(map[string]any{"channels": chs})
			}
			rows := make([][]string, 0, len(chs))
			for _, ch := range chs {
				rows = append(rows, []string{ch.ID, ch.Title})
			}
			return app.Out.Table([]string{"CHANNEL ID", "TITLE"}, rows)
		},
	}
	cmd.Flags().StringVar(&token, "access-token", "", "Google OAuth access token")
	cmd.Flags().StringVar(&alias, "alias", "", "existing YouTube account alias")
	return cmd
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
