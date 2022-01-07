package task

// Request is the request payload sent from Concourse to execute the task.
//
// This is currently not really exercised by Concourse; it's a mock-up of what
// a future 'reusable tasks' design may look like.
type Request struct {
	ResponsePath string `json:"response_path"`
	Config       Config `json:"config"`
}

// Response is sent back to Concourse by writing this structure to the
// `response_path` specified in the request.
//
// This is also a mock-up. Right now it communicates the available outputs,
// which may be useful to assist pipeline authors in knowing what artifacts are
// available after a task excutes.
//
// In the future, pipeline authors may list which outputs they would like to
// propagate to the rest of the build plan, by specifying `outputs` or
// `output_mapping` like so:
//
//   task: build
//   outputs: [image]
//
//   task: build
//   output_mapping: {image: my-image}
//
// Outputs may also be 'cached', meaning their previous value will be present
// for subsequent runs of the task:
//
//   task: build
//   outputs: [image]
//   caches: [cache]
type Response struct {
	Outputs []string `json:"outputs"`
}

// Config contains the configuration for the task.
//
// In the future, when Concourse supports a 'reusable task' interface, this
// will be provided as a JSON request on `stdin`.
//
// For now, and for backwards-compatibility, we will also support taking values
// from task params (i.e. env), hence the use of `envconfig:`.
type Config struct {
	Debug bool `json:"debug" envconfig:"optional"`

	ContextDir     string `json:"context"              envconfig:"CONTEXT,optional"`
	DockerfilePath string `json:"dockerfile,omitempty" envconfig:"DOCKERFILE,optional"`
	BuildkitSSH    string `json:"buildkit_ssh"         envconfig:"optional"`

	Target            string   `json:"target"      envconfig:"optional"`
	TargetFile        string   `json:"target_file" envconfig:"optional"`
	AdditionalTargets []string `json:"additional_targets" envconfig:"ADDITIONAL_TARGETS,optional"`
	Push              string   `json:"push" envconfig:"optional"`

	BuildArgs     []string `json:"build_args"      envconfig:"optional"`
	BuildArgsFile string   `json:"build_args_file" envconfig:"optional"`

	RegistryMirrors []string `json:"registry_mirrors" envconfig:"REGISTRY_MIRRORS,optional"`

	Labels     []string `json:"labels"      envconfig:"optional"`
	LabelsFile string   `json:"labels_file" envconfig:"optional"`

	BuildkitSecrets map[string]string `json:"buildkit_secrets" envconfig:"optional"`

	// Unpack the OCI image into Concourse's rootfs/ + metadata.json image scheme.
	//
	// Theoretically this would go away if/when we standardize on OCI.
	UnpackRootfs bool `json:"unpack_rootfs" envconfig:"optional"`

	// Images to pre-load in order to avoid fetching at build time. Mapping from
	// build arg name to OCI image tarball path.
	//
	// Each image will be pre-loaded and a build arg will be set to a value
	// appropriate for setting in 'FROM ...'.
	ImageArgs []string `json:"image_args" envconfig:"optional"`

	AddHosts string `json:"add_hosts" envconfig:"BUILDKIT_ADD_HOSTS,optional"`

	ImagePlatform string `json:"image_platform" envconfig:"optional"`
}

// ImageMetadata is the schema written to manifest.json when producing the
// legacy Concourse image format (rootfs/..., metadata.json).
type ImageMetadata struct {
	Env  []string `json:"env"`
	User string   `json:"user"`
}
