package config

import (
	"fmt"
	"slices"
)

var validUbuntuDistro = []string{
	"all",
	"bionic",
	"focal",
	"jammy",
	"noble",
	"trusty",
}

var validStorageTypes = []string{
	"s3",
	"aws",
	"oss",
	"aliyun",
}

type Config struct {
	UbuntuDistro    string
	DebPaths        []string
	Architectures   []string
	StorageType     string
	Endpoint        string
	Region          string
	BucketName      string
	AccessKeyId     string
	AccessKeySecret string
	GpgPrivateKey   []byte
}

func (c *Config) IsValid() error {
	if !slices.Contains(validUbuntuDistro, c.UbuntuDistro) {
		return fmt.Errorf("ubuntu distribution is not valid: %s", c.UbuntuDistro)
	}
	if c.DebPaths == nil {
		return fmt.Errorf("deb paths is required: %s", c.DebPaths)
	}
	if len(c.DebPaths) <= 0 {
		return fmt.Errorf("deb paths is required: %s", c.DebPaths)
	}
	if c.Architectures == nil {
		return fmt.Errorf("architectures is required: %s", c.Architectures)
	}
	if len(c.Architectures) <= 0 {
		return fmt.Errorf("architectures is required: %s", c.Architectures)
	}
	if !slices.Contains(validStorageTypes, c.StorageType) {
		return fmt.Errorf("storage type is not valid: %s", c.StorageType)
	}
	if c.Endpoint == "" {
		return fmt.Errorf("endpoint is required: %s", c.Endpoint)
	}
	if c.BucketName == "" {
		return fmt.Errorf("bucket name is required: %s", c.BucketName)
	}
	if c.AccessKeyId == "" {
		return fmt.Errorf("access key id is required: %s", c.AccessKeyId)
	}
	if c.AccessKeySecret == "" {
		return fmt.Errorf("access key secret is required: %s", c.AccessKeySecret)
	}
	if c.GpgPrivateKey == nil {
		return fmt.Errorf("gpg private key is required: %s", c.GpgPrivateKey)
	}
	if len(c.GpgPrivateKey) == 0 {
		return fmt.Errorf("gpg private key is required: %s", c.GpgPrivateKey)
	}
	return nil
}

type SingleConfig struct {
	UbuntuDistro string
	DebPath      string
	Architecture string
	Container    string
}
