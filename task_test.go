package task_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	task "github.com/concourse/oci-build-task"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
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
	s.buildkitd, err = task.SpawnBuildkitd(task.Request{}, nil)
	s.NoError(err)
}

func (s *TaskSuite) TearDownSuite() {
	err := s.buildkitd.Cleanup()
	s.NoError(err)
}

func (s *TaskSuite) SetupTest() {
	var err error
	s.outputsDir, err = ioutil.TempDir("", "oci-build-task-test")
	s.NoError(err)

	err = os.Mkdir(s.imagePath(), 0755)
	s.NoError(err)

	s.req = task.Request{
		ResponsePath: filepath.Join(s.outputsDir, "response.json"),
		Config: task.Config{
			Debug: true,
		},
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

func (s *TaskSuite) TestNoOutputBuild() {
	s.req.Config.ContextDir = "testdata/basic"

	err := os.RemoveAll(s.imagePath())
	s.NoError(err)

	_, err = s.build()
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
	s.req.Config.ContextDir = "testdata/target"
	s.req.Config.Target = "working-target"

	_, err := s.build()
	s.NoError(err)
}

func (s *TaskSuite) TestBuildkitSSH() {
	s.req.Config.ContextDir = "testdata/buildkit-ssh"
	s.req.Config.BuildkitSSH = "my_ssh_key=testdata/buildkit-ssh/id_rsa_test"

	_, err := s.build()
	s.NoError(err)
}

func (s *TaskSuite) TestTargetFile() {
	s.req.Config.ContextDir = "testdata/target"
	s.req.Config.TargetFile = "testdata/target/target_file"

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

func (s *TaskSuite) TestLabels() {
	s.req.Config.ContextDir = "testdata/labels"
	expectedLabels := map[string]string{
		"some_label":       "some_value",
		"some_other_label": "some_other_value",
	}
	s.req.Config.Labels = make([]string, 0, len(expectedLabels))

	for k, v := range expectedLabels {
		s.req.Config.Labels = append(s.req.Config.Labels, fmt.Sprintf("%s=%s", k, v))
	}

	_, err := s.build()
	s.NoError(err)

	image, err := tarball.ImageFromPath(s.imagePath("image.tar"), nil)
	s.NoError(err)

	configFile, err := image.ConfigFile()
	s.NoError(err)

	s.True(reflect.DeepEqual(expectedLabels, configFile.Config.Labels))
}

func (s *TaskSuite) TestLabelsFile() {
	s.req.Config.ContextDir = "testdata/labels"
	expectedLabels := map[string]string{
		"some_label":       "some_value",
		"some_other_label": "some_other_value",
	}
	s.req.Config.LabelsFile = "testdata/labels/labels_file"

	_, err := s.build()
	s.NoError(err)

	image, err := tarball.ImageFromPath(s.imagePath("image.tar"), nil)
	s.NoError(err)

	configFile, err := image.ConfigFile()
	s.NoError(err)

	s.True(reflect.DeepEqual(expectedLabels, configFile.Config.Labels))
}

func (s *TaskSuite) TestLabelsStaticAndFileAndLayer() {
	s.req.Config.ContextDir = "testdata/labels"
	s.req.Config.DockerfilePath = "testdata/labels/label_layer.dockerfile"
	expectedLabels := map[string]string{
		"some_label":       "some_value",
		"some_other_label": "some_other_value",
		"label_layer":      "some_label_layer_value",
	}
	s.req.Config.Labels = []string{"some_label=some_value"}
	s.req.Config.LabelsFile = "testdata/labels/label_file"

	_, err := s.build()
	s.NoError(err)

	image, err := tarball.ImageFromPath(s.imagePath("image.tar"), nil)
	s.NoError(err)

	configFile, err := image.ConfigFile()
	s.NoError(err)

	s.True(reflect.DeepEqual(expectedLabels, configFile.Config.Labels))
}

func (s *TaskSuite) TestUnpackRootfs() {
	s.req.Config.ContextDir = "testdata/unpack-rootfs"
	s.req.Config.UnpackRootfs = true

	_, err := s.build()
	s.NoError(err)

	meta, err := s.imageMetadata("image")
	s.NoError(err)

	rootfsContent, err := ioutil.ReadFile(s.imagePath("rootfs", "Dockerfile"))
	s.NoError(err)

	expectedContent, err := ioutil.ReadFile("testdata/unpack-rootfs/Dockerfile")
	s.NoError(err)

	s.Equal(rootfsContent, expectedContent)

	s.Equal(meta.User, "banana")
	s.Equal(meta.Env, []string{"PATH=/darkness", "BA=nana"})
}

func (s *TaskSuite) TestBuildkitTextualSecrets() {
	s.req.Config.ContextDir = "testdata/buildkit-secret"
	err := task.StoreSecret(&s.req, "secret", "hello-world")
	s.NoError(err)

	_, err = s.build()
	s.NoError(err)
}

func (s *TaskSuite) TestBuildkitSecrets() {
	s.req.Config.ContextDir = "testdata/buildkit-secret"
	s.req.Config.BuildkitSecrets = map[string]string{"secret": "testdata/buildkit-secret/secret"}

	_, err := s.build()
	s.NoError(err)
}

func (s *TaskSuite) TestRegistryMirrors() {
	mirror := httptest.NewServer(registry.New())
	defer mirror.Close()

	image, err := random.Image(1024, 2)
	s.NoError(err)

	mirrorURL, err := url.Parse(mirror.URL)
	s.NoError(err)

	mirrorRef, err := name.NewTag(fmt.Sprintf("%s/library/mirrored-image:some-tag", mirrorURL.Host))
	s.NoError(err)

	err = remote.Write(mirrorRef, image)
	s.NoError(err)

	s.req.Config.ContextDir = "testdata/mirror"
	s.req.Config.RegistryMirrors = []string{mirrorURL.Host}

	rootDir, err := ioutil.TempDir("", "mirrored-buildkitd")
	s.NoError(err)

	defer os.RemoveAll(rootDir)

	mirroredBuildkitd, err := task.SpawnBuildkitd(s.req, &task.BuildkitdOpts{
		RootDir: rootDir,
	})
	s.NoError(err)

	defer mirroredBuildkitd.Cleanup()

	_, err = task.Build(mirroredBuildkitd, s.outputsDir, s.req)
	s.NoError(err)

	builtImage, err := tarball.ImageFromPath(s.imagePath("image.tar"), nil)
	s.NoError(err)

	layers, err := image.Layers()
	s.NoError(err)

	builtLayers, err := builtImage.Layers()
	s.NoError(err)
	s.Len(builtLayers, len(layers))

	for i := 0; i < len(layers); i++ {
		digest, err := layers[i].Digest()
		s.NoError(err)

		builtDigest, err := builtLayers[i].Digest()
		s.NoError(err)

		s.Equal(digest, builtDigest)
	}
}

func (s *TaskSuite) TestImageArgs() {
	imagesDir, err := ioutil.TempDir("", "preload-images")
	s.NoError(err)

	defer os.RemoveAll(imagesDir)

	firstImage, err := random.Image(1024, 2)
	s.NoError(err)
	firstPath := filepath.Join(imagesDir, "first.tar")
	err = tarball.WriteToFile(firstPath, nil, firstImage)
	s.NoError(err)

	secondImage, err := random.Image(1024, 2)
	s.NoError(err)
	secondPath := filepath.Join(imagesDir, "second.tar")
	err = tarball.WriteToFile(secondPath, nil, secondImage)
	s.NoError(err)

	s.req.Config.ContextDir = "testdata/image-args"
	s.req.Config.AdditionalTargets = []string{"first"}
	s.req.Config.ImageArgs = []string{
		"first_image=" + firstPath,
		"second_image=" + secondPath,
	}

	err = os.Mkdir(s.outputPath("first"), 0755)
	s.NoError(err)

	_, err = s.build()
	s.NoError(err)

	firstBuiltImage, err := tarball.ImageFromPath(s.outputPath("first", "image.tar"), nil)
	s.NoError(err)

	secondBuiltImage, err := tarball.ImageFromPath(s.outputPath("image", "image.tar"), nil)
	s.NoError(err)

	for image, builtImage := range map[v1.Image]v1.Image{
		firstImage:  firstBuiltImage,
		secondImage: secondBuiltImage,
	} {
		layers, err := image.Layers()
		s.NoError(err)

		builtLayers, err := builtImage.Layers()
		s.NoError(err)
		s.Len(builtLayers, len(layers)+1)

		for i := 0; i < len(layers); i++ {
			digest, err := layers[i].Digest()
			s.NoError(err)

			builtDigest, err := builtLayers[i].Digest()
			s.NoError(err)

			s.Equal(digest, builtDigest)
		}
	}
}

func (s *TaskSuite) TestImageArgsWithUppercaseName() {
	imagesDir, err := ioutil.TempDir("", "preload-images")
	s.NoError(err)

	defer os.RemoveAll(imagesDir)

	image, err := random.Image(1024, 2)
	s.NoError(err)
	imagePath := filepath.Join(imagesDir, "first.tar")
	err = tarball.WriteToFile(imagePath, nil, image)
	s.NoError(err)

	s.req.Config.ContextDir = "testdata/image-args"
	s.req.Config.DockerfilePath = "testdata/image-args/Dockerfile.uppercase"
	s.req.Config.ImageArgs = []string{
		"FIRST_IMAGE=" + imagePath,
	}
	s.req.Config.UnpackRootfs = true

	_, err = s.build()
	s.NoError(err)

	meta, err := s.imageMetadata("image")
	s.NoError(err)

	rootfsContent, err := ioutil.ReadFile(s.imagePath("rootfs", "Dockerfile.second"))
	s.NoError(err)

	expectedContent, err := ioutil.ReadFile("testdata/image-args/Dockerfile.uppercase")
	s.NoError(err)

	s.Equal(rootfsContent, expectedContent)

	s.Equal(meta.User, "banana")
	s.Equal(meta.Env, []string{"PATH=/darkness", "BA=nana"})
}

func (s *TaskSuite) TestImageArgsUnpack() {
	imagesDir, err := ioutil.TempDir("", "preload-images")
	s.NoError(err)

	defer os.RemoveAll(imagesDir)

	image, err := random.Image(1024, 2)
	s.NoError(err)
	imagePath := filepath.Join(imagesDir, "first.tar")
	err = tarball.WriteToFile(imagePath, nil, image)
	s.NoError(err)

	s.req.Config.ContextDir = "testdata/image-args"
	s.req.Config.AdditionalTargets = []string{"first"}
	s.req.Config.ImageArgs = []string{
		"first_image=" + imagePath,
		"second_image=" + imagePath,
	}
	s.req.Config.UnpackRootfs = true

	_, err = s.build()
	s.NoError(err)

	meta, err := s.imageMetadata("image")
	s.NoError(err)

	rootfsContent, err := ioutil.ReadFile(s.imagePath("rootfs", "Dockerfile.second"))
	s.NoError(err)

	expectedContent, err := ioutil.ReadFile("testdata/image-args/Dockerfile")
	s.NoError(err)

	s.Equal(rootfsContent, expectedContent)

	s.Equal(meta.User, "banana")
	s.Equal(meta.Env, []string{"PATH=/darkness", "BA=nana"})
}

func (s *TaskSuite) TestMultiTarget() {
	s.req.Config.ContextDir = "testdata/multi-target"
	s.req.Config.AdditionalTargets = []string{"additional-target"}

	err := os.Mkdir(s.outputPath("additional-target"), 0755)
	s.NoError(err)

	_, err = s.build()
	s.NoError(err)

	finalImage, err := tarball.ImageFromPath(s.imagePath("image.tar"), nil)
	s.NoError(err)

	finalCfg, err := finalImage.ConfigFile()
	s.NoError(err)
	s.Equal("final-target", finalCfg.Config.Labels["target"])

	additionalImage, err := tarball.ImageFromPath(s.outputPath("additional-target", "image.tar"), nil)
	s.NoError(err)

	additionalCfg, err := additionalImage.ConfigFile()
	s.NoError(err)
	s.Equal("additional-target", additionalCfg.Config.Labels["target"])
}

func (s *TaskSuite) TestMultiTargetExplicitTarget() {
	s.req.Config.ContextDir = "testdata/multi-target"
	s.req.Config.AdditionalTargets = []string{"additional-target"}
	s.req.Config.Target = "final-target"

	err := os.Mkdir(s.outputPath("additional-target"), 0755)
	s.NoError(err)

	_, err = s.build()
	s.NoError(err)

	finalImage, err := tarball.ImageFromPath(s.imagePath("image.tar"), nil)
	s.NoError(err)

	finalCfg, err := finalImage.ConfigFile()
	s.NoError(err)
	s.Equal("final-target", finalCfg.Config.Labels["target"])

	additionalImage, err := tarball.ImageFromPath(s.outputPath("additional-target", "image.tar"), nil)
	s.NoError(err)

	additionalCfg, err := additionalImage.ConfigFile()
	s.NoError(err)
	s.Equal("additional-target", additionalCfg.Config.Labels["target"])
}

func (s *TaskSuite) TestMultiTargetDigest() {
	s.req.Config.ContextDir = "testdata/multi-target"
	s.req.Config.AdditionalTargets = []string{"additional-target"}

	err := os.Mkdir(s.outputPath("additional-target"), 0755)
	s.NoError(err)

	_, err = s.build()
	s.NoError(err)

	additionalImage, err := tarball.ImageFromPath(s.outputPath("additional-target", "image.tar"), nil)
	s.NoError(err)
	digest, err := ioutil.ReadFile(s.outputPath("additional-target", "digest"))
	s.NoError(err)
	additionalManifest, err := additionalImage.Manifest()
	s.NoError(err)
	s.Equal(string(digest), additionalManifest.Config.Digest.String())

	finalImage, err := tarball.ImageFromPath(s.imagePath("image.tar"), nil)
	s.NoError(err)
	digest, err = ioutil.ReadFile(s.outputPath("image", "digest"))
	s.NoError(err)
	finalManifest, err := finalImage.Manifest()
	s.NoError(err)
	s.Equal(string(digest), finalManifest.Config.Digest.String())
}

func (s *TaskSuite) TestMultiTargetUnpack() {
	s.req.Config.ContextDir = "testdata/multi-target"
	s.req.Config.AdditionalTargets = []string{"additional-target"}
	s.req.Config.UnpackRootfs = true

	err := os.Mkdir(s.outputPath("additional-target"), 0755)
	s.NoError(err)

	_, err = s.build()
	s.NoError(err)

	rootfsContent, err := ioutil.ReadFile(s.outputPath("additional-target", "rootfs", "Dockerfile.banana"))
	s.NoError(err)
	expectedContent, err := ioutil.ReadFile("testdata/multi-target/Dockerfile")
	s.NoError(err)
	s.Equal(rootfsContent, expectedContent)

	meta, err := s.imageMetadata("additional-target")
	s.NoError(err)
	s.Equal(meta.User, "banana")
	s.Equal(meta.Env, []string{"PATH=/darkness", "BA=nana"})

	rootfsContent, err = ioutil.ReadFile(s.outputPath("image", "rootfs", "Dockerfile.orange"))
	s.NoError(err)
	expectedContent, err = ioutil.ReadFile("testdata/multi-target/Dockerfile")
	s.NoError(err)
	s.Equal(rootfsContent, expectedContent)

	meta, err = s.imageMetadata("image")
	s.NoError(err)
	s.Equal(meta.User, "orange")
	s.Equal(meta.Env, []string{"PATH=/lightness", "OR=ange"})
}

func (s *TaskSuite) TestAddHosts() {
	s.req.Config.ContextDir = "testdata/add-hosts"
	s.req.Config.AddHosts = "test-host=1.2.3.4"

	_, err := s.build()
	s.NoError(err)
}

func (s *TaskSuite) TestImagePlatform() {
	s.req.Config.ContextDir = "testdata/basic"
	s.req.Config.ImagePlatform = "linux/arm64"

	_, err := s.build()
	s.NoError(err)

	image, err := tarball.ImageFromPath(s.imagePath("image.tar"), nil)
	s.NoError(err)

	configFile, err := image.ConfigFile()
	s.NoError(err)

	s.Equal("linux", configFile.OS)
	s.Equal("arm64", configFile.Architecture)
}

func (s *TaskSuite) TestOciImage() {
	s.req.Config.ContextDir = "testdata/multi-arch"
	s.req.Config.ImagePlatform = "linux/arm64,linux/amd64"
	s.req.Config.OutputOCI = true

	_, err := s.build()
	s.NoError(err)

	l, err := layout.ImageIndexFromPath(s.imagePath("image"))
	s.NoError(err)

	im, err := l.IndexManifest()
	s.NoError(err)

	desc := im.Manifests[0]
	ii, err := l.ImageIndex(desc.Digest)
	s.NoError(err)

	images, err := ii.IndexManifest()
	s.NoError(err)

	expectedArch := []string{"arm64", "amd64"}
	var actualArch []string
	for _, manifest := range images.Manifests {
		actualArch = append(actualArch, string(manifest.Platform.Architecture))
	}

	s.True(reflect.DeepEqual(expectedArch, actualArch))
}

func (s *TaskSuite) build() (task.Response, error) {
	return task.Build(s.buildkitd, s.outputsDir, s.req)
}

func (s *TaskSuite) imagePath(path ...string) string {
	return s.outputPath(append([]string{"image"}, path...)...)
}

func (s *TaskSuite) outputPath(path ...string) string {
	return filepath.Join(append([]string{s.outputsDir}, path...)...)
}

func (s *TaskSuite) imageMetadata(output string) (task.ImageMetadata, error) {
	metadataPayload, err := ioutil.ReadFile(s.outputPath(output, "metadata.json"))
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
