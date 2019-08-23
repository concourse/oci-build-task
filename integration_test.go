package task_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	task "github.com/concourse/builder-task"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type TaskSuite struct {
	suite.Suite
	*require.Assertions

	rootDir string
	req     task.Request
}

func (s *TaskSuite) SetupTest() {
	var err error
	s.rootDir, err = ioutil.TempDir("", "builder-task-test")
	s.NoError(err)

	s.req = task.Request{
		ResponsePath: filepath.Join(s.rootDir, "response.json"),
		Config: task.Config{
			Repository: "builder-task-test",
		},
	}
}

func (s *TaskSuite) TearDownTest() {
	err := os.RemoveAll(s.rootDir)
	s.NoError(err)
}

func (s *TaskSuite) TestMissingRepositoryValidation() {
	s.req.Config.Repository = ""

	_, err := task.Build(s.rootDir, s.req)
	s.EqualError(err, "config: repository must be specified")
}

func (s *TaskSuite) TestBasicBuild() {
	s.req.Config.ContextPath = "testdata/basic"

	_, err := task.Build(s.rootDir, s.req)
	s.NoError(err)
}

func (s *TaskSuite) TestBuildArgs() {
	s.req.Config.ContextPath = "testdata/build-args"
	s.req.Config.BuildArgs = []string{
		"some_arg=some_value",
		"some_other_arg=some_other_value",
	}

	// the Dockerfile itself asserts that the arg has been received
	_, err := task.Build(s.rootDir, s.req)
	s.NoError(err)
}

func (s *TaskSuite) TestBuildArgsFile() {
	s.req.Config.ContextPath = "testdata/build-args"
	s.req.Config.BuildArgsFile = "testdata/build-args/build_args_file"

	// the Dockerfile itself asserts that the arg has been received
	_, err := task.Build(s.rootDir, s.req)
	s.NoError(err)
}

func (s *TaskSuite) TestBuildArgsStaticAndFile() {
	s.req.Config.ContextPath = "testdata/build-args"
	s.req.Config.BuildArgs = []string{"some_arg=some_value"}
	s.req.Config.BuildArgsFile = "testdata/build-args/build_arg_file"

	// the Dockerfile itself asserts that the arg has been received
	_, err := task.Build(s.rootDir, s.req)
	s.NoError(err)
}

func (s *TaskSuite) TestUnpackRootfs() {
	s.req.Config.ContextPath = "testdata/unpack-rootfs"
	s.req.Config.UnpackRootfs = true

	_, err := task.Build(s.rootDir, s.req)
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

func (s *TaskSuite) imagePath(path ...string) string {
	return filepath.Join(append([]string{s.rootDir, "image"}, path...)...)
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
