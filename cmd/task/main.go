package main

import (
	"encoding/json"
	"os"

	task "github.com/vito/oci-build-task"
	"github.com/sirupsen/logrus"
	"github.com/u-root/u-root/pkg/termios"
)

func main() {
	var req task.Request
	err := json.NewDecoder(os.Stdin).Decode(&req)
	failIf("read request", err)

	wd, err := os.Getwd()
	failIf("get root path", err)

	// limit max columns; Concourse sets a super high value and buildctl happily
	// fills the whole screen with whitespace
	ws, err := termios.GetWinSize(os.Stdout.Fd())
	if err == nil {
		ws.Col = 100

		err = termios.SetWinSize(os.Stdout.Fd(), ws)
		if err != nil {
			logrus.Warn("failed to set window size:", err)
		}
	}

	buildkitd, err := task.SpawnBuildkitd()
	failIf("start buildkitd", err)

	res, err := task.Build(buildkitd, wd, req)
	failIf("build", err)

	err = buildkitd.Cleanup()
	failIf("cleanup buildkitd", err)

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
