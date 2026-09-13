package r2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/config"
	"github.com/maskedsyntax/goggles/internal/keychain"
	"github.com/maskedsyntax/goggles/internal/storage"
)

const (
	envAccess = "GOGGLES_R2_ACCESS_KEY"
	envSecret = "GOGGLES_R2_SECRET_KEY"
)

type api interface {
	PutObject(ctx context.Context, key string, body io.Reader, contentType string, size int64) error
	DeleteObject(ctx context.Context, key string) error
	HeadBucket(ctx context.Context) error
}

type Client struct {
	bucket    string
	public    string
	ttl       time.Duration
	api       api
	accountID string
	endpoint  string
}

type Options struct {
	AccountID     string
	Bucket        string
	PublicBaseURL string
	Endpoint      string
	AccessKey     string
	SecretKey     string
	TTL           time.Duration
	API           api
}

func Configured(cfg config.File) bool {
	return strings.TrimSpace(cfg.R2.AccountID) != "" &&
		strings.TrimSpace(cfg.R2.Bucket) != "" &&
		strings.TrimSpace(cfg.R2.PublicBaseURL) != ""
}

func ResolveCredentials(cfg config.R2, kc keychain.Store) (access, secret string, err error) {
	access = strings.TrimSpace(os.Getenv(envAccess))
	secret = strings.TrimSpace(os.Getenv(envSecret))
	if access != "" && secret != "" {
		return access, secret, nil
	}
	if kc == nil {
		return "", "", storageErrNoCreds()
	}
	if access == "" {
		access, err = kc.Get(refKey(cfg.AccessKeyRef, "goggles-r2-access"))
		if err != nil {
			if errors.Is(err, keychain.ErrNotFound) {
				return "", "", storageErrNoCreds()
			}
			return "", "", err
		}
	}
	if secret == "" {
		secret, err = kc.Get(refKey(cfg.SecretKeyRef, "goggles-r2-secret"))
		if err != nil {
			if errors.Is(err, keychain.ErrNotFound) {
				return "", "", storageErrNoCreds()
			}
			return "", "", err
		}
	}
	if access == "" || secret == "" {
		return "", "", storageErrNoCreds()
	}
	return access, secret, nil
}

func StoreCredentials(kc keychain.Store, cfg config.R2, access, secret string) error {
	if strings.TrimSpace(access) == "" || strings.TrimSpace(secret) == "" {
		return apperr.Invalid("access key and secret key are required")
	}
	if err := kc.Set(refKey(cfg.AccessKeyRef, "goggles-r2-access"), access); err != nil {
		return err
	}
	return kc.Set(refKey(cfg.SecretKeyRef, "goggles-r2-secret"), secret)
}

func CredentialsPresent(cfg config.R2, kc keychain.Store) bool {
	_, _, err := ResolveCredentials(cfg, kc)
	return err == nil
}

func refKey(ref, fallback string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fallback
	}
	return strings.TrimPrefix(ref, "keychain:")
}

func storageErrNoCreds() error {
	return apperr.New(apperr.AuthRequired, "R2 credentials are missing; run goggles storage credentials")
}

func New(ctx context.Context, opts Options) (*Client, error) {
	if opts.Bucket == "" || opts.AccountID == "" {
		return nil, apperr.New(apperr.ConfigMissing, "R2 account_id and bucket are required")
	}
	if opts.PublicBaseURL == "" {
		return nil, apperr.New(apperr.R2URLUnavailable, "r2.public_base_url is required")
	}
	if opts.TTL <= 0 {
		opts.TTL = storage.DefaultTTL
	}
	c := &Client{
		bucket:    opts.Bucket,
		public:    opts.PublicBaseURL,
		ttl:       opts.TTL,
		accountID: opts.AccountID,
		endpoint:  opts.Endpoint,
	}
	if opts.API != nil {
		c.api = opts.API
		return c, nil
	}
	if opts.AccessKey == "" || opts.SecretKey == "" {
		return nil, storageErrNoCreds()
	}
	endpoint := opts.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", opts.AccountID)
	}
	c.endpoint = endpoint
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(opts.AccessKey, opts.SecretKey, "")),
		awsconfig.WithRegion("auto"),
	)
	if err != nil {
		return nil, apperr.Wrap(apperr.R2UploadFailed, "cannot init R2 client", err)
	}
	s3c := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	c.api = s3API{client: s3c, bucket: opts.Bucket}
	return c, nil
}

func NewFromConfig(ctx context.Context, cfg config.File, kc keychain.Store) (*Client, error) {
	if !Configured(cfg) {
		return nil, apperr.New(apperr.ConfigMissing, "R2 is not configured (account_id, bucket, public_base_url)")
	}
	access, secret, err := ResolveCredentials(cfg.R2, kc)
	if err != nil {
		return nil, err
	}
	return New(ctx, Options{
		AccountID:     cfg.R2.AccountID,
		Bucket:        cfg.R2.Bucket,
		PublicBaseURL: cfg.R2.PublicBaseURL,
		Endpoint:      cfg.R2.Endpoint,
		AccessKey:     access,
		SecretKey:     secret,
		TTL:           storage.TTL(cfg.Storage.CleanupTTLMinutes),
	})
}

func (c *Client) Upload(ctx context.Context, path string) (*storage.HostedVideo, error) {
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, apperr.New(apperr.FileNotFound, path)
		}
		return nil, apperr.Wrap(apperr.FileUnreadable, "cannot stat file", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, apperr.Wrap(apperr.FileUnreadable, "cannot open file", err)
	}
	defer f.Close()
	key := storage.ObjectKey(path)
	pub, err := storage.JoinPublicURL(c.public, key)
	if err != nil {
		return nil, err
	}
	if err := c.api.PutObject(ctx, key, f, storage.ContentType(path), st.Size()); err != nil {
		return nil, apperr.WrapRetryable(apperr.R2UploadFailed, "R2 upload failed", err)
	}
	return &storage.HostedVideo{
		ObjectKey: key,
		PublicURL: pub,
		SizeBytes: st.Size(),
		ExpiresAt: time.Now().UTC().Add(c.TTL()),
	}, nil
}

func (c *Client) Delete(ctx context.Context, objectKey string) error {
	if err := c.api.DeleteObject(ctx, objectKey); err != nil {
		return apperr.Wrap(apperr.R2DeleteFailed, "R2 delete failed", err)
	}
	return nil
}

func (c *Client) Ping(ctx context.Context) error {
	if err := c.api.HeadBucket(ctx); err != nil {
		return apperr.Wrap(apperr.R2URLUnavailable, "cannot reach R2 bucket", err)
	}
	return nil
}

func (c *Client) TTL() time.Duration { return c.ttl }

func (c *Client) Endpoint() string { return c.endpoint }

type s3API struct {
	client *s3.Client
	bucket string
}

func (a s3API) PutObject(ctx context.Context, key string, body io.Reader, contentType string, size int64) error {
	_, err := a.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(a.bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(size),
		CacheControl:  aws.String("public, max-age=7200"),
	})
	return err
}

func (a s3API) DeleteObject(ctx context.Context, key string) error {
	_, err := a.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(key),
	})
	return err
}

func (a s3API) HeadBucket(ctx context.Context) error {
	_, err := a.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(a.bucket)})
	return err
}
