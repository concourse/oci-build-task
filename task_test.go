package task_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	task "github.com/concourse/builder-task"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type TaskSuite struct {
	suite.Suite
	*require.Assertions

	buildkitd  *task.Buildkitd
	outputsDir string
	req        task.Request
}

func (s *TaskSuite) SetupSuite() {
	var err error
	s.buildkitd, err = task.SpawnBuildkitd()
	s.NoError(err)
}

func (s *TaskSuite) TearDownSuite() {
	err := s.buildkitd.Cleanup()
	s.NoError(err)
}

func (s *TaskSuite) SetupTest() {
	var err error
	s.outputsDir, err = ioutil.TempDir("", "builder-task-test")
	s.NoError(err)

	err = os.Mkdir(s.imagePath(), 0755)
	s.NoError(err)

	s.req = task.Request{
		ResponsePath: filepath.Join(s.outputsDir, "response.json"),
		Config:       task.Config{},
	}
}

func (s *TaskSuite) TearDownTest() {
	err := os.RemoveAll(s.outputsDir)
	s.NoError(err)
}

func (s *TaskSuite) TestBasicBuild() {
	s.req.Config.ContextDir = "testdata/basic"

	_, err := s.build()
	s.NoError(err)
}

func (s *TaskSuite) TestDigestFile() {
	s.req.Config.ContextDir = "testdata/basic"

	_, err := s.build()
	s.NoError(err)

	digest, err := ioutil.ReadFile(s.imagePath("digest"))
	s.NoError(err)

	image, err := tarball.ImageFromPath(s.imagePath("image.tar"), nil)
	s.NoError(err)

	manifest, err := image.Manifest()
	s.NoError(err)

	s.Equal(string(digest), manifest.Config.Digest.String())
}

func (s *TaskSuite) TestDockerfilePath() {
	s.req.Config.ContextDir = "testdata/dockerfile-path"
	s.req.Config.DockerfilePath = "testdata/dockerfile-path/hello.Dockerfile"

	_, err := s.build()
	s.NoError(err)
}

func (s *TaskSuite) TestTarget() {
	s.req.Config.ContextDir = "testdata/multi-target"
	s.req.Config.Target = "working-target"

	_, err := s.build()
	s.NoError(err)
}

func (s *TaskSuite) TestTargetFile() {
	s.req.Config.ContextDir = "testdata/multi-target"
	s.req.Config.TargetFile = "testdata/multi-target/target_file"

	_, err := s.build()
	s.NoError(err)
}

func (s *TaskSuite) TestBuildArgs() {
	s.req.Config.ContextDir = "testdata/build-args"
	s.req.Config.BuildArgs = []string{
		"some_arg=some_value",
		"some_other_arg=some_other_value",
	}

	// the Dockerfile itself asserts that the arg has been received
	_, err := s.build()
	s.NoError(err)
}

func (s *TaskSuite) TestBuildArgsFile() {
	s.req.Config.ContextDir = "testdata/build-args"
	s.req.Config.BuildArgsFile = "testdata/build-args/build_args_file"

	// the Dockerfile itself asserts that the arg has been received
	_, err := s.build()
	s.NoError(err)
}

func (s *TaskSuite) TestBuildArgsStaticAndFile() {
	s.req.Config.ContextDir = "testdata/build-args"
	s.req.Config.BuildArgs = []string{"some_arg=some_value"}
	s.req.Config.BuildArgsFile = "testdata/build-args/build_arg_file"

	// the Dockerfile itself asserts that the arg has been received
	_, err := s.build()
	s.NoError(err)
}

func (s *TaskSuite) TestUnpackRootfs() {
	s.req.Config.ContextDir = "testdata/unpack-rootfs"
	s.req.Config.UnpackRootfs = true

	_, err := s.build()
	s.NoError(err)

	meta, err := s.imageMetadata()
	s.NoError(err)

	rootfsContent, err := ioutil.ReadFile(s.imagePath("rootfs", "Dockerfile"))
	s.NoError(err)

	expectedContent, err := ioutil.ReadFile("testdata/unpack-rootfs/Dockerfile")
	s.NoError(err)

	s.Equal(rootfsContent, expectedContent)

	s.Equal(meta.User, "banana")
	s.Equal(meta.Env, []string{"PATH=/darkness", "BA=nana"})
}

func (s *TaskSuite) build() (task.Response, error) {
	return task.Build(s.buildkitd, s.outputsDir, s.req)
}

func (s *TaskSuite) imagePath(path ...string) string {
	return filepath.Join(append([]string{s.outputsDir, "image"}, path...)...)
}

func (s *TaskSuite) imageMetadata() (task.ImageMetadata, error) {
	metadataPayload, err := ioutil.ReadFile(s.imagePath("metadata.json"))
	if err != nil {
		return task.ImageMetadata{}, err
	}

	var meta task.ImageMetadata
	err = json.Unmarshal(metadataPayload, &meta)
	if err != nil {
		return task.ImageMetadata{}, err
	}

	return meta, nil
}

func TestSuite(t *testing.T) {
	suite.Run(t, &TaskSuite{
		Assertions: require.New(t),
	})
}
