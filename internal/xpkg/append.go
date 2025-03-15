package xpkg

import (
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/spf13/afero"
)

const (
	errGetManifestList          = "error retrieving manifest list"
	errManifestDigest           = "error getting manifest digest"
	errWriteIndex               = "error writing image index to remote ref"
	errReadIndex                = "error reading remote index"
	errCreateExtensionsManifest = "error creating package extensions manifest"
	errNotIndex                 = "expected image index but a manifest was found"
)

type Appender struct {
	fs        afero.Fs
	paths     []string
	keychain  remote.Option
	remoteImg name.Reference
}

// TODO maybe interface for Append()

func NewAppender(fs afero.Fs, paths []string, keychain remote.Option, remoteImg name.Reference) *Appender {
	return &Appender{
		fs:        fs,
		paths:     paths,
		keychain:  keychain,
		remoteImg: remoteImg,
	}
}

func (a *Appender) Append(indexRef name.Reference, layer v1.Layer, keychain remote.Option) error {
	// Ensure we are working with an image index
	// We do not currently support converting a single manifest into an index.
	index, err := remote.Index(indexRef, keychain)
	if err != nil {
		return errors.Wrap(err, errReadIndex)
	}

	// Create the extensions manifest
	extManifest, err := mutate.Append(
		empty.Image,
		mutate.Addendum{
			Layer: layer,
			Annotations: map[string]string{
				"io.crossplane.xpkg": "xpkg-extensions", //TODO: const
			},
		})
	if err != nil {
		return errors.Wrap(err, errCreateExtensionsManifest)
	}

	extManifestDigest, err := extManifest.Digest()
	if err != nil {
		return errors.Wrap(err, errManifestDigest)
	}

	// No-op if there already exists a manifest with the same digest in the index.
	manifestList, err := index.IndexManifest()
	if err != nil {
		return errors.Wrap(err, errGetManifestList)
	}

	for _, manifest := range manifestList.Manifests {
		if manifest.Digest.String() == extManifestDigest.String() {
			return nil
		}
	}

	// Create the new index to replace
	newIndex := mutate.AppendManifests(index, mutate.IndexAddendum{
		Add: extManifest,
		Descriptor: v1.Descriptor{
			MediaType: types.OCIManifestSchema1,
			Digest:    extManifestDigest,
			Size:      0,
			Annotations: map[string]string{
				"io.crossplane.xpkg": "package extra assets manifest",
			},
		},
	})

	err = remote.WriteIndex(indexRef, newIndex, keychain)
	if err != nil {
		return errors.Wrap(err, errWriteIndex)
	}

	return nil
}
