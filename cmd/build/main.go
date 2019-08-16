package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"

	task "github.com/concourse/builder-task"
	"github.com/sirupsen/logrus"
	"github.com/vrischmann/envconfig"
)

func main() {
	req := task.Request{
		ResponsePath: "/dev/stderr",
	}

	err := envconfig.Init(&req.Config)
	failIf("parse config from env", err)

	logrus.Debugf("read config from env: %#v\n", req.Config)

	reqPayload, err := json.Marshal(req)
	failIf("marshal request", err)

	task := exec.Command("builder-task")
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
