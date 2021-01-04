package task

type BuildkitdConfig struct {
	Registries map[string]RegistryConfig `toml:"registry"`
}

type RegistryConfig struct {
	Mirrors      []string     `toml:"mirrors"`
	PlainHTTP    *bool        `toml:"http"`
	Insecure     *bool        `toml:"insecure"`
	RootCAs      []string     `toml:"ca"`
	KeyPairs     []TLSKeyPair `toml:"keypair"`
	TLSConfigDir []string     `toml:"tlsconfigdir"`
}

type TLSKeyPair struct {
	Key         string `toml:"key"`
	Certificate string `toml:"cert"`
}

type TLSConfig struct {
	Cert string `toml:"cert"`
	Key  string `toml:"key"`
	CA   string `toml:"ca"`
}
