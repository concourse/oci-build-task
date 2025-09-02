package task

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Q: Audit name to not include "/"?
func StoreSecret(req *Request, name, value string) error {
	secretDir := filepath.Join(os.TempDir(), "buildkit-secrets")
	secretFile := filepath.Join(secretDir, name)
	err := os.MkdirAll(secretDir, 0700)
	if err != nil {
		return fmt.Errorf("unable to create secret directory: %w", err)
	}
	err = os.WriteFile(secretFile, []byte(value), 0600)
	if err != nil {
		return fmt.Errorf("unable to write secret to file: %w", err)
	}
	if req.Config.BuildkitSecrets == nil {
		req.Config.BuildkitSecrets = make(map[string]string, 1)
	}
	req.Config.BuildkitSecrets[name] = secretFile
	return nil
}

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

	if len(cfg.ImageArgs) > 0 {
		imagePaths := map[string]string{}
		for _, arg := range cfg.ImageArgs {
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

	outputType := "docker"
	if cfg.OutputOCI {
		outputType = "oci"
	}

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
				"--output", "type="+outputType+",dest="+imagePath,
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
			"--output", "type="+outputType+",dest="+imagePath,
		)
	}

	if cfg.Target != "" {
		buildctlArgs = append(buildctlArgs,
			"--opt", "target="+cfg.Target,
		)
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

	if cfg.ImagePlatform != "" {
		buildctlArgs = append(buildctlArgs,
			"--opt", "platform="+cfg.ImagePlatform,
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

	if cfg.OutputOCI {
		err = loadOciImages(imagePaths, req)
		if err != nil {
			return Response{}, err
		}
	} else {
		err = loadImages(imagePaths, req)
		if err != nil {
			return Response{}, err
		}
	}

	return res, nil
}

func loadImages(imagePaths []string, req Request) error {
	for _, imagePath := range imagePaths {
		image, err := tarball.ImageFromPath(imagePath, nil)
		if err != nil {
			return errors.Wrap(err, "open oci image")
		}

		outputDir := filepath.Dir(imagePath)

		m, err := image.Manifest()
		if err != nil {
			return errors.Wrap(err, "get image manifest")
		}

		err = writeDigest(outputDir, m.Config.Digest)
		if err != nil {
			return err
		}

		if req.Config.UnpackRootfs {
			err = unpackRootfs(outputDir, image, req.Config)
			if err != nil {
				return errors.Wrap(err, "unpack rootfs")
			}
		}
	}
	return nil
}

func loadOciImages(imagePaths []string, req Request) error {
	for _, imagePath := range imagePaths {
		_, err := os.Stat(imagePath)
		if err != nil {
			return errors.Wrapf(err, "image path %s not valid", imagePath)
		}

		// go-containerregistry does not currently have support for loading a OCI formated
		// image from a tarball, so we decompress it before doing anything.
		targetDir := filepath.Dir(imagePath)
		imageDir := filepath.Join(targetDir, "image")
		logrus.Infof("decompressing OCI image tar to: %s", imageDir)
		err = os.MkdirAll(imageDir, 0700)
		if err != nil {
			return errors.Wrapf(err, "unable to create image dir %s", imageDir)
		}
		run(os.Stdout, "tar", "-xvf", imagePath, "-C", imageDir)

		l, err := layout.ImageIndexFromPath(imageDir)
		if err != nil {
			return errors.Wrapf(err, "failed to load %s as OCI layout", imagePath)
		}

		m, err := l.IndexManifest()
		if err != nil {
			return errors.Wrap(err, "error getting index manifest")
		}

		manifest := m.Manifests[0]

		outputDir := filepath.Dir(imagePath)

		err = writeDigest(outputDir, manifest.Digest)
		if err != nil {
			return err
		}
	}

	return nil
}

func writeDigest(dest string, digest v1.Hash) error {
	digestPath := filepath.Join(dest, "digest")

	err := os.WriteFile(digestPath, []byte(digest.String()), 0644)
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
		target, err := os.ReadFile(cfg.TargetFile)
		if err != nil {
			return errors.Wrap(err, "read target file")
		}

		cfg.Target = strings.TrimSpace(string(target))
	}

	if cfg.BuildArgsFile != "" {
		buildArgs, err := os.ReadFile(cfg.BuildArgsFile)
		if err != nil {
			return errors.Wrap(err, "read build args file")
		}

		if strings.HasSuffix(cfg.BuildArgsFile, ".yml") || strings.HasSuffix(cfg.BuildArgsFile, ".yaml") {
			var buildArgsData map[string]string
			err = yaml.Unmarshal(buildArgs, &buildArgsData)
			if err != nil {
				return errors.Wrap(err, "read build args yaml file")
			}
			for key, arg := range buildArgsData {
				cfg.BuildArgs = append(cfg.BuildArgs, key+"="+arg)
			}
		} else {
			for _, arg := range strings.Split(string(buildArgs), "\n") {
				if len(arg) == 0 {
					// skip blank lines
					continue
				}

				cfg.BuildArgs = append(cfg.BuildArgs, arg)
			}
		}
	}

	if cfg.LabelsFile != "" {
		Labels, err := os.ReadFile(cfg.LabelsFile)
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

	// When multiple image platforms are targetted for building, we must output
	// in OCI format. The default "docker" format does not support exporting
	// multi-platform images
	if strings.Contains(cfg.ImagePlatform, ",") {
		cfg.OutputOCI = true
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
