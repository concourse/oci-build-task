package task

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func Build(buildkitd *Buildkitd, outputsDir string, req Request) (Response, error) {
	if req.Config.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	cfg := req.Config
	err := sanitize(&cfg)
	if err != nil {
		return Response{}, errors.Wrap(err, "config")
	}

	cacheDir := filepath.Join(outputsDir, "cache")

	res := Response{
		Outputs: []string{"image", "cache"},
	}

	dockerfileDir := filepath.Dir(cfg.DockerfilePath)
	dockerfileName := filepath.Base(cfg.DockerfilePath)

	buildctlArgs := []string{
		"build",
		"--progress", "plain",
		"--frontend", "dockerfile.v0",
		"--local", "context=" + cfg.ContextDir,
		"--local", "dockerfile=" + dockerfileDir,
		"--opt", "filename=" + dockerfileName,
	}

	for _, arg := range cfg.Labels {
		buildctlArgs = append(buildctlArgs,
			"--opt", "label:"+arg,
		)
	}

	for _, arg := range cfg.BuildArgs {
		buildctlArgs = append(buildctlArgs,
			"--opt", "build-arg:"+arg,
		)
	}

	if len(req.Config.ImageArgs) > 0 {
		imagePaths := map[string]string{}
		for _, arg := range req.Config.ImageArgs {
			segs := strings.SplitN(arg, "=", 2)
			imagePaths[segs[0]] = segs[1]
		}

		registry, err := LoadRegistry(imagePaths)
		if err != nil {
			return Response{}, fmt.Errorf("create local image registry: %w", err)
		}

		port, err := ServeRegistry(registry)
		if err != nil {
			return Response{}, fmt.Errorf("create local image registry: %w", err)
		}

		for _, arg := range registry.BuildArgs(port) {
			buildctlArgs = append(buildctlArgs,
				"--opt", "build-arg:"+arg,
			)
		}
	}

	if _, err := os.Stat(cacheDir); err == nil {
		buildctlArgs = append(buildctlArgs,
			"--export-cache", "type=local,mode=max,dest="+cacheDir,
		)
	}

	for id, src := range cfg.BuildkitSecrets {
		buildctlArgs = append(buildctlArgs,
			"--secret", "id="+id+",src="+src,
		)
	}

	var builds [][]string
	var targets []string
	var imagePaths []string

	for _, t := range cfg.AdditionalTargets {
		// prevent re-use of the buildctlArgs slice as it is appended to later on,
		// and that would clobber args for all targets if the slice was re-used
		targetArgs := make([]string, len(buildctlArgs))
		copy(targetArgs, buildctlArgs)

		targetArgs = append(targetArgs, "--opt", "target="+t)

		targetDir := filepath.Join(outputsDir, t)

		if _, err := os.Stat(targetDir); err == nil {
			imagePath := filepath.Join(targetDir, "image.tar")
			imagePaths = append(imagePaths, imagePath)

			targetArgs = append(targetArgs,
				"--output", "type=docker,dest="+imagePath,
			)
		}

		builds = append(builds, targetArgs)
		targets = append(targets, t)
	}

	finalTargetDir := filepath.Join(outputsDir, "image")
	if _, err := os.Stat(finalTargetDir); err == nil {
		imagePath := filepath.Join(finalTargetDir, "image.tar")
		imagePaths = append(imagePaths, imagePath)

		buildctlArgs = append(buildctlArgs,
			"--output", "type=docker,dest="+imagePath,
		)
	}

	if cfg.Target != "" {
		buildctlArgs = append(buildctlArgs,
			"--opt", "target="+cfg.Target,
		)
	}

	if cfg.Push != "" {
		buildctlArgs = append(buildctlArgs,
			"--output", fmt.Sprintf("type=image,name=%s,push=true", cfg.Push))
	}

	if cfg.AddHosts != "" {
		buildctlArgs = append(buildctlArgs,
			"--opt", "add-hosts="+cfg.AddHosts,
		)
	}

	if cfg.BuildkitSSH != "" {
		buildctlArgs = append(buildctlArgs,
			"--ssh", cfg.BuildkitSSH,
		)
	}

	if req.Config.ImagePlatform != "" {
		buildctlArgs = append(buildctlArgs,
			"--opt", "platform="+req.Config.ImagePlatform,
		)
	}

	builds = append(builds, buildctlArgs)
	targets = append(targets, "")

	for i, args := range builds {
		if i > 0 {
			fmt.Fprintln(os.Stderr)
		}

		targetName := targets[i]
		if targetName == "" {
			logrus.Info("building image")
		} else {
			logrus.Infof("building target '%s'", targetName)
		}

		if _, err := os.Stat(filepath.Join(cacheDir, "index.json")); err == nil {
			args = append(args,
				"--import-cache", "type=local,src="+cacheDir,
			)
		}

		logrus.Debugf("running buildctl %s", strings.Join(args, " "))

		err = buildctl(buildkitd.Addr, os.Stdout, args...)
		if err != nil {
			return Response{}, errors.Wrap(err, "build")
		}
	}

	for _, imagePath := range imagePaths {
		image, err := tarball.ImageFromPath(imagePath, nil)
		if err != nil {
			return Response{}, errors.Wrap(err, "open oci image")
		}

		outputDir := filepath.Dir(imagePath)

		err = writeDigest(outputDir, image)
		if err != nil {
			return Response{}, err
		}

		if req.Config.UnpackRootfs {
			err = unpackRootfs(outputDir, image, cfg)
			if err != nil {
				return Response{}, errors.Wrap(err, "unpack rootfs")
			}
		}
	}

	return res, nil
}

func writeDigest(dest string, image v1.Image) error {
	digestPath := filepath.Join(dest, "digest")

	manifest, err := image.Manifest()
	if err != nil {
		return errors.Wrap(err, "get image digest")
	}

	err = ioutil.WriteFile(digestPath, []byte(manifest.Config.Digest.String()), 0644)
	if err != nil {
		return errors.Wrap(err, "write digest file")
	}

	return nil
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

	err = json.NewEncoder(meta).Encode(ImageMetadata{
		Env:  cfg.Config.Env,
		User: cfg.Config.User,
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

	if cfg.LabelsFile != "" {
		Labels, err := ioutil.ReadFile(cfg.LabelsFile)
		if err != nil {
			return errors.Wrap(err, "read labels file")
		}

		for _, arg := range strings.Split(string(Labels), "\n") {
			if len(arg) == 0 {
				// skip blank lines
				continue
			}

			cfg.Labels = append(cfg.Labels, arg)
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
