package config

import (
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"go.uber.org/multierr"
)

type (
	Application struct {
		Lang      string
		LogLevel  string
		LogFormat string
	}

	Broker struct {
		UserURL         string
		UserCredits     string
		ExchangePrefix  string
		ExchangePostfix string
	}

	Config struct {
		Application Application
		Broker      Broker
	}
)

func (b *Broker) validate() error {
	if b.UserURL == "" {
		return errors.New("empty broker url provided")
	}
	if b.UserCredits == "" {
		return errors.New("empty broker credentials provided")
	}
	return nil
}

func (a *Application) validate() error {
	return nil
}

func (c *Config) validate() error {
	return multierr.Combine(
		c.Application.validate(),
		c.Broker.validate(),
	)
}

func Parse(filepath string) (*Config, error) {
	setDefaults()

	// parse the file
	viper.SetConfigFile(filepath)
	if err := viper.ReadInConfig(); err != nil {
		return nil, errors.Wrap(err, "failed to read the config file")
	}

	// unmarshal the config
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal the configurations")
	}

	// validate the provided configuration
	if err := cfg.validate(); err != nil {
		return nil, errors.Wrap(err, "failed to validate the configurations")
	}

	return &cfg, nil
}

func setDefaults() {
	viper.SetDefault("Application.Lang", "ru")
	viper.SetDefault("Application.LogLevel", "debug")
	viper.SetDefault("Application.LogFormat", "text")

	viper.SetDefault("Broker.UserURL", "localhost")
	viper.SetDefault("Broker.UserCredits", "login:password")
}
