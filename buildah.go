package buildah

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/containers/storage/pkg/ioutils"
	"github.com/containers/storage/storage"
)

const (
	// Package is the name of this package, used in help output and to
	// identify working containers.
	Package       = "buildah"
	containerType = Package + " 0.0.0"
	stateFile     = Package + ".json"
)

// Builder objects are used to represent containers which are being used to
// build images.  They also carry potential updates which will be applied to
// the image's configuration when the container's contents are used to build an
// image.
type Builder struct {
	store storage.Store

	// Type is used to help identify a build container's metadata.  It
	// should not be modified.
	Type string `json:"type"`
	// FromImage is the name of the source image which was used to create
	// the container, if one was used.  It should not be modified.
	FromImage string `json:"image,omitempty"`
	// Config is the source image's configuration.  It should not be
	// modified.
	Config []byte `json:"config,omitempty"`
	// Manifest is the source image's manifest.  It should not be modified.
	Manifest []byte `json:"manifest,omitempty"`

	// Container is the name of the build container.  It should not be modified.
	Container string `json:"container-name,omitempty"`
	// ContainerID is the ID of the build container.  It should not be modified.
	ContainerID string `json:"container-id,omitempty"`
	// MountPoint is the last location where the container's root
	// filesystem was mounted.  It should not be modified.
	MountPoint string `json:"mountpoint,omitempty"`
	// Mounts is a list of places where the container's root filesystem has
	// been mounted.  It should not be modified.
	Mounts []string `json:"mounts,omitempty"`
	// Links is a list of symbolic links to the currently-mounted container
	// root filesystem.  They will be removed when the container is
	// unmounted.  It should not be modified.
	Links []string `json:"links,omitempty"`

	// Annotations is a set of key-value pairs which is stored in the
	// image's manifest.
	Annotations map[string]string `json:"annotations,omitempty"`

	// CreatedBy is a description of how this container was built.
	CreatedBy string `json:"created-by,omitempty"`
	// OS is the operating system for which binaries in the image are
	// built.  The default is the current OS.
	OS string `json:"os,omitempty"`
	// Architecture is the type of processor for which binaries in the
	// image are built.  The default is the current architecture.
	Architecture string `json:"arch,omitempty"`
	// Maintainer is the point of contact for this container.
	Maintainer string `json:"maintainer,omitempty"`
	// User is the user as whom commands are run in the container.
	User string `json:"user,omitempty"`
	// Workdir is the default working directory for commands started in the
	// container.
	Workdir string `json:"workingdir,omitempty"`
	// Env is a list of environment variables to set for the container, in
	// the form NAME=VALUE.
	Env []string `json:"env,omitempty"`
	// Cmd sets a default command to run in containers based on the image.
	Cmd []string `json:"cmd,omitempty"`
	// Entrypoint is an entry point for containers based on the image.
	Entrypoint []string `json:"entrypoint,omitempty"`
	// Expose is a map keyed by specifications of ports to expose when a
	// container based on the image is run.
	Expose map[string]interface{} `json:"expose,omitempty"`
	// Labels is a set of key-value pairs which is stored in the
	// image's configuration.
	Labels map[string]string `json:"labels,omitempty"`
	// Volumes is a list of data volumes which will be created in
	// containers based on the image.
	Volumes []string `json:"volumes,omitempty"`
	// Arg is a set of build-time variables.
	Arg map[string]string `json:"arg,omitempty"`
}

// BuilderOptions are used to initialize a Builder.
type BuilderOptions struct {
	// FromImage is the name of the image which should be used as the
	// starting point for the container.  It can be set to an empty value
	// or "scratch" to indicate that the container should not be based on
	// an image.
	FromImage string
	// Container is a desired name for the build container.
	Container string
	// PullIfMissing signals to NewBuilder() that it should pull the image
	// if it is not present in the local Store.
	PullIfMissing bool
	// PullAlways signals to NewBuilder() that it should pull the source
	// image before creating the container.
	PullAlways bool
	// Registry is a value which is prepended to the image's name, if it
	// needs to be pulled and the image name alone can not be resolved to a
	// reference to a source image.
	Registry string
	// Mount signals to NewBuilder() that the container should be mounted
	// immediately.
	Mount bool
	// Link specifies a location for a symbolic link which should be
	// created if the container is mounted immediately.
	Link string
	// SignaturePolicyPath specifies an override location for the signature
	// policy which should be used for verifying the new image as it is
	// being written.  Except in specific circumstances, no value should be
	// specified, indicating that the shared, system-wide default policy
	// should be used.
	SignaturePolicyPath string
}

// ImportOptions are used to initialize a Builder.
type ImportOptions struct {
	// Container is the name of the build container.
	Container string
	// SignaturePolicyPath specifies an override location for the signature
	// policy which should be used for verifying the new image as it is
	// being written.  Except in specific circumstances, no value should be
	// specified, indicating that the shared, system-wide default policy
	// should be used.
	SignaturePolicyPath string
}

// NewBuilder creates a new build container.
func NewBuilder(store storage.Store, options BuilderOptions) (*Builder, error) {
	return newBuilder(store, options)
}

// ImportBuilder creates a new build configuration using an already-present
// container.
func ImportBuilder(store storage.Store, options ImportOptions) (*Builder, error) {
	return importBuilder(store, options)
}

// OpenBuilder loads information about a build container given its name or ID.
func OpenBuilder(store storage.Store, container string) (*Builder, error) {
	cdir, err := store.GetContainerDirectory(container)
	if err != nil {
		return nil, err
	}
	buildstate, err := ioutil.ReadFile(filepath.Join(cdir, stateFile))
	if err != nil {
		return nil, err
	}
	b := &Builder{}
	err = json.Unmarshal([]byte(buildstate), &b)
	if err != nil {
		return nil, err
	}
	if b.Type != containerType {
		return nil, fmt.Errorf("container is not a %s container", Package)
	}
	b.store = store
	return b, nil
}

// OpenBuilderByPath loads information about a build container given either a
// path to the container's root filesystem or a the path of a symbolic link
// which points to it.
func OpenBuilderByPath(store storage.Store, path string) (*Builder, error) {
	containers, err := store.Containers()
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	builderMatchesPath := func(b *Builder, path string) bool {
		if b.MountPoint == path {
			return true
		}
		for _, m := range b.Mounts {
			if m == path {
				return true
			}
		}
		for _, l := range b.Links {
			if l == path {
				return true
			}
		}
		return false
	}
	for _, container := range containers {
		buildstate, err := store.GetMetadata(container.ID)
		if err != nil {
			return nil, err
		}
		b := &Builder{}
		err = json.Unmarshal([]byte(buildstate), &b)
		if err == nil && b.Type == containerType && builderMatchesPath(b, abs) {
			b.store = store
			return b, nil
		}
	}
	return nil, storage.ErrContainerUnknown
}

// Save saves the builder's current state to the build container's metadata.
// This should not need to be called directly, as other methods of the Builder
// object take care of saving their state.
func (b *Builder) Save() error {
	buildstate, err := json.Marshal(b)
	if err != nil {
		return err
	}
	cdir, err := b.store.GetContainerDirectory(b.ContainerID)
	if err != nil {
		return err
	}
	return ioutils.AtomicWriteFile(filepath.Join(cdir, stateFile), buildstate, 0600)
}
