package task_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	task "github.com/vito/oci-build-task"
)

type BuildkitdSuite struct {
	suite.Suite
	*require.Assertions

	buildkitd  *task.Buildkitd
	outputsDir string
	req        task.Request
}

func (s *BuildkitdSuite) TearDownSuite() {
	err := s.buildkitd.Cleanup()
	s.NoError(err)
}

func (s *BuildkitdSuite) SetupTest() {
	var err error
	s.outputsDir, err = ioutil.TempDir("", "oci-build-task-test")
	s.NoError(err)

	s.req = task.Request{
		ResponsePath: filepath.Join(s.outputsDir, "response.json"),
		Config:       task.Config{},
	}
}

func (s *BuildkitdSuite) TearDownTest() {
	err := os.RemoveAll(s.outputsDir)
	s.NoError(err)
}

func (s *BuildkitdSuite) TestNoConfig() {
	var pathExists bool

	if _, err := os.Stat(s.configPath()); err == nil {
		pathExists = true
	}

	s.Assert().False(pathExists)
}

func (s *BuildkitdSuite) TestGenerateConfig() {
	var err error

	var registryConfigs map[string]task.RegistryConfig
	registryConfigs = make(map[string]task.RegistryConfig)
	registryConfigs["docker.io"] = task.RegistryConfig{
		Mirrors:   []string{"hub.docker.io"},
		PlainHTTP: &[]bool{true}[0],
		Insecure:  &[]bool{true}[0],
		RootCAs:   []string{"/etc/config/myca.pem"},
		KeyPairs: []task.TLSKeyPair{
			{
				Key:         "/etc/config/key.pem",
				Certificate: "/etc/config/cert.pem",
			},
		},
	}

	s.buildkitd, err = task.SpawnBuildkitd(&task.BuildkitdOpts{
		Config: &task.BuildkitdConfig{
			Registries: registryConfigs,
		},
		ConfigPath: s.configPath("mirrors.toml"),
	})
	s.NoError(err)

	configContent, err := ioutil.ReadFile(s.configPath("mirrors.toml"))
	s.NoError(err)

	expectedContent, err := ioutil.ReadFile("testdata/buildkitd-config/mirrors.toml")
	s.NoError(err)

	s.Equal(expectedContent, configContent)
}

func (s *BuildkitdSuite) configPath(path ...string) string {
	return filepath.Join(append([]string{s.outputsDir, "config"}, path...)...)
}

func TestBuildkitd(t *testing.T) {
	suite.Run(t, &BuildkitdSuite{
		Assertions: require.New(t),
	})
}
