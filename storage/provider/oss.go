package storage

import (
	"bytes"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"io"
)

type OSSProvider struct {
	Client *oss.Client
}

func (p *OSSProvider) PutObject(bucket, key string, content []byte) error {
	b, err := p.Client.Bucket(bucket)
	if err != nil {
		return err
	}
	return b.PutObject(key, bytes.NewReader(content))
}

func (p *OSSProvider) GetObject(bucket, key string) ([]byte, error) {
	b, err := p.Client.Bucket(bucket)
	if err != nil {
		return nil, err
	}
	obj, err := b.GetObject(key)
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(obj)
}

func (p *OSSProvider) DeleteObject(bucket, key string) error {
	b, err := p.Client.Bucket(bucket)
	if err != nil {
		return err
	}
	return b.DeleteObject(key)
}

func (p *OSSProvider) HeadObject(bucket, key string) (bool, error) {
	b, err := p.Client.Bucket(bucket)
	if err != nil {
		return false, err
	}
	_, err = b.GetObjectMeta(key)
	if err != nil {
		if ossErr, ok := err.(oss.ServiceError); ok && ossErr.StatusCode == 404 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (p *OSSProvider) CreateSymlink(bucket, target, symlink string) error {
	b, err := p.Client.Bucket(bucket)
	if err != nil {
		return err
	}
	return b.PutSymlink(symlink, target)
}
