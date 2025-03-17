package config

type Config struct {
	UbuntuDistro    string
	DebPaths        []string
	Architectures   []string
	AccessKeyId     string
	AccessKeySecret string
	GpgPrivateKey   []byte
}

func (c *Config) IsValid() bool {
	return c.UbuntuDistro != "" && c.DebPaths != nil && len(c.DebPaths) > 0 && c.Architectures != nil && len(c.Architectures) > 0 && c.AccessKeyId != "" && c.AccessKeySecret != "" && c.GpgPrivateKey != nil
}

type SingleConfig struct {
	//UbuntuDistro string
	DebPath      string
	Architecture string
	Container    string
	//AccessKeyId     string
	//AccessKeySecret string
	//GpgPrivateKey   string
}
