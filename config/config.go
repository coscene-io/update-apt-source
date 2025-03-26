package config

import "slices"

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

func (c *Config) IsValid() bool {
	return slices.Contains(validUbuntuDistro, c.UbuntuDistro) &&
		c.DebPaths != nil &&
		len(c.DebPaths) > 0 &&
		c.Architectures != nil &&
		len(c.Architectures) > 0 &&
		slices.Contains(validStorageTypes, c.StorageType) &&
		c.Endpoint != "" &&
		c.BucketName != "" &&
		c.AccessKeyId != "" &&
		c.AccessKeySecret != "" &&
		c.GpgPrivateKey != nil &&
		len(c.GpgPrivateKey) > 0
}

type SingleConfig struct {
	UbuntuDistro string
	DebPath      string
	Architecture string
	Container    string
}
