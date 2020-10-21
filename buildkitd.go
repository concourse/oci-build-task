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

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type Buildkitd struct {
	Addr string

	rootDir string
	opts    BuildkitdOpts
	proc    *os.Process
}

// BuildkitdOpts to provide to Buildkitd
type BuildkitdOpts struct {
	Config BuildkitdConfig
}

func SpawnBuildkitd(opts *BuildkitdOpts) (*Buildkitd, error) {
	buildkitd := Buildkitd{}
	if opts != nil {
		buildkitd.opts = *opts
	}

	err := run(os.Stdout, "setup-cgroups")
	if err != nil {
		return nil, errors.Wrap(err, "setup cgroups")
	}

	err = generateConfig(buildkitd.opts.Config)
	if err != nil {
		return nil, errors.Wrap(err, "generate config")
	}

	buildkitd.rootDir = filepath.Join(os.TempDir(), "buildkitd")
	err = os.MkdirAll(buildkitd.rootDir, 0755)
	if err != nil {
		return nil, errors.Wrap(err, "create root dir")
	}

	sockPath := filepath.Join(buildkitd.rootDir, "buildkitd.sock")
	logPath := filepath.Join(buildkitd.rootDir, "buildkitd.log")

	buildkitd.Addr = (&url.URL{
		Scheme: "unix",
		Path:   sockPath,
	}).String()

	buildkitdFlags := []string{
		"--root", buildkitd.rootDir,
		"--addr", buildkitd.Addr,
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
		err := buildctl(buildkitd.Addr, ioutil.Discard, "debug", "workers")
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

	buildkitd.proc = cmd.Process
	return &buildkitd, nil
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

func generateConfig(config BuildkitdConfig) error {
	return nil
}
