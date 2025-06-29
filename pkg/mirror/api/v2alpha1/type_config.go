package v2alpha1

import (
	"fmt"
	"regexp"
	"strings"

	"ocpack/pkg/mirror/image"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ImageSetConfiguration object kind.
const ImageSetConfigurationKind = "ImageSetConfiguration"

// ImageSetConfiguration configures image set creation.
type ImageSetConfiguration struct {
	metav1.TypeMeta `json:",inline"`
	// ImageSetConfigurationSpec defines the global configuration for an imageset.
	ImageSetConfigurationSpec `json:",inline"`
}

// ImageSetConfigurationSpec defines the global configuration for an imageset.
type ImageSetConfigurationSpec struct {
	// Mirror defines the configuration for content types within the imageset.
	Mirror Mirror `json:"mirror"`
	// ArchiveSize is the size of the segmented archive in GB
	ArchiveSize int64 `json:"archiveSize,omitempty"`
}

// DeleteImageSetConfiguration object kind.
const DeleteImageSetConfigurationKind = "DeleteImageSetConfiguration"

// DeleteImageSetConfiguration configures image set creation.
type DeleteImageSetConfiguration struct {
	metav1.TypeMeta `json:",inline"`
	// DeleteImageSetConfigurationSpec defines the global configuration for an imageset.
	DeleteImageSetConfigurationSpec `json:",inline"`
}

// DeleteImageSetConfigurationSpec defines the global configuration for a delete imageset.
// This is to ensure a clean differentiation between delete and mirror
// and is designed to avoid accidental deletion of images (when using imagesetconfig)
type DeleteImageSetConfigurationSpec struct {
	// Delete defines the configuration for content types within the imageset.
	Delete Delete `json:"delete"`
}

// Mirror defines the configuration for content types within the imageset.
type Mirror struct {
	// Platform defines the configuration for OpenShift and OKD platform types.
	Platform Platform `json:"platform,omitempty"`
	// Operators defines the configuration for Operator content types.
	Operators []Operator `json:"operators,omitempty"`
	// AdditionalImages defines the configuration for a list
	// of individual image content types.
	AdditionalImages []Image `json:"additionalImages,omitempty"`
	// Helm define the configuration for Helm content types.
	Helm Helm `json:"helm,omitempty"`
	// BlockedImages define a list of images that will be blocked
	// from the mirroring process if they exist in other content
	// types in the configuration.
	BlockedImages []Image `json:"blockedImages,omitempty"`
	// Samples defines the configuration for Sample content types.
	// This is currently not implemented.
	Samples []SampleImages `json:"samples,omitempty"`
}

// Delete defines the configuration for content types within the imageset.
type Delete struct {
	// Platform defines the configuration for OpenShift and OKD platform types.
	Platform Platform `json:"platform,omitempty"`
	// Operators defines the configuration for Operator content types.
	Operators []Operator `json:"operators,omitempty"`
	// AdditionalImages defines the configuration for a list
	// of individual image content types.
	AdditionalImages []Image `json:"additionalImages,omitempty"`
	// Helm define the configuration for Helm content types.
	Helm Helm `json:"helm,omitempty"`
	// Samples defines the configuration for Sample content types.
	// This is currently not implemented.
	Samples []SampleImages `json:"samples,omitempty"`
}

// Platform defines the configuration for OpenShift and OKD platform types.
type Platform struct {
	// Graph defines whether Cincinnati graph data will
	// downloaded and publish
	Graph bool `json:"graph,omitempty"`
	// Channels defines the configuration for individual
	// OCP and OKD channels
	Channels []ReleaseChannel `json:"channels,omitempty"`
	// Architectures defines one or more architectures
	// to mirror for the release image. This is defined at the
	// platform level to enable cross-channel upgrades.
	Architectures []string `json:"architectures,omitempty"`
	// This new field will allow the diskToMirror functionality
	// to copy from a release location on disk
	Release string `json:"release,omitempty"`
	// The kubeVirtContainer flag when set to true (default false)
	// will be used to extract the kubeVirtContainer image
	// from the release payload file 0000_50_installer_coreos-bootimages
	KubeVirtContainer bool `json:"kubeVirtContainer,omitempty"`
}

func (p Platform) DeepCopy() Platform {
	platformCopy := Platform{
		Graph: p.Graph,
	}

	platformCopy.Channels = make([]ReleaseChannel, len(p.Channels))
	copy(platformCopy.Channels, p.Channels)

	platformCopy.Architectures = make([]string, len(p.Architectures))
	copy(platformCopy.Architectures, p.Architectures)

	return platformCopy
}

// ReleaseChannel defines the configuration for individual
// OCP and OKD channels
type ReleaseChannel struct {
	Name string `json:"name"`
	// Type of the platform in the context of this tool.
	// See the PlatformType enum for options. OCP is the default.
	Type PlatformType `json:"type"`
	// MinVersion is minimum version in the
	// release channel to mirror
	MinVersion string `json:"minVersion,omitempty"`
	// MaxVersion is maximum version in the
	// release channel to mirror
	MaxVersion string `json:"maxVersion,omitempty"`
	// ShortestPath mode calculates the shortest path
	// between the min and mav version
	ShortestPath bool `json:"shortestPath,omitempty"`
	// Full mode set the MinVersion to the
	// first release in the channel and the MaxVersion
	// to the last release in the channel.
	Full bool `json:"full,omitempty"`
}

// IsHeadsOnly determine if the mode set mirrors only channel head.
// Setting MaxVersion will override this setting.
func (r ReleaseChannel) IsHeadsOnly() bool {
	return !r.Full
}

// Operator defines the configuration for operator catalog mirroring.
type Operator struct {
	// Mirror specific operator packages, channels, and versions, and their dependencies.
	// If HeadsOnly is true, these objects are mirrored on top of heads of all channels.
	// Otherwise, only these specific objects are mirrored.
	IncludeConfig `json:",inline"`
	// Catalog image to mirror. This image must be pullable and available for subsequent
	// pulls on later mirrors.
	// This image should be an exact image pin (registry/namespace/name@sha256:<hash>)
	// but is not required to be.
	Catalog string `json:"catalog"`
	// TargetCatalog replaces TargetName and allows for specifying the exact URL of the target
	// catalog, including any path-components (organization, namespace) of the target catalog's location
	// on the disconnected registry.
	// This answer some customers requests regarding restrictions on where images can be placed.
	// The targetCatalog field consists of an optional namespace followed by the target image name,
	// described in extended Backus–Naur form below:
	//     target-catalog = [namespace '/'] target-name
	//     target-name    = path-component
	//     namespace      = path-component ['/' path-component]*
	//     path-component = alpha-numeric [separator alpha-numeric]*
	//     alpha-numeric  = /[a-z0-9]+/
	//     separator      = /[_.]|__|[-]*/
	TargetCatalog string `json:"targetCatalog,omitempty"`
	// TargetTag is the tag the catalog image will be built with. If unset,
	// the catalog will be publish with the provided tag in the Catalog
	// field or a tag calculated from the partial digest.
	TargetTag string `json:"targetTag,omitempty"`
	// Full defines whether all packages within the catalog
	// or specified IncludeConfig will be mirrored or just channel heads.
	Full bool `json:"full,omitempty"`
	// SkipDependencies will not include dependencies
	// of bundles included in the diff if true.
	SkipDependencies bool `json:"skipDependencies,omitempty"`
	// path on disk for a template to use to complete catalogSource custom resource
	// generated by oc-mirror
	TargetCatalogSourceTemplate string `json:"targetCatalogSourceTemplate,omitempty"`
}

// GetUniqueName determines the catalog name that will
// be tracked in the metadata and built. This depends on what fields
// are set between Catalog, TargetName, and TargetTag.
func (o Operator) GetUniqueName() (string, error) {
	ctlgSpec, err := image.ParseRef(o.Catalog)
	if err != nil {
		return "", err
	}
	if o.TargetCatalog == "" && o.TargetTag == "" {
		return ctlgSpec.Reference, nil
	}

	if o.TargetTag != "" {
		ctlgSpec.Reference = strings.Replace(ctlgSpec.Reference, ctlgSpec.Tag, o.TargetTag, 1)
		ctlgSpec.ReferenceWithTransport = strings.Replace(ctlgSpec.ReferenceWithTransport, ctlgSpec.Tag, o.TargetTag, 1)
		ctlgSpec.Tag = o.TargetTag
	}
	if o.TargetCatalog != "" {
		if IsValidPathComponent(o.TargetCatalog) {
			ctlgSpec.Reference = strings.Replace(ctlgSpec.Reference, ctlgSpec.PathComponent, o.TargetCatalog, 1)
			ctlgSpec.ReferenceWithTransport = strings.Replace(ctlgSpec.ReferenceWithTransport, ctlgSpec.PathComponent, o.TargetCatalog, 1)
			ctlgSpec.PathComponent = o.TargetCatalog
		} else {
			return "", fmt.Errorf("targetCatalog: %s - value is not valid. It should not contain a tag or a digest. It is expected to be composed of 1 or more path components separated by /, where each path component is a set of alpha-numeric and  regexp (?:[._]|__|[-]*). For more, see https://github.com/containers/image/blob/main/docker/reference/regexp.go", o.TargetCatalog)
		}
	}
	return ctlgSpec.Reference, nil
}

func IsValidPathComponent(targetCatalog string) bool {
	pathComponentPattern := regexp.MustCompile(`^([a-z0-9]+((?:[._]|__|[-]*)[a-z0-9]+)*)(/([a-z0-9]+((?:[._]|__|[-]*)[a-z0-9]+)*))*$`)
	return pathComponentPattern.MatchString(targetCatalog)
}

// IsHeadsOnly determine if the mode set mirrors only channel heads of all packages in the catalog.
// Channels specified in DiffIncludeConfig will override this setting;
// heads will still be included, but prior versions may also be included.
func (o Operator) IsHeadsOnly() bool {
	return !o.Full
}

func (o Operator) IsFBCOCI() bool {
	return strings.HasPrefix(o.Catalog, "oci:")
}

// Helm defines the configuration for Helm chart download
// and image mirroring
type Helm struct {
	// Repositories are the Helm repositories containing the charts
	Repositories []Repository `json:"repositories,omitempty"`
	// Local is the configuration for locally stored helm charts
	Local []Chart `json:"local,omitempty"`
}

// Repository defines the configuration for a Helm repository.
type Repository struct {
	// URL is the url of the Helm repository
	URL string `json:"url"`
	// Name is the name of the Helm repository
	Name string `json:"name"`
	// Charts is a list of charts to pull from the repo
	Charts []Chart `json:"charts"`
}

// Chart is the information an individual Helm chart
type Chart struct {
	// Chart is the chart name as define
	// in the Chart.yaml or in the Helm repo.
	Name string `json:"name"`
	// Version is the chart version as define in the
	// Chart.yaml or in the Helm repo.
	Version string `json:"version,omitempty"`
	// Path defines the path on disk where the
	// chart is stored.
	// This is applicable for a local chart.
	Path string `json:"path,omitempty"`
	// ImagePaths are custom JSON paths for images location
	// in the helm manifest or templates
	ImagePaths []string `json:"imagePaths,omitempty"`
}

// Image contains image pull information.
type Image struct {
	// Name of the image. This should be an exact image pin (registry/namespace/name@sha256:<hash>)
	// but is not required to be.
	Name string `json:"name"`
}

// SampleImages define the configuration
// for Sameple content types.
// Not implemented.
type SampleImages struct {
	Image `json:",inline"`
}
