package task

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func Build(buildkitd *Buildkitd, outputsDir string, req Request) (Response, error) {
	if req.Config.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	cfg := req.Config
	err := sanitize(&cfg)
	if err != nil {
		return Response{}, errors.Wrap(err, "config")
	}

	imageDir := filepath.Join(outputsDir, "image")
	cacheDir := filepath.Join(outputsDir, "cache")

	res := Response{
		Outputs: []string{"image", "cache"},
	}

	dockerfileDir := filepath.Dir(cfg.DockerfilePath)
	dockerfileName := filepath.Base(cfg.DockerfilePath)

	buildctlArgs := []string{
		"build",
		"--progress", "plain",
		"--frontend", "dockerfile.v0",
		"--local", "context=" + cfg.ContextDir,
		"--local", "dockerfile=" + dockerfileDir,
		"--opt", "filename=" + dockerfileName,
	}

	var imagePath, digestPath string
	if _, err := os.Stat(imageDir); err == nil {
		imagePath = filepath.Join(imageDir, "image.tar")
		digestPath = filepath.Join(imageDir, "digest")

		buildctlArgs = append(buildctlArgs,
			"--output", "type=docker,dest="+imagePath,
		)
	}

	if _, err := os.Stat(cacheDir); err == nil {
		buildctlArgs = append(buildctlArgs,
			"--export-cache", "type=local,mode=min,dest="+cacheDir,
		)
	}

	if _, err := os.Stat(filepath.Join(cacheDir, "index.json")); err == nil {
		buildctlArgs = append(buildctlArgs,
			"--import-cache", "type=local,src="+cacheDir,
		)
	}

	if cfg.Target != "" {
		buildctlArgs = append(buildctlArgs,
			"--opt", "target="+cfg.Target,
		)
	}

	if cfg.BuildkitSSH != "" {
		buildctlArgs = append(buildctlArgs,
			"--ssh", cfg.BuildkitSSH,
		)
	}

	for _, arg := range cfg.BuildArgs {
		buildctlArgs = append(buildctlArgs,
			"--opt", "build-arg:"+arg,
		)
	}

	logrus.WithFields(logrus.Fields{
		"buildctl-args": buildctlArgs,
	}).Debug("building")

	err = buildctl(buildkitd.Addr, os.Stdout, buildctlArgs...)
	if err != nil {
		return Response{}, errors.Wrap(err, "build")
	}

	if imagePath != "" {
		image, err := tarball.ImageFromPath(imagePath, nil)
		if err != nil {
			return Response{}, errors.Wrap(err, "open oci image")
		}

		manifest, err := image.Manifest()
		if err != nil {
			return Response{}, errors.Wrap(err, "get image digest")
		}

		err = ioutil.WriteFile(digestPath, []byte(manifest.Config.Digest.String()), 0644)
		if err != nil {
			return Response{}, errors.Wrap(err, "write digest")
		}

		if req.Config.UnpackRootfs {
			err = unpackRootfs(imageDir, image, cfg)
			if err != nil {
				return Response{}, errors.Wrap(err, "unpack rootfs")
			}
		}
	}

	return res, nil
}

func unpackRootfs(dest string, image v1.Image, cfg Config) error {
	rootfsDir := filepath.Join(dest, "rootfs")
	metadataPath := filepath.Join(dest, "metadata.json")

	logrus.Info("unpacking image")

	err := unpackImage(rootfsDir, image, cfg.Debug)
	if err != nil {
		return errors.Wrap(err, "unpack image")
	}

	err = writeImageMetadata(metadataPath, image)
	if err != nil {
		return errors.Wrap(err, "write image metadata")
	}

	return nil
}

func writeImageMetadata(metadataPath string, image v1.Image) error {
	cfg, err := image.ConfigFile()
	if err != nil {
		return errors.Wrap(err, "load image config")
	}

	meta, err := os.Create(metadataPath)
	if err != nil {
		return errors.Wrap(err, "create metadata file")
	}

	env := cfg.Config.Env
	if len(env) == 0 {
		env = cfg.ContainerConfig.Env
	}

	user := cfg.Config.User
	if user == "" {
		user = cfg.ContainerConfig.User
	}

	err = json.NewEncoder(meta).Encode(ImageMetadata{
		Env:  env,
		User: user,
	})
	if err != nil {
		return errors.Wrap(err, "encode metadata")
	}

	err = meta.Close()
	if err != nil {
		return errors.Wrap(err, "close meta")
	}

	return nil
}

func sanitize(cfg *Config) error {
	if cfg.ContextDir == "" {
		cfg.ContextDir = "."
	}

	if cfg.DockerfilePath == "" {
		cfg.DockerfilePath = filepath.Join(cfg.ContextDir, "Dockerfile")
	}

	if cfg.TargetFile != "" {
		target, err := ioutil.ReadFile(cfg.TargetFile)
		if err != nil {
			return errors.Wrap(err, "read target file")
		}

		cfg.Target = strings.TrimSpace(string(target))
	}

	if cfg.BuildArgsFile != "" {
		buildArgs, err := ioutil.ReadFile(cfg.BuildArgsFile)
		if err != nil {
			return errors.Wrap(err, "read build args file")
		}

		for _, arg := range strings.Split(string(buildArgs), "\n") {
			if len(arg) == 0 {
				// skip blank lines
				continue
			}

			cfg.BuildArgs = append(cfg.BuildArgs, arg)
		}
	}

	return nil
}

func buildctl(addr string, out io.Writer, args ...string) error {
	return run(out, "buildctl", append([]string{"--addr=" + addr}, args...)...)
}

func run(out io.Writer, path string, args ...string) error {
	cmd := exec.Command(path, args...)
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
