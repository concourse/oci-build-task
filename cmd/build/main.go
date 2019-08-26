package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
	task "github.com/vito/oci-build-task"
	"github.com/vrischmann/envconfig"
)

const buildArgPrefix = "BUILD_ARG_"

func main() {
	req := task.Request{
		ResponsePath: "/dev/null",
	}

	err := envconfig.Init(&req.Config)
	failIf("parse config from env", err)

	// carry over BUILD_ARG_* vars manually
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, buildArgPrefix) {
			continue
		}

		req.Config.BuildArgs = append(
			req.Config.BuildArgs,
			strings.TrimPrefix(env, buildArgPrefix),
		)
	}

	logrus.Debugf("read config from env: %#v\n", req.Config)

	reqPayload, err := json.Marshal(req)
	failIf("marshal request", err)

	task := exec.Command("task")
	task.Stdin = bytes.NewBuffer(reqPayload)
	task.Stdout = os.Stdout
	task.Stderr = os.Stderr

	err = task.Run()
	failIf("run task", err)
}

func failIf(msg string, err error) {
	if err != nil {
		logrus.Fatalln("failed to", msg+":", err)
	}
}
