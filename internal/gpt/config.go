package gpt

import (
	"os"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

type gptConfig struct {
	Model       string  `yaml:"model"`
	Temperature float32 `yaml:"temperature"`
	Prompt      string  `yaml:"prompt"`
}

func loadGTPConfig() *gptConfig {
	const sourcePath = "./conf/gpt.yaml"
	raw, err := os.ReadFile(sourcePath)
	if err != nil {
		log.Error().Err(err).Str("path", sourcePath).Msg("unable to load gpt config")
		return &defaultGTPConfig
	}

	var cfg gptConfig
	err = yaml.Unmarshal(raw, &cfg)
	if err != nil {
		log.Error().Err(err).Str("path", sourcePath).Msg("unable to parse gpt config")
		return &defaultGTPConfig
	}

	return &cfg
}

var defaultGTPConfig = gptConfig{
	Model:       "gpt-3.5-turbo",
	Temperature: 0.9,
	Prompt: "    Как бы малообразованный человек в состоянии алкогольного опьянения, " +
		" разговаривающий матом и эмоционально, пересказал бы следующую новость?" +
		"Комментарий должен быть эмоциональным, просторечным и матерным.",
}
