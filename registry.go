package task

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/containers/image/v5/docker/archive"
	"github.com/containers/image/v5/types"
	"github.com/julienschmidt/httprouter"
	"github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
)

type LocalRegistry map[string]types.ImageSource

func LoadRegistry(imagePaths map[string]string) (LocalRegistry, error) {
	images := LocalRegistry{}
	for name, path := range imagePaths {
		ref, err := archive.NewReference(path, nil)
		if err != nil {
			return nil, fmt.Errorf("new reference: %w", err)
		}

		src, err := ref.NewImageSource(context.TODO(), nil)
		if err != nil {
			return nil, fmt.Errorf("new image source: %w", err)
		}

		images[name] = src
	}

	return images, nil
}

func ServeRegistry(reg LocalRegistry) (string, error) {
	router := httprouter.New()
	router.GET("/v2/:name/manifests/:ignored", reg.GetManifest)
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
	for name := range registry {
		buildArgs = append(buildArgs, fmt.Sprintf("%s=localhost:%s/%s", name, port, name))
	}

	return buildArgs
}

func (registry LocalRegistry) GetManifest(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	name := p.ByName("name")

	src, found := registry[name]
	if !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	blob, mt, err := src.GetManifest(r.Context(), nil)
	if err != nil {
		logrus.Errorf("failed to get manifest: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", mt)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(blob)))
	w.Header().Set("Docker-Content-Digest", digest.FromBytes(blob).String())

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

	src, found := registry[name]
	if !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	blob, size, err := src.GetBlob(r.Context(), types.BlobInfo{
		Digest: digest.Digest(p.ByName("digest")),
	}, nil)
	if err != nil {
		logrus.Errorf("failed to get blob: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))

	if r.Method == "HEAD" {
		return
	}

	_, err = io.Copy(w, blob)
	if err != nil {
		logrus.Errorf("write blob: %s", err)
		return
	}
}
