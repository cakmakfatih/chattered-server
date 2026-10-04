package storage

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

const profilePhotoUploadLifetime = 10 * time.Minute

type TigrisProfilePhotoStore struct {
	bucket    string
	client    *s3.Client
	presigner *s3.PresignClient
}

func NewTigrisProfilePhotoStore(ctx context.Context, endpoint, bucket, region, accessKey, secretKey string) (*TigrisProfilePhotoStore, error) {
	if strings.TrimSpace(endpoint) == "" || strings.TrimSpace(bucket) == "" || strings.TrimSpace(region) == "" || strings.TrimSpace(accessKey) == "" || strings.TrimSpace(secretKey) == "" {
		return nil, errors.New("Tigris endpoint, bucket, region, and credentials are required")
	}

	configuration, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(configuration, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
	})
	return &TigrisProfilePhotoStore{
		bucket:    bucket,
		client:    client,
		presigner: s3.NewPresignClient(client),
	}, nil
}

func (store *TigrisProfilePhotoStore) AuthorizeUpload(ctx context.Context, objectKey, mimeType string, sizeBytes int64) (string, time.Time, error) {
	expiresAt := time.Now().UTC().Add(profilePhotoUploadLifetime)
	request, err := store.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(store.bucket),
		Key:           aws.String(objectKey),
		ContentType:   aws.String(mimeType),
		ContentLength: aws.Int64(sizeBytes),
	}, func(options *s3.PresignOptions) {
		options.Expires = profilePhotoUploadLifetime
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return request.URL, expiresAt, nil
}

func (store *TigrisProfilePhotoStore) OpenObject(ctx context.Context, objectKey string) (io.ReadCloser, string, int64, error) {
	result, err := store.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(store.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		if objectDoesNotExist(err) {
			return nil, "", 0, fs.ErrNotExist
		}
		return nil, "", 0, err
	}
	contentType := aws.ToString(result.ContentType)
	contentLength := aws.ToInt64(result.ContentLength)
	return result.Body, contentType, contentLength, nil
}

func objectDoesNotExist(err error) bool {
	var apiError smithy.APIError
	if errors.As(err, &apiError) {
		return apiError.ErrorCode() == "NoSuchKey" || apiError.ErrorCode() == "NotFound"
	}
	return false
}
