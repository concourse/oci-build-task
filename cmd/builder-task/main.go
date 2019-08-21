package main

import (
	"encoding/json"
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

	task "github.com/concourse/builder-task"
	"github.com/concourse/go-archive/tarfs"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"github.com/u-root/u-root/pkg/termios"
)

func main() {
	var req task.Request
	err := json.NewDecoder(os.Stdin).Decode(&req)
	failIf("read request", err)

	if req.Config.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	rootPath, err := os.Getwd()
	failIf("get root path", err)

	imageDir := filepath.Join(rootPath, "image")
	cacheDir := filepath.Join(rootPath, "cache")

	res := task.Response{
		Outputs: []string{"image", "cache"},
	}

	responseFile, err := os.Create(req.ResponsePath)
	failIf("open response path", err)

	defer func() {
		err := json.NewEncoder(responseFile).Encode(res)
		failIf("write response", err)

		err = responseFile.Close()
		failIf("close response file", err)
	}()

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
	failIf("create image output folder", err)

	cfg := req.Config
	sanitize(&cfg)

	err = run(os.Stdout, "setup-cgroups")
	failIf("setup cgroups", err)

	addr := spawnBuildkitd("/var/log/buildkitd.log")

	buildctlArgs := []string{
		"build",
		"--frontend", "dockerfile.v0",
		"--local", "context=" + cfg.ContextPath,
		"--local", "dockerfile=" + cfg.DockerfilePath,
		"--export-cache", "type=local,mode=min,dest=cache",
	}

	if _, err := os.Stat(filepath.Join(cacheDir, "index.json")); err == nil {
		buildctlArgs = append(buildctlArgs,
			"--import-cache", "type=local,src=cache",
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
	failIf("build", err)

	if req.Config.UnpackRootfs {
		unpackRootfs(imageDir, ociImagePath, cfg)
	}
}

func unpackRootfs(dest string, ociImagePath string, cfg task.Config) {
	layoutDir := filepath.Join(dest, "layout")
	rootfsDir := filepath.Join(dest, "rootfs")
	manifestPath := filepath.Join(dest, "manifest.json")

	logrus.Debug("unpacking oci layout")

	tarFile, err := os.Open(ociImagePath)
	failIf("open oci archive", err)

	err = tarfs.Extract(tarFile, layoutDir)
	failIf("unpack oci archive", err)

	err = tarFile.Close()
	failIf("close oci archive", err)

	imageIndex, err := layout.ImageIndexFromPath(layoutDir)
	failIf("load image layout", err)

	manifest, err := imageIndex.IndexManifest()
	failIf("get index manifest", err)

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
		failIf("get image from oci layout", err)

		unpackImage(rootfsDir, image, false)
		writeImageManifest(manifestPath, image)

		unpacked = m.Digest.String()
	}

	if unpacked == "" {
		logrus.Fatalln("could not determine image to unpack")
	}
}

func writeImageManifest(manifestPath string, image v1.Image) {
	cfg, err := image.ConfigFile()
	failIf("load image config", err)

	meta, err := os.Create(manifestPath)
	failIf("create metadata file", err)

	env := cfg.Config.Env
	if len(env) == 0 {
		env = cfg.ContainerConfig.Env
	}

	user := cfg.Config.User
	if user == "" {
		user = cfg.ContainerConfig.User
	}

	err = json.NewEncoder(meta).Encode(task.ImageMetadata{
		Env:  env,
		User: user,
	})
	failIf("encode metadata", err)

	err = meta.Close()
	failIf("close meta", err)
}

func sanitize(cfg *task.Config) {
	if cfg.Repository == "" {
		logrus.Fatal("repository must be specified")
	}

	if cfg.ContextPath == "" {
		cfg.ContextPath = "."
	}

	if cfg.DockerfilePath == "" {
		cfg.DockerfilePath = cfg.ContextPath
	}

	if cfg.TagFile != "" {
		target, err := ioutil.ReadFile(cfg.TagFile)
		failIf("read target file", err)

		cfg.Tag = strings.TrimSpace(string(target))
	} else if cfg.Tag == "" {
		cfg.Tag = "latest"
	}

	if cfg.TargetFile != "" {
		target, err := ioutil.ReadFile(cfg.TargetFile)
		failIf("read target file", err)

		cfg.Target = strings.TrimSpace(string(target))
	}

	if cfg.BuildArgsFile != "" {
		buildArgs, err := ioutil.ReadFile(cfg.BuildArgsFile)
		failIf("read build args file", err)

		for _, arg := range strings.Split(string(buildArgs), "\n") {
			cfg.BuildArgs = append(cfg.BuildArgs, arg)
		}
	}
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

func spawnBuildkitd(logPath string) string {
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
	failIf("open log file", err)

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	err = cmd.Start()
	failIf("start buildkitd", err)

	err = logFile.Close()
	failIf("close log file", err)

	for {
		err := buildctl(addr, ioutil.Discard, "debug", "workers")
		if err == nil {
			break
		}

		err = cmd.Process.Signal(syscall.Signal(0))
		if err != nil {
			logrus.Warn("builtkitd process probe failed:", err)
			logrus.Info("dumping buildkit logs due to probe failure")

			// fmt.Fprintln(os.Stderr)
			// dumpLogFile(logFile)
			// os.Exit(1)
		}

		logrus.Debugf("waiting for buildkitd...")
		time.Sleep(100 * time.Millisecond)
	}

	logrus.Debug("buildkitd started")

	return addr
}

func dumpLogFile(logFile *os.File) {
	_, err := logFile.Seek(0, 0)
	failIf("seek log file", err)

	_, err = io.Copy(os.Stderr, logFile)
	failIf("copy from log file", err)
}

func failIf(msg string, err error) {
	if err != nil {
		logrus.Fatalln("failed to", msg+":", err)
	}
}
