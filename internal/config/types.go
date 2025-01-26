package config

type APIConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type NATSConfig struct {
	URL  string `yaml:"url"`
	Name string `yaml:"name"`
}

type ControllerConfig struct {
}

type WorkerConfig struct {
}

type Config struct {
	API        APIConfig        `yaml:"api"`
	NATS       NATSConfig       `yaml:"nats"`
	Controller ControllerConfig `yaml:"controller"`
	Worker     WorkerConfig     `yaml:"worker"`
}
