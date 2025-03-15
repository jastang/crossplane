/*
Copyright 2023 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package xpkg

import (
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/spf13/afero"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane/internal/xpkg"
)

const (
	errParseReference          = "error parsing remote reference"
	errCreateExtensionsTarball = "error creating package extensions tarball"
	errAppendExtensions        = "error appending package extensions to image"
)

// AfterApply constructs and binds context to any subcommands
// that have Run() methods that receive it.
func (c *appendCmd) AfterApply() error {
	// TODO: prompt and warn about re-signing if already signed
	c.fs = afero.NewOsFs()
	root, err := filepath.Abs(c.ExtensionsRoot)
	if err != nil {
		return err
	}
	paths := []string{}
	err = afero.Walk(c.fs, root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return err
	}
	c.absPaths = paths

	// Get default docker auth.
	c.keychain = remote.WithAuthFromKeychain(authn.NewMultiKeychain(authn.DefaultKeychain))

	// Make sure the ref parses properly
	ref, err := name.ParseReference(c.RemoteRef)
	if err != nil {
		return errors.Wrap(err, errParseReference)
	}
	// TODO: maybe don't need this
	c.indexRef = ref

	c.appender = xpkg.NewAppender(
		c.fs,
		paths,
		c.keychain,
		c.indexRef,
	)

	return nil
}

// buildCmd builds a crossplane package.
type appendCmd struct {
	// Arguments
	RemoteRef string `arg:"" help:"The fully qualitifed remote image reference" required:""`

	// Flags. Keep sorted alphabetically.
	ExtensionsRoot string `help:"An optional directory of arbitrary files for additional consumers of the package." default:"./extensions" placeholder:"PATH" type:"path"`

	// Internal state. These aren't part of the user-exposed CLI structure.
	fs       afero.Fs
	absPaths []string
	indexRef name.Reference
	keychain remote.Option
	appender *xpkg.Appender
}

func (c *appendCmd) Help() string {
	return `
This command creates a tarball from a local directory of additional package
assets, such as images or documentation, and appends them to a remote image.

Examples:

  # Add all files under an "/extensions" folder to a remote image.
  crossplane xpkg append --extensions-root=./extensions my-registry/my-organization/my-repo@sha256:<digest>

`
}

// Run executes the append command.
func (c *appendCmd) Run(logger logging.Logger) error {
	extLayer, err := xpkg.LayerFromFiles(c.absPaths, c.fs)
	if err != nil {
		return errors.Wrap(err, errCreateExtensionsTarball)
	}
	if err = c.appender.Append(c.indexRef, extLayer, c.keychain); err != nil {
		return errors.Wrap(err, errAppendExtensions)
	}
	return nil
}
