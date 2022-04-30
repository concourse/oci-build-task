package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	task "github.com/concourse/oci-build-task"
	"github.com/vrischmann/envconfig"
)

const buildArgPrefix = "BUILD_ARG_"
const imageArgPrefix = "IMAGE_ARG_"
const labelPrefix = "LABEL_"

const buildkitSecretPrefix = "BUILDKIT_SECRET_"
const buildkitSecretTextPrefix = "BUILDKIT_SECRETTEXT_"

func main() {
	req := task.Request{
		ResponsePath: "/dev/null",
	}

	err := envconfig.Init(&req.Config)
	failIf("parse config from env", err)

	// envconfig does not support maps, so we initialize it here
	req.Config.BuildkitSecrets = make(map[string]string)

	// carry over BUILD_ARG_* and LABEL_* vars manually
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, buildArgPrefix) {
			req.Config.BuildArgs = append(
				req.Config.BuildArgs,
				strings.TrimPrefix(env, buildArgPrefix),
			)
		}

		if strings.HasPrefix(env, imageArgPrefix) {
			req.Config.ImageArgs = append(
				req.Config.ImageArgs,
				strings.TrimPrefix(env, imageArgPrefix),
			)
		}

		if strings.HasPrefix(env, labelPrefix) {
			req.Config.Labels = append(
				req.Config.Labels,
				strings.TrimPrefix(env, labelPrefix),
			)
		}

		if strings.HasPrefix(env, buildkitSecretPrefix) {
			seg := strings.SplitN(
				strings.TrimPrefix(env, buildkitSecretPrefix), "=", 2)

			req.Config.BuildkitSecrets[seg[0]] = seg[1]
		}

		if strings.HasPrefix(env, buildkitSecretTextPrefix) {
			seg := strings.SplitN(
				strings.TrimPrefix(env, buildkitSecretTextPrefix), "=", 2)

			// Q: Filter for environment variable names that are also legal shell variable names to disallow ../ etc?
			secretDir := filepath.Join(os.TempDir(), "buildkit-secrets")
			secretFile := filepath.Join(secretDir, seg[0])
			err := os.MkdirAll(secretDir, 0700)
			failIf("create secret directory", err)
			err = ioutil.WriteFile(secretFile, []byte(seg[1]), 0600)
			failIf("write to secret directory", err)
			req.Config.BuildkitSecrets[seg[0]] = secretFile
		}
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
