package task

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func Build(outputsDir string, req Request) (Response, error) {
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

	err = os.MkdirAll(imageDir, 0755)
	if err != nil {
		return Response{}, errors.Wrap(err, "create image output folder")
	}

	err = os.MkdirAll(cacheDir, 0755)
	if err != nil {
		return Response{}, errors.Wrap(err, "create cache output folder")
	}

	addr, err := spawnBuildkitd()
	if err != nil {
		return Response{}, errors.Wrap(err, "spawn buildkitd")
	}

	imagePath := filepath.Join(imageDir, "image.tar")
	digestPath := filepath.Join(imageDir, "digest")

	dockerfileDir := filepath.Dir(cfg.DockerfilePath)
	dockerfileName := filepath.Base(cfg.DockerfilePath)

	buildctlArgs := []string{
		"build",
		"--frontend", "dockerfile.v0",
		"--local", "context=" + cfg.ContextDir,
		"--local", "dockerfile=" + dockerfileDir,
		"--opt", "filename=" + dockerfileName,
		"--export-cache", "type=local,mode=min,dest=" + cacheDir,
		"--output", "type=docker,dest=" + imagePath,
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

	for _, arg := range cfg.BuildArgs {
		buildctlArgs = append(buildctlArgs,
			"--opt", "build-arg:"+arg,
		)
	}

	logrus.WithFields(logrus.Fields{
		"buildctl-args": buildctlArgs,
	}).Debug("building")

	err = buildctl(addr, os.Stdout, buildctlArgs...)
	if err != nil {
		return Response{}, errors.Wrap(err, "build")
	}

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

func spawnBuildkitd() (string, error) {
	err := run(os.Stdout, "setup-cgroups")
	if err != nil {
		return "", errors.Wrap(err, "setup cgroups")
	}

	var logPath string

	runDir := os.Getenv("XDG_RUNTIME_DIR")
	if runDir == "" {
		runDir = "/run"
		logPath = "/var/log/buildkitd.log"
	} else {
		logPath = filepath.Join(runDir, "buildkitd.log")
	}

	addr := (&url.URL{
		Scheme: "unix",
		Path:   path.Join(runDir, "buildkitd", "buildkitd.sock"),
	}).String()

	buildkitdFlags := []string{"--addr=" + addr}

	var cmd *exec.Cmd
	if os.Getuid() == 0 {
		cmd = exec.Command("buildkitd", buildkitdFlags...)
	} else {
		cmd = exec.Command("rootlesskit", append([]string{"buildkitd"}, buildkitdFlags...)...)
	}

	// kill buildkitd on exit
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGTERM,
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return "", errors.Wrap(err, "open log file")
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	err = cmd.Start()
	if err != nil {
		return "", errors.Wrap(err, "start buildkitd")
	}

	err = logFile.Close()
	if err != nil {
		return "", errors.Wrap(err, "close log file")
	}

	for {
		err := buildctl(addr, ioutil.Discard, "debug", "workers")
		if err == nil {
			break
		}

		err = cmd.Process.Signal(syscall.Signal(0))
		if err != nil {
			logrus.Warn("builtkitd process probe failed:", err)
			logrus.Info("dumping buildkit logs due to probe failure")

			fmt.Fprintln(os.Stderr)
			dumpLogFile(logPath)
			os.Exit(1)
		}

		logrus.Debugf("waiting for buildkitd...")
		time.Sleep(100 * time.Millisecond)
	}

	logrus.Debug("buildkitd started")

	return addr, nil
}

func dumpLogFile(logPath string) {
	logFile, err := os.Open(logPath)
	if err != nil {
		logrus.Warn("error opening log file:", err)
		return
	}

	_, err = io.Copy(os.Stderr, logFile)
	if err != nil {
		logrus.Warn("error streaming log file:", err)
		return
	}

	err = logFile.Close()
	if err != nil {
		logrus.Warn("error closing log file:", err)
	}
}
