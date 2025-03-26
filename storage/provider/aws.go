package storage

import (
	"bytes"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
)

type S3Provider struct {
	Client *s3.S3
}

func (p *S3Provider) PutObject(bucket, key string, content []byte) error {
	_, err := p.Client.PutObject(&s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(content),
		ContentLength: aws.Int64(int64(len(content))),
	})
	return err
}

func (p *S3Provider) GetObject(bucket, key string) ([]byte, error) {
	result, err := p.Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()
	return io.ReadAll(result.Body)
}

func (p *S3Provider) DeleteObject(bucket, key string) error {
	_, err := p.Client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	return err
}

func (p *S3Provider) HeadObject(bucket, key string) (bool, error) {
	_, err := p.Client.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (p *S3Provider) CreateSymlink(bucket, target, symlink string) error {
	// TODO(fei): better way to create symlink
	_, err := p.Client.CopyObject(&s3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		CopySource: aws.String(bucket + "/" + target),
		Key:        aws.String(symlink),
	})
	return err
}
