package config

type APIConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type NATSConfig struct {
	URLS []string `yaml:"urls"`
	Name string   `yaml:"name"`
}

type Config struct {
	API  APIConfig  `yaml:"api"`
	NATS NATSConfig `yaml:"nats"`
}
