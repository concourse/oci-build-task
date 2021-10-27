package task

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
)

type ImageArg struct {
	Image   v1.Image
	ArgName string
}
type LocalRegistry map[string]ImageArg

func LoadRegistry(imagePaths map[string]string) (LocalRegistry, error) {
	images := LocalRegistry{}
	for name, path := range imagePaths {
		image, err := tarball.ImageFromPath(path, nil)
		if err != nil {
			return nil, fmt.Errorf("image from path: %w", err)
		}

		images[strings.ToLower(name)] = ImageArg{Image: image, ArgName: name}
	}

	return images, nil
}

func ServeRegistry(reg LocalRegistry) (string, error) {
	router := httprouter.New()
	router.GET("/v2/:name/manifests/:ref", reg.GetManifest)
	router.GET("/v2/:name/blobs/:digest", reg.GetBlob)

	router.NotFound = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logrus.WithFields(logrus.Fields{
			"method": r.Method,
			"path":   r.URL.Path,
		}).Warnf("unknown request")
	})

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", fmt.Errorf("listen: %w", err)
	}

	go http.Serve(listener, router)

	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return "", fmt.Errorf("split registry host/port: %w", err)
	}

	return port, nil
}

func (registry LocalRegistry) BuildArgs(port string) []string {
	var buildArgs []string
	for name, image := range registry {
		buildArgs = append(buildArgs, fmt.Sprintf("%s=localhost:%s/%s", image.ArgName, port, name))
	}

	return buildArgs
}

func (registry LocalRegistry) GetManifest(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	name := p.ByName("name")
	ref := p.ByName("ref")

	logrus.WithFields(logrus.Fields{
		"accept": r.Header["Accept"],
	}).Debugf("get manifest for %s at %s", name, ref)

	img, found := registry[name]
	if !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	image := img.Image

	mt, err := image.MediaType()
	if err != nil {
		logrus.Errorf("failed to get media type: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	blob, err := image.RawManifest()
	if err != nil {
		logrus.Errorf("failed to get manifest: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	digest, err := image.Digest()
	if err != nil {
		logrus.Errorf("failed to get digest: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", string(mt))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(blob)))
	w.Header().Set("Docker-Content-Digest", digest.String())

	if r.Method == "HEAD" {
		return
	}

	_, err = w.Write(blob)
	if err != nil {
		logrus.Errorf("write manifest blob: %s", err)
		return
	}
}

func (registry LocalRegistry) GetBlob(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	name := p.ByName("name")
	dig := p.ByName("digest")

	logrus.WithFields(logrus.Fields{
		"accept": r.Header["Accept"],
	}).Debugf("get blob %s", dig)

	img, found := registry[name]
	if !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	image := img.Image

	hash, err := v1.NewHash(dig)
	if err != nil {
		logrus.Errorf("failed to parse digest: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	cfgHash, err := image.ConfigName()
	if err != nil {
		logrus.Errorf("failed to get config hash: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if hash == cfgHash {
		manifest, err := image.Manifest()
		if err != nil {
			logrus.Errorf("get image manifest: %s", err)
			return
		}

		cfgBlob, err := image.RawConfigFile()
		if err != nil {
			logrus.Errorf("failed to get config file: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", string(manifest.Config.MediaType))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(cfgBlob)))

		if r.Method == "HEAD" {
			return
		}

		_, err = w.Write(cfgBlob)
		if err != nil {
			logrus.Errorf("write config blob: %s", err)
			return
		}

		return
	}

	layer, err := image.LayerByDigest(hash)
	if err != nil {
		logrus.Errorf("failed to get layer: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	size, err := layer.Size()
	if err != nil {
		logrus.Errorf("failed to get layer size: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	mt, err := layer.MediaType()
	if err != nil {
		logrus.Errorf("failed to get layer media type: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", string(mt))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))

	if r.Method == "HEAD" {
		return
	}

	blob, err := layer.Compressed()
	if err != nil {
		logrus.Errorf("failed to read layer: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_, err = io.Copy(w, blob)
	if err != nil {
		logrus.Errorf("write blob: %s", err)
		return
	}
}
