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
	Repository string `json:"repository"`

	Tag     string `json:"tag"`
	TagFile string `json:"tag_file" envconfig:"optional"`

	ContextPath    string `json:"context"              envconfig:"CONTEXT,optional"`
	DockerfilePath string `json:"dockerfile,omitempty" envconfig:"DOCKERFILE,optional"`

	Target     string `json:"target"      envconfig:"optional"`
	TargetFile string `json:"target_file" envconfig:"optional"`

	OutputType string `json:"output_type" envconfig:"optional"`

	BuildArgs     []string `json:"build_args"      envconfig:"optional"`
	BuildArgsFile string   `json:"build_args_file" envconfig:"optional"`
}
