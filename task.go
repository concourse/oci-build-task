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
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/concourse/go-archive/tarfs"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/u-root/u-root/pkg/termios"
)

func Build(rootPath string, req Request) (Response, error) {
	if req.Config.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	imageDir := filepath.Join(rootPath, "image")
	cacheDir := filepath.Join(rootPath, "cache")

	res := Response{
		Outputs: []string{"image", "cache"},
	}

	// limit max columns; Concourse sets a super high value and buildctl happily
	// fills the whole screen with whitespace
	ws, err := termios.GetWinSize(os.Stdout.Fd())
	if err == nil {
		ws.Col = 80

		err = termios.SetWinSize(os.Stdout.Fd(), ws)
		if err != nil {
			logrus.Warn("failed to set window size:", err)
		}
	}

	err = os.MkdirAll(cacheDir, 0755)
	if err != nil {
		return Response{}, errors.Wrap(err, "create image output folder")
	}

	cfg := req.Config
	err = sanitize(&cfg)
	if err != nil {
		return Response{}, errors.Wrap(err, "config")
	}

	err = run(os.Stdout, "setup-cgroups")
	if err != nil {
		return Response{}, errors.Wrap(err, "setup cgroups")
	}

	addr, err := spawnBuildkitd("/var/log/buildkitd.log")
	if err != nil {
		return Response{}, errors.Wrap(err, "spawn buildkitd")
	}

	dockerfileDir := filepath.Dir(cfg.DockerfilePath)
	dockerfileName := filepath.Base(cfg.DockerfilePath)

	buildctlArgs := []string{
		"build",
		"--frontend", "dockerfile.v0",
		"--local", "context=" + cfg.ContextPath,
		"--local", "dockerfile=" + dockerfileDir,
		"--frontend-opt", "filename=" + dockerfileName,
		"--export-cache", "type=local,mode=min,dest=cache",
	}

	if _, err := os.Stat(filepath.Join(cacheDir, "index.json")); err == nil {
		buildctlArgs = append(buildctlArgs,
			"--import-cache", "type=local,src="+cacheDir,
		)
	}

	var ociImagePath string
	if _, err := os.Stat(imageDir); err == nil {
		ociImagePath = filepath.Join(imageDir, "image.tar")
		buildctlArgs = append(buildctlArgs,
			"--output", "type=oci,name="+cfg.ImageName()+",dest="+ociImagePath,
		)
	}

	if cfg.Target != "" {
		buildctlArgs = append(buildctlArgs,
			"--opt", "target="+cfg.Target,
		)
	}

	for _, arg := range cfg.BuildArgs {
		buildctlArgs = append(buildctlArgs,
			"--build-arg", arg,
		)
	}

	logrus.WithFields(logrus.Fields{
		"buildctl-args": buildctlArgs,
	}).Debug("building")

	err = buildctl(addr, os.Stdout, buildctlArgs...)
	if err != nil {
		return Response{}, errors.Wrap(err, "build")
	}

	if req.Config.UnpackRootfs {
		err = unpackRootfs(imageDir, ociImagePath, cfg)
		if err != nil {
			return Response{}, errors.Wrap(err, "unpack rootfs")
		}
	}

	return res, nil
}

func unpackRootfs(dest string, ociImagePath string, cfg Config) error {
	layoutDir := filepath.Join(dest, "layout")
	rootfsDir := filepath.Join(dest, "rootfs")
	manifestPath := filepath.Join(dest, "manifest.json")

	logrus.Debug("unpacking oci layout")

	tarFile, err := os.Open(ociImagePath)
	if err != nil {
		return errors.Wrap(err, "open oci archive")
	}

	err = tarfs.Extract(tarFile, layoutDir)
	if err != nil {
		return errors.Wrap(err, "unpack oci archive")
	}

	err = tarFile.Close()
	if err != nil {
		return errors.Wrap(err, "close oci archive")
	}

	imageIndex, err := layout.ImageIndexFromPath(layoutDir)
	if err != nil {
		return errors.Wrap(err, "load image layout")
	}

	manifest, err := imageIndex.IndexManifest()
	if err != nil {
		return errors.Wrap(err, "get index manifest")
	}

	var unpacked string
	for _, m := range manifest.Manifests {
		if m.Platform != nil {
			if m.Platform.OS != runtime.GOOS || m.Platform.Architecture != runtime.GOARCH {
				continue
			}
		}

		if m.Annotations != nil && m.Annotations[ocispec.AnnotationRefName] != cfg.Tag {
			continue
		}

		if unpacked != "" {
			logrus.WithFields(logrus.Fields{
				"digest":           m.Digest,
				"already-unpacked": unpacked,
			}).Fatalln("found another image to unpack after already unpacking one")
		}

		logrus.WithFields(logrus.Fields{
			"platform":    m.Platform,
			"annotations": m.Annotations,
			"digest":      m.Digest,
		}).Debug("unpacking image")

		image, err := imageIndex.Image(m.Digest)
		if err != nil {
			return errors.Wrap(err, "get image from oci layout")
		}

		err = unpackImage(rootfsDir, image, false)
		if err != nil {
			return errors.Wrap(err, "unpack image")
		}

		err = writeImageManifest(manifestPath, image)
		if err != nil {
			return errors.Wrap(err, "write image manifest")
		}

		unpacked = m.Digest.String()
	}

	if unpacked == "" {
		return errors.New("could not determine image to unpack")
	}

	return nil
}

func writeImageManifest(manifestPath string, image v1.Image) error {
	cfg, err := image.ConfigFile()
	if err != nil {
		return errors.Wrap(err, "load image config")
	}

	meta, err := os.Create(manifestPath)
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
	if cfg.Repository == "" {
		return errors.New("repository must be specified")
	}

	if cfg.ContextPath == "" {
		cfg.ContextPath = "."
	}

	if cfg.DockerfilePath == "" {
		cfg.DockerfilePath = filepath.Join(cfg.ContextPath, "Dockerfile")
	}

	if cfg.TagFile != "" {
		target, err := ioutil.ReadFile(cfg.TagFile)
		if err != nil {
			return errors.Wrap(err, "read target file")
		}

		cfg.Tag = strings.TrimSpace(string(target))
	} else if cfg.Tag == "" {
		cfg.Tag = "latest"
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

func spawnBuildkitd(logPath string) (string, error) {
	runPath := os.Getenv("XDG_RUNTIME_PATH")
	if runPath == "" {
		runPath = "/run"
	}

	addr := (&url.URL{
		Scheme: "unix",
		Path:   path.Join(runPath, "buildkitd", "buildkitd.sock"),
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
