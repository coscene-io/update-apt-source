package storage

import (
	"fmt"
	provider "github.com/coscene-io/update-apt-source/storage/provider"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type StorageProvider interface {
	PutObject(bucket, key string, content []byte) error
	GetObject(bucket, key string) ([]byte, error)
	DeleteObject(bucket, key string) error
	HeadObject(bucket, key string) (bool, error)
	CreateSymlink(bucket, target, symlink string) error
}

func NewStorageProvider(providerType, endpoint, region, accessKey, secretKey string) (StorageProvider, error) {
	switch strings.ToLower(providerType) {
	case "oss", "aliyun":
		client, err := oss.New(endpoint, accessKey, secretKey)
		if err != nil {
			return nil, fmt.Errorf("initialize Aliyun OSS client failed: %v", err)
		}
		return &provider.OSSProvider{Client: client}, nil

	case "s3", "aws":
		sess, err := session.NewSession(&aws.Config{
			Region:      aws.String(region),
			Endpoint:    aws.String(endpoint),
			Credentials: credentials.NewStaticCredentials(accessKey, secretKey, ""),
			DisableSSL:  aws.Bool(strings.HasPrefix(endpoint, "http://")),
			//S3ForcePathStyle: aws.Bool(false),
			//S3UseAccelerate: aws.Bool(false),
		})
		if err != nil {
			return nil, fmt.Errorf("initialize AWS S3 client failed: %v", err)
		}
		return &provider.S3Provider{Client: s3.New(sess)}, nil

	default:
		return nil, fmt.Errorf("unsupported storage provider type: %s", providerType)
	}
}
