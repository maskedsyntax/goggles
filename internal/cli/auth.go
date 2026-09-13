package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/maskedsyntax/goggles/internal/account"
	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/config"
	"github.com/maskedsyntax/goggles/internal/keychain"
	"github.com/maskedsyntax/goggles/internal/oauth"
	"github.com/maskedsyntax/goggles/internal/platform"
	ig "github.com/maskedsyntax/goggles/internal/platform/instagram"
	yt "github.com/maskedsyntax/goggles/internal/platform/youtube"
	"github.com/spf13/cobra"
)

func newAuthCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Authenticate with Meta and Google"}
	var alias, token, userID, username, channelID, channelTitle string
	var noBrowser bool
	var port int
	login := &cobra.Command{
		Use:   "login <platform>",
		Short: "Log in via browser (or --access-token)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := platform.Parse(args[0])
			if err != nil {
				return err
			}
			if strings.TrimSpace(alias) == "" {
				return apperr.Invalid("--alias is required")
			}
			if err := app.openDB(); err != nil {
				return err
			}
			var expires time.Time
			if strings.TrimSpace(token) == "" {
				if app.NonInteractive {
					return apperr.New(apperr.AuthRequired, "pass --access-token in non-interactive mode")
				}
				token, userID, username, expires, err = app.browserLogin(cmd, p, noBrowser, port)
				if err != nil {
					return err
				}
			}
			in := account.AddInput{
				Platform: p,
				Alias:    alias,
				Timezone: app.Config.General.Timezone,
				Enabled:  true,
			}
			var refresh string
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
				if strings.TrimSpace(channelID) == "" && cmd.Flags().Changed("access-token") {
					return apperr.New(apperr.YouTubeChannelRequired, "pass --channel-id; goggles never selects a YouTube channel silently")
				}
				parts := strings.SplitN(token, "\n", 2)
				token = parts[0]
				if len(parts) == 2 {
					refresh = parts[1]
				}
				client := yt.NewClient("", token, nil)
				chs, err := client.ListChannels(cmd.Context())
				if err != nil {
					return err
				}
				if strings.TrimSpace(channelID) == "" {
					if app.JSON || app.NonInteractive {
						err := apperr.New(apperr.YouTubeChannelRequired, "pass --channel-id; goggles never selects a YouTube channel silently")
						err.Details = map[string]any{"channels": chs}
						return err
					}
					_ = app.Out.Table([]string{"CHANNEL ID", "TITLE"}, channelRows(chs))
					return apperr.New(apperr.YouTubeChannelRequired, "re-run with --channel-id and --access-token, or run login again with --channel-id")
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
			if refresh != "" {
				_ = app.Keychain.Set(ref+"/refresh", refresh)
			}
			if !expires.IsZero() {
				_ = app.Keychain.Set(ref+"/expires", expires.UTC().Format(time.RFC3339))
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
	login.Flags().StringVar(&token, "access-token", "", "skip the browser and store this token")
	login.Flags().StringVar(&userID, "user-id", "", "Instagram professional account id")
	login.Flags().StringVar(&username, "username", "", "Instagram username")
	login.Flags().StringVar(&channelID, "channel-id", "", "YouTube channel ID")
	login.Flags().StringVar(&channelTitle, "channel-title", "", "YouTube channel title")
	login.Flags().BoolVar(&noBrowser, "no-browser", false, "print the URL instead of opening a browser")
	login.Flags().IntVar(&port, "port", 0, "loopback port (default 8787)")

	cmd.AddCommand(login, newAuthSetupCmd(app), newAuthStatusCmd(app), newAuthLogoutCmd(app))
	return cmd
}

func newAuthSetupCmd(app *App) *cobra.Command {
	var appID, appSecret, clientID, clientSecret string
	cmd := &cobra.Command{
		Use:   "setup <platform>",
		Short: "Store OAuth app id and secret for browser login",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := platform.Parse(args[0])
			if err != nil {
				return err
			}
			cfg := app.Config
			switch p {
			case platform.Instagram:
				if appID == "" || appSecret == "" {
					return apperr.Invalid("--app-id and --app-secret are required")
				}
				if err := cfg.Set("meta.app_id", appID); err != nil {
					return err
				}
				if err := keychain.SetRef(app.Keychain, cfg.Meta.AppSecretRef, "goggles-meta-app-secret", appSecret); err != nil {
					return err
				}
			case platform.YouTube:
				if clientID == "" || clientSecret == "" {
					return apperr.Invalid("--client-id and --client-secret are required")
				}
				if err := cfg.Set("google.client_id", clientID); err != nil {
					return err
				}
				if err := keychain.SetRef(app.Keychain, cfg.Google.ClientSecretRef, "goggles-google-client-secret", clientSecret); err != nil {
					return err
				}
			default:
				return apperr.NotImpl(string(p) + " setup")
			}
			if err := config.Write(app.Paths.ConfigFile, cfg); err != nil {
				return err
			}
			app.Config = cfg
			return app.Out.Success(map[string]any{"platform": p, "stored": true})
		},
	}
	cmd.Flags().StringVar(&appID, "app-id", "", "Instagram app id")
	cmd.Flags().StringVar(&appSecret, "app-secret", "", "Instagram app secret")
	cmd.Flags().StringVar(&clientID, "client-id", "", "Google OAuth client id")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "Google OAuth client secret")
	return cmd
}

func newAuthStatusCmd(app *App) *cobra.Command {
	return &cobra.Command{
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
				"instagram_app":      app.Config.Meta.AppID != "",
				"google_client":      app.Config.Google.ClientID != "",
			})
		},
	}
}

func newAuthLogoutCmd(app *App) *cobra.Command {
	var alias string
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove stored tokens for an alias",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if alias == "" {
				return apperr.Invalid("--alias is required")
			}
			if err := app.openDB(); err != nil {
				return err
			}
			pair, err := account.GetByAlias(cmd.Context(), app.DB, alias)
			if err != nil {
				return err
			}
			ref := string(pair.Account.Platform) + "/" + pair.Account.ID
			_ = app.Keychain.Delete(ref)
			_ = app.Keychain.Delete(ref + "/refresh")
			_ = app.Keychain.Delete(ref + "/expires")
			if err := account.SetCredentialRef(cmd.Context(), app.DB, pair.Account.ID, ""); err != nil {
				return err
			}
			return app.Out.Success(map[string]any{"logged_out": alias})
		},
	}
	cmd.Flags().StringVar(&alias, "alias", "", "account alias")
	return cmd
}

func (a *App) browserLogin(cmd *cobra.Command, p platform.Platform, noBrowser bool, port int) (token, userID, username string, expires time.Time, err error) {
	if port == 0 {
		port = a.Config.Meta.OAuthPort
	}
	if port == 0 {
		port = 8787
	}
	state := oauth.NewState()
	sess, err := oauth.Listen(port, state)
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	defer sess.Close()
	redirect := sess.Redirect

	var authURL string
	var ytPKCE yt.PKCE
	switch p {
	case platform.Instagram:
		if a.Config.Meta.AppID == "" {
			return "", "", "", time.Time{}, apperr.New(apperr.ConfigMissing, "set Instagram app id via goggles auth setup instagram")
		}
		secret, err := keychain.GetRef(a.Keychain, a.Config.Meta.AppSecretRef, "goggles-meta-app-secret", "GOGGLES_META_APP_SECRET")
		if err != nil {
			return "", "", "", time.Time{}, err
		}
		authURL = ig.AuthorizeURLWith(a.Config.Meta.AppID, redirect, state)
		_ = secret // validated above
	case platform.YouTube:
		if a.Config.Google.ClientID == "" {
			return "", "", "", time.Time{}, apperr.New(apperr.ConfigMissing, "set Google client id via goggles auth setup youtube")
		}
		ytPKCE = yt.NewPKCE()
		authURL = yt.AuthorizeURLWith(a.Config.Google.ClientID, redirect, state, ytPKCE.Challenge)
	default:
		return "", "", "", time.Time{}, apperr.NotImpl(string(p) + " browser login")
	}

	if noBrowser || a.JSON {
		if a.JSON {
			_ = a.Out.Result(map[string]any{"authorization_url": authURL, "redirect_uri": redirect, "waiting": true})
		} else {
			fmt.Fprintln(a.Stderr, "Open this URL in a browser:")
			fmt.Fprintln(a.Stderr, authURL)
		}
	} else {
		fmt.Fprintln(a.Stderr, "Opening browser for", p, "login…")
		fmt.Fprintln(a.Stderr, "Redirect URI:", redirect)
		if err := oauth.OpenBrowser(authURL); err != nil {
			fmt.Fprintln(a.Stderr, "Could not open browser. Open this URL:")
			fmt.Fprintln(a.Stderr, authURL)
		}
	}

	code, err := sess.Wait(cmd.Context(), 5*time.Minute)
	if err != nil {
		return "", "", "", time.Time{}, err
	}

	switch p {
	case platform.Instagram:
		secret, err := keychain.GetRef(a.Keychain, a.Config.Meta.AppSecretRef, "goggles-meta-app-secret", "GOGGLES_META_APP_SECRET")
		if err != nil {
			return "", "", "", time.Time{}, err
		}
		tok, err := ig.Exchange(cmd.Context(), nil, a.Config.Meta.AppID, secret, redirect, code)
		if err != nil {
			return "", "", "", time.Time{}, err
		}
		exp := time.Time{}
		if long, err := ig.ExchangeLongLived(cmd.Context(), nil, secret, tok.AccessToken); err == nil && long.AccessToken != "" {
			tok.AccessToken = long.AccessToken
			if long.ExpiresIn > 0 {
				exp = time.Now().Add(time.Duration(long.ExpiresIn) * time.Second)
			}
		}
		return tok.AccessToken, tok.UserID, "", exp, nil
	case platform.YouTube:
		secret, err := keychain.GetRef(a.Keychain, a.Config.Google.ClientSecretRef, "goggles-google-client-secret", "GOGGLES_GOOGLE_CLIENT_SECRET")
		if err != nil {
			return "", "", "", time.Time{}, err
		}
		tok, err := yt.Exchange(cmd.Context(), nil, a.Config.Google.ClientID, secret, redirect, code, ytPKCE.Verifier)
		if err != nil {
			return "", "", "", time.Time{}, err
		}
		packed := tok.AccessToken
		if tok.RefreshToken != "" {
			packed = tok.AccessToken + "\n" + tok.RefreshToken
		}
		exp := time.Time{}
		if tok.ExpiresIn > 0 {
			exp = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
		}
		return packed, "", "", exp, nil
	default:
		return "", "", "", time.Time{}, apperr.NotImpl(string(p) + " browser login")
	}
}

func channelRows(chs []yt.Channel) [][]string {
	rows := make([][]string, 0, len(chs))
	for _, ch := range chs {
		rows = append(rows, []string{ch.ID, ch.Title})
	}
	return rows
}
