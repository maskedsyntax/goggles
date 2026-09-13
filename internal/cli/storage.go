package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/storage"
	"github.com/maskedsyntax/goggles/internal/storage/r2"
	"github.com/spf13/cobra"
)

func newStorageCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "storage", Short: "Temporary R2 hosting for Instagram uploads"}

	var accessKey, secretKey string
	creds := &cobra.Command{
		Use:   "credentials",
		Short: "Store R2 access keys in the keychain",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if accessKey == "" {
				accessKey = os.Getenv("GOGGLES_R2_ACCESS_KEY")
			}
			if secretKey == "" {
				secretKey = os.Getenv("GOGGLES_R2_SECRET_KEY")
			}
			if app.DryRun {
				return app.Out.Success(map[string]any{"dry_run": true, "stored": false})
			}
			if err := r2.StoreCredentials(app.Keychain, app.Config.R2, accessKey, secretKey); err != nil {
				return err
			}
			return app.Out.Success(map[string]any{"stored": true})
		},
	}
	creds.Flags().StringVar(&accessKey, "access-key", "", "R2 access key id")
	creds.Flags().StringVar(&secretKey, "secret-key", "", "R2 secret access key")

	cmd.AddCommand(
		creds,
		&cobra.Command{
			Use:   "status",
			Short: "Show R2 configuration and pending cleanup",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := app.openDB(); err != nil {
					return err
				}
				pending, err := storage.CountPending(cmd.Context(), app.DB)
				if err != nil {
					return err
				}
				configured := r2.Configured(app.Config) || os.Getenv("GOGGLES_STORAGE") == "memory"
				payload := map[string]any{
					"provider":            app.Config.Storage.Provider,
					"configured":          configured,
					"account_id":          app.Config.R2.AccountID,
					"bucket":              app.Config.R2.Bucket,
					"public_base_url":     app.Config.R2.PublicBaseURL,
					"endpoint":            r2Endpoint(app),
					"credentials":         r2.CredentialsPresent(app.Config.R2, app.Keychain) || os.Getenv("GOGGLES_STORAGE") == "memory",
					"cleanup_ttl_minutes": app.Config.Storage.CleanupTTLMinutes,
					"pending_cleanup":     pending,
				}
				return app.Out.Success(payload)
			},
		},
		newStorageTestCmd(app),
		&cobra.Command{
			Use:   "cleanup",
			Short: "Delete expired temporary objects",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := app.openDB(); err != nil {
					return err
				}
				host, err := app.openHost(cmd.Context())
				if err != nil {
					return err
				}
				res, err := storage.Cleanup(cmd.Context(), app.DB, host, time.Now().UTC(), app.DryRun)
				if err != nil {
					return err
				}
				return app.Out.Success(map[string]any{
					"dry_run": app.DryRun,
					"scanned": res.Scanned,
					"deleted": res.Deleted,
					"failed":  res.Failed,
					"keys":    res.Keys,
				})
			},
		},
	)
	return cmd
}

func newStorageTestCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "test [file]",
		Short: "Upload, verify, and delete a probe object",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.openDB(); err != nil {
				return err
			}
			host, err := app.openHost(cmd.Context())
			if err != nil {
				return err
			}
			path := ""
			if len(args) == 1 {
				path = args[0]
			} else {
				f, err := os.CreateTemp("", "goggles-r2-*.txt")
				if err != nil {
					return apperr.Wrap(apperr.R2UploadFailed, "cannot create probe file", err)
				}
				path = f.Name()
				defer os.Remove(path)
				if _, err := f.WriteString("goggles r2 probe\n"); err != nil {
					_ = f.Close()
					return err
				}
				_ = f.Close()
			}
			if app.DryRun {
				return app.Out.Success(map[string]any{"dry_run": true, "path": path})
			}
			hosted, err := host.Upload(cmd.Context(), path)
			if err != nil {
				return err
			}
			rec, err := storage.RecordHosted(cmd.Context(), app.DB, hosted, "")
			if err != nil {
				return err
			}
			delErr := host.Delete(cmd.Context(), hosted.ObjectKey)
			if delErr == nil {
				_ = storage.MarkDeleted(cmd.Context(), app.DB, rec.ID, time.Now().UTC())
			}
			payload := map[string]any{
				"object_key": hosted.ObjectKey,
				"public_url": hosted.PublicURL,
				"bytes":      hosted.SizeBytes,
				"expires_at": hosted.ExpiresAt.Format(time.RFC3339),
				"deleted":    delErr == nil,
			}
			if delErr != nil {
				payload["delete_error"] = delErr.Error()
				if err := app.Out.Result(map[string]any{"success": false, "error": map[string]any{
					"code": apperr.R2DeleteFailed, "message": delErr.Error(), "retryable": true,
				}, "upload": payload}); err != nil {
					return err
				}
				return apperr.Printed(delErr)
			}
			return app.Out.Success(payload)
		},
	}
}

func (a *App) openHost(ctx context.Context) (storage.VideoHost, error) {
	if os.Getenv("GOGGLES_STORAGE") == "memory" {
		base := a.Config.R2.PublicBaseURL
		if base == "" {
			base = "https://r2.test"
		}
		return storage.NewMemory(base, storage.TTL(a.Config.Storage.CleanupTTLMinutes)), nil
	}
	return r2.NewFromConfig(ctx, a.Config, a.Keychain)
}

func r2Endpoint(app *App) string {
	if app.Config.R2.Endpoint != "" {
		return app.Config.R2.Endpoint
	}
	if app.Config.R2.AccountID == "" {
		return ""
	}
	return fmt.Sprintf("https://%s.r2.cloudflarestorage.com", app.Config.R2.AccountID)
}
