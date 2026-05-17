package artifact

import (
	"bytes"
	"context"
	"io"
	"mime"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type r2Storage struct {
	client    *s3.Client
	bucket    string
	keyPrefix string
}

func newR2(ctx context.Context, projectID string, cfg Config) (*r2Storage, error) {
	accessKey, err := ResolveSecretRef(cfg.AccessKeyRef)
	if err != nil {
		return nil, err
	}
	secretKey, err := ResolveSecretRef(cfg.SecretKeyRef)
	if err != nil {
		return nil, err
	}

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithRegion("auto"),
		config.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
		config.WithResponseChecksumValidation(aws.ResponseChecksumValidationWhenRequired),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("https://" + cfg.AccountID + ".r2.cloudflarestorage.com")
	})

	return &r2Storage{
		client:    client,
		bucket:    cfg.Bucket,
		keyPrefix: cfg.Prefix + "nrflo/" + projectID + "/",
	}, nil
}

func (r *r2Storage) Put(ctx context.Context, key string, reader io.Reader) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	ct := mime.TypeByExtension(filepath.Ext(key))
	if ct == "" {
		ct = "application/octet-stream"
	}

	size := int64(len(data))
	_, err = r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(r.bucket),
		Key:           aws.String(r.keyPrefix + key),
		Body:          bytes.NewReader(data),
		ContentLength: &size,
		ContentType:   aws.String(ct),
	})
	return err
}

func (r *r2Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := r.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(r.keyPrefix + key),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (r *r2Storage) Delete(ctx context.Context, key string) error {
	_, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(r.keyPrefix + key),
	})
	return err
}
