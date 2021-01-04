package task

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type Buildkitd struct {
	Addr string

	rootDir string
	proc    *os.Process
}

// BuildkitdOpts to provide to Buildkitd
type BuildkitdOpts struct {
	RootDir    string
	ConfigPath string
}

func SpawnBuildkitd(req Request, opts *BuildkitdOpts) (*Buildkitd, error) {
	err := run(os.Stdout, "setup-cgroups")
	if err != nil {
		return nil, errors.Wrap(err, "setup cgroups")
	}

	rootDir := filepath.Join(os.TempDir(), "buildkitd")
	if opts != nil && opts.RootDir != "" {
		rootDir = opts.RootDir
	}

	err = os.MkdirAll(rootDir, 0755)
	if err != nil {
		return nil, errors.Wrap(err, "create root dir")
	}

	sockPath := filepath.Join(rootDir, "buildkitd.sock")
	logPath := filepath.Join(rootDir, "buildkitd.log")

	configPath := filepath.Join(rootDir, "builtkitd.toml")
	if opts != nil && opts.ConfigPath != "" {
		configPath = opts.ConfigPath
	}

	err = generateConfig(req, configPath)
	if err != nil {
		return nil, errors.Wrap(err, "generate config")
	}

	addr := (&url.URL{Scheme: "unix", Path: sockPath}).String()

	buildkitdFlags := []string{
		"--root", rootDir,
		"--addr", addr,
		"--config", configPath,
	}

	if req.Config.Debug {
		buildkitdFlags = append(buildkitdFlags, "--debug")
	}

	var cmd *exec.Cmd
	if os.Getuid() == 0 {
		cmd = exec.Command("buildkitd", buildkitdFlags...)
	} else {
		cmd = exec.Command("rootlesskit", append([]string{"buildkitd"}, buildkitdFlags...)...)
	}

	// kill buildkitd on exit
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return nil, errors.Wrap(err, "open log file")
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	err = cmd.Start()
	if err != nil {
		return nil, errors.Wrap(err, "start buildkitd")
	}

	err = logFile.Close()
	if err != nil {
		return nil, errors.Wrap(err, "close log file")
	}

	for {
		err := buildctl(addr, ioutil.Discard, "debug", "workers")
		if err == nil {
			break
		}

		err = cmd.Process.Signal(syscall.Signal(0))
		if err != nil {
			logrus.Warn("builtkitd process probe failed:", err)

			logrus.Warn("dumping buildkit logs due to probe failure")
			fmt.Fprintln(os.Stderr)
			dumpLogFile(logPath)

			os.Exit(1)
		}

		logrus.Debugf("waiting for buildkitd...")
		time.Sleep(100 * time.Millisecond)
	}

	logrus.Debug("buildkitd started")

	return &Buildkitd{
		Addr: addr,

		rootDir: rootDir,
		proc:    cmd.Process,
	}, nil
}

func (buildkitd *Buildkitd) Cleanup() error {
	err := buildkitd.proc.Signal(syscall.SIGTERM)
	if err != nil {
		return errors.Wrap(err, "terminate buildkitd")
	}

	_, err = buildkitd.proc.Wait()
	if err != nil {
		return errors.Wrap(err, "wait buildkitd")
	}

	return nil
}

func generateConfig(req Request, configPath string) error {
	var config BuildkitdConfig

	if len(req.Config.RegistryMirrors) > 0 {
		var registryConfigs map[string]RegistryConfig
		registryConfigs = make(map[string]RegistryConfig)
		registryConfigs["docker.io"] = RegistryConfig{
			Mirrors: req.Config.RegistryMirrors,
		}

		config.Registries = registryConfigs
	}

	err := os.MkdirAll(filepath.Dir(configPath), 0700)
	if err != nil {
		return err
	}

	f, err := os.Create(configPath)
	if err != nil {
		return err
	}

	err = toml.NewEncoder(f).Encode(config)
	if err != nil {
		return err
	}

	return f.Close()
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
