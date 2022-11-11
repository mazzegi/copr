package copr

import (
	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

const (
	GlobalEnvFile = "copr.global.env"
)

func LoadGlobalEnv(file string, secs *Secrets) (map[string]string, error) {
	var glbEnv map[string]string
	_, err := toml.DecodeFile(file, &glbEnv)
	if err != nil {
		return nil, errors.Wrapf(err, "toml.decode-file %q", file)
	}
	// desecretize values
	for k, v := range glbEnv {
		glbEnv[k] = secs.Expanded(v)
	}
	return glbEnv, nil
}
