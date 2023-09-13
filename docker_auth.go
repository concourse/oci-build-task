package task

import (
	"encoding/json"
	"os"

	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/config/types"
	"github.com/sirupsen/logrus"
)

func CheckForDockerCredentials() error {
	username := os.Getenv("DOCKER_USERNAME")
	password := os.Getenv("DOCKER_PASSWORD")
	registry := os.Getenv("DOCKER_REGISTRY")

	if username == "" || password == "" || registry == "" {
		logrus.Debugf("No docker credentials in environment variables")
		return nil
	}

	config := configfile.ConfigFile{
		AuthConfigs: map[string]types.AuthConfig{
			registry: types.AuthConfig{
				Username: username,
				Password: password,
			},
		},
	}

	fileContents, err := json.MarshalIndent(config, "", " ")
	if err != nil {
		return err

	}

	fileMode := os.FileMode(0444)

	homedir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configPath := homedir + "/.docker"
	if err := os.MkdirAll(configPath, fileMode); err != nil {
		return err
	}

	filePath := configPath + "/config.json"
	if err := os.WriteFile(filePath, fileContents, fileMode); err != nil {
		return err
	}

	return nil
}
