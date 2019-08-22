package main

import (
	"encoding/json"
	"os"

	task "github.com/concourse/builder-task"
	"github.com/sirupsen/logrus"
)

func main() {
	var req task.Request
	err := json.NewDecoder(os.Stdin).Decode(&req)
	failIf("read request", err)

	wd, err := os.Getwd()
	failIf("get root path", err)

	res, err := task.Build(wd, req)
	failIf("failed to build", err)

	responseFile, err := os.Create(req.ResponsePath)
	failIf("open response path", err)

	err = json.NewEncoder(responseFile).Encode(res)
	failIf("write response", err)

	err = responseFile.Close()
	failIf("close response file", err)
}

func failIf(msg string, err error) {
	if err != nil {
		logrus.Fatalln("failed to", msg+":", err)
	}
}
