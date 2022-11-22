package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	v1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/utils/pointer"
)

// Label keys.
const (
	LabelKeyNamePrefixForComposed = "crossplane.io/composite"
	LabelKeyClaimName             = "crossplane.io/claim-name"
	LabelKeyClaimNamespace        = "crossplane.io/claim-namespace"
)
const (
	CategoryClaim     = "claim"
	CategoryComposite = "composite"
)

const (
	errFmtGetProps             = "cannot get %q properties from validation schema"
	errParseValidation         = "cannot parse validation schema"
	errInvalidClaimNames       = "invalid resource claim names"
	errMissingClaimNames       = "missing names"
	errFmtConflictingClaimName = "%q conflicts with composite resource name"
)

var PropagateSpecProps = []string{"compositionRef", "compositionSelector", "compositionRevisionRef", "compositionUpdatePolicy"}

func BaseProps() *extv1.JSONSchemaProps {
	return &extv1.JSONSchemaProps{
		Type:     "object",
		Required: []string{"spec"},
		Properties: map[string]extv1.JSONSchemaProps{
			"apiVersion": {
				Type: "string",
			},
			"kind": {
				Type: "string",
			},
			"metadata": {
				// NOTE(muvaf): api-server takes care of validating
				// metadata.
				Type: "object",
			},
			"spec": {
				Type:       "object",
				Properties: map[string]extv1.JSONSchemaProps{},
			},
			"status": {
				Type:       "object",
				Properties: map[string]extv1.JSONSchemaProps{},
			},
		},
	}
}

// CompositeResourceSpecProps is a partial OpenAPIV3Schema for the spec fields
// that Crossplane expects to be present for all defined infrastructure
// resources.
func CompositeResourceSpecProps() map[string]extv1.JSONSchemaProps {
	return map[string]extv1.JSONSchemaProps{
		"compositionRef": {
			Type:     "object",
			Required: []string{"name"},
			Properties: map[string]extv1.JSONSchemaProps{
				"name": {Type: "string"},
			},
		},
		"compositionSelector": {
			Type:     "object",
			Required: []string{"matchLabels"},
			Properties: map[string]extv1.JSONSchemaProps{
				"matchLabels": {
					Type: "object",
					AdditionalProperties: &extv1.JSONSchemaPropsOrBool{
						Allows: true,
						Schema: &extv1.JSONSchemaProps{Type: "string"},
					},
				},
			},
		},
		"compositionRevisionRef": {
			Type:     "object",
			Required: []string{"name"},
			Properties: map[string]extv1.JSONSchemaProps{
				"name": {Type: "string"},
			},
			Description: "Alpha: This field may be deprecated or changed without notice.",
		},
		"compositionUpdatePolicy": {
			Type: "string",
			Enum: []extv1.JSON{
				{Raw: []byte(`"Automatic"`)},
				{Raw: []byte(`"Manual"`)},
			},
			Default:     &extv1.JSON{Raw: []byte(`"Automatic"`)},
			Description: "Alpha: This field may be deprecated or changed without notice.",
		},
		"claimRef": {
			Type:     "object",
			Required: []string{"apiVersion", "kind", "namespace", "name"},
			Properties: map[string]extv1.JSONSchemaProps{
				"apiVersion": {Type: "string"},
				"kind":       {Type: "string"},
				"namespace":  {Type: "string"},
				"name":       {Type: "string"},
			},
		},
		"resourceRefs": {
			Type: "array",
			Items: &extv1.JSONSchemaPropsOrArray{
				Schema: &extv1.JSONSchemaProps{
					Type: "object",
					Properties: map[string]extv1.JSONSchemaProps{
						"apiVersion": {Type: "string"},
						"name":       {Type: "string"},
						"kind":       {Type: "string"},
					},
					Required: []string{"apiVersion", "kind"},
				},
			},
		},
		"publishConnectionDetailsTo": {
			Type:     "object",
			Required: []string{"name"},
			Properties: map[string]extv1.JSONSchemaProps{
				"name": {Type: "string"},
				"configRef": {
					Type:    "object",
					Default: &extv1.JSON{Raw: []byte(`{"name": "default"}`)},
					Properties: map[string]extv1.JSONSchemaProps{
						"name": {
							Type: "string",
						},
					},
				},
				"metadata": {
					Type: "object",
					Properties: map[string]extv1.JSONSchemaProps{
						"labels": {
							Type: "object",
							AdditionalProperties: &extv1.JSONSchemaPropsOrBool{
								Allows: true,
								Schema: &extv1.JSONSchemaProps{Type: "string"},
							},
						},
						"annotations": {
							Type: "object",
							AdditionalProperties: &extv1.JSONSchemaPropsOrBool{
								Allows: true,
								Schema: &extv1.JSONSchemaProps{Type: "string"},
							},
						},
						"type": {
							Type: "string",
						},
					},
				},
			},
		},
		"writeConnectionSecretToRef": {
			Type:     "object",
			Required: []string{"name", "namespace"},
			Properties: map[string]extv1.JSONSchemaProps{
				"name":      {Type: "string"},
				"namespace": {Type: "string"},
			},
		},
	}
}

// CompositeResourceClaimSpecProps is a partial OpenAPIV3Schema for the spec
// fields that Crossplane expects to be present for all published infrastructure
// resources.
func CompositeResourceClaimSpecProps() map[string]extv1.JSONSchemaProps {
	return map[string]extv1.JSONSchemaProps{
		"compositionRef": {
			Type:     "object",
			Required: []string{"name"},
			Properties: map[string]extv1.JSONSchemaProps{
				"name": {Type: "string"},
			},
		},
		"compositionSelector": {
			Type:     "object",
			Required: []string{"matchLabels"},
			Properties: map[string]extv1.JSONSchemaProps{
				"matchLabels": {
					Type: "object",
					AdditionalProperties: &extv1.JSONSchemaPropsOrBool{
						Allows: true,
						Schema: &extv1.JSONSchemaProps{Type: "string"},
					},
				},
			},
		},
		"compositionRevisionRef": {
			Type:     "object",
			Required: []string{"name"},
			Properties: map[string]extv1.JSONSchemaProps{
				"name": {Type: "string"},
			},
		},
		"compositionUpdatePolicy": {
			Type: "string",
			Enum: []extv1.JSON{
				{Raw: []byte(`"Automatic"`)},
				{Raw: []byte(`"Manual"`)},
			},
			Default: &extv1.JSON{Raw: []byte(`"Automatic"`)},
		},
		"compositeDeletePolicy": {
			Type: "string",
			Enum: []extv1.JSON{
				{Raw: []byte(`"Background"`)},
				{Raw: []byte(`"Foreground"`)},
			},
			Default: &extv1.JSON{Raw: []byte(`"Background"`)}},
		"resourceRef": {
			Type:     "object",
			Required: []string{"apiVersion", "kind", "name"},
			Properties: map[string]extv1.JSONSchemaProps{
				"apiVersion": {Type: "string"},
				"kind":       {Type: "string"},
				"name":       {Type: "string"},
			},
		},
		"publishConnectionDetailsTo": {
			Type:     "object",
			Required: []string{"name"},
			Properties: map[string]extv1.JSONSchemaProps{
				"name": {Type: "string"},
				"configRef": {
					Type:    "object",
					Default: &extv1.JSON{Raw: []byte(`{"name": "default"}`)},
					Properties: map[string]extv1.JSONSchemaProps{
						"name": {
							Type: "string",
						},
					},
				},
				"metadata": {
					Type: "object",
					Properties: map[string]extv1.JSONSchemaProps{
						"labels": {
							Type: "object",
							AdditionalProperties: &extv1.JSONSchemaPropsOrBool{
								Allows: true,
								Schema: &extv1.JSONSchemaProps{Type: "string"},
							},
						},
						"annotations": {
							Type: "object",
							AdditionalProperties: &extv1.JSONSchemaPropsOrBool{
								Allows: true,
								Schema: &extv1.JSONSchemaProps{Type: "string"},
							},
						},
						"type": {
							Type: "string",
						},
					},
				},
			},
		},
		"writeConnectionSecretToRef": {
			Type:     "object",
			Required: []string{"name"},
			Properties: map[string]extv1.JSONSchemaProps{
				"name": {Type: "string"},
			},
		},
	}
}

// CompositeResourceStatusProps is a partial OpenAPIV3Schema for the status
// fields that Crossplane expects to be present for all defined or published
// infrastructure resources.
func CompositeResourceStatusProps() map[string]extv1.JSONSchemaProps {
	return map[string]extv1.JSONSchemaProps{
		"conditions": {
			Description: "Conditions of the resource.",
			Type:        "array",
			Items: &extv1.JSONSchemaPropsOrArray{
				Schema: &extv1.JSONSchemaProps{
					Type:     "object",
					Required: []string{"lastTransitionTime", "reason", "status", "type"},
					Properties: map[string]extv1.JSONSchemaProps{
						"lastTransitionTime": {Type: "string", Format: "date-time"},
						"message":            {Type: "string"},
						"reason":             {Type: "string"},
						"status":             {Type: "string"},
						"type":               {Type: "string"},
					},
				},
			},
		},
		"connectionDetails": {
			Type: "object",
			Properties: map[string]extv1.JSONSchemaProps{
				"lastPublishedTime": {Type: "string", Format: "date-time"},
			},
		},
	}
}

// CompositeResourcePrinterColumns returns the set of default printer columns
// that should exist in all generated composite resource CRDs.
func CompositeResourcePrinterColumns() []extv1.CustomResourceColumnDefinition {
	return []extv1.CustomResourceColumnDefinition{
		{
			Name:     "SYNCED",
			Type:     "string",
			JSONPath: ".status.conditions[?(@.type=='Synced')].status",
		},
		{
			Name:     "READY",
			Type:     "string",
			JSONPath: ".status.conditions[?(@.type=='Ready')].status",
		},
		{
			Name:     "COMPOSITION",
			Type:     "string",
			JSONPath: ".spec.compositionRef.name",
		},
		{
			Name:     "AGE",
			Type:     "date",
			JSONPath: ".metadata.creationTimestamp",
		},
	}
}

// CompositeResourceClaimPrinterColumns returns the set of default printer
// columns that should exist in all generated composite resource claim CRDs.
func CompositeResourceClaimPrinterColumns() []extv1.CustomResourceColumnDefinition {
	return []extv1.CustomResourceColumnDefinition{
		{
			Name:     "SYNCED",
			Type:     "string",
			JSONPath: ".status.conditions[?(@.type=='Synced')].status",
		},
		{
			Name:     "READY",
			Type:     "string",
			JSONPath: ".status.conditions[?(@.type=='Ready')].status",
		},
		{
			Name:     "CONNECTION-SECRET",
			Type:     "string",
			JSONPath: ".spec.writeConnectionSecretToRef.name",
		},
		{
			Name:     "AGE",
			Type:     "date",
			JSONPath: ".metadata.creationTimestamp",
		},
	}
}

// GetPropFields returns the fields from a map of schema properties
func GetPropFields(props map[string]extv1.JSONSchemaProps) []string {
	propFields := make([]string, len(props))
	i := 0
	for k := range props {
		propFields[i] = k
		i++
	}
	return propFields
}

func ForCompositeResource(xrd *v1.CompositeResourceDefinition) (*extv1.CustomResourceDefinition, error) {
	crd := &extv1.CustomResourceDefinition{
		Spec: extv1.CustomResourceDefinitionSpec{
			Scope:    extv1.ClusterScoped,
			Group:    xrd.Spec.Group,
			Names:    xrd.Spec.Names,
			Versions: make([]extv1.CustomResourceDefinitionVersion, len(xrd.Spec.Versions)),
		},
	}

	crd.SetName(xrd.GetName())
	crd.SetLabels(xrd.GetLabels())

	crd.Spec.Names.Categories = append(crd.Spec.Names.Categories, CategoryComposite)

	for i, vr := range xrd.Spec.Versions {
		crd.Spec.Versions[i] = extv1.CustomResourceDefinitionVersion{
			Name:                     vr.Name,
			Served:                   vr.Served,
			Storage:                  vr.Referenceable,
			Deprecated:               pointer.BoolDeref(vr.Deprecated, false),
			DeprecationWarning:       vr.DeprecationWarning,
			AdditionalPrinterColumns: append(vr.AdditionalPrinterColumns, CompositeResourcePrinterColumns()...),
			Schema: &extv1.CustomResourceValidation{
				OpenAPIV3Schema: BaseProps(),
			},
			Subresources: &extv1.CustomResourceSubresources{
				Status: &extv1.CustomResourceSubresourceStatus{},
			},
		}

		p, required, err := getProps("spec", vr.Schema)
		if err != nil {
			return nil, errors.Wrapf(err, errFmtGetProps, "spec")
		}
		specProps := crd.Spec.Versions[i].Schema.OpenAPIV3Schema.Properties["spec"]
		specProps.Required = append(specProps.Required, required...)
		for k, v := range p {
			specProps.Properties[k] = v
		}
		for k, v := range CompositeResourceSpecProps() {
			specProps.Properties[k] = v
		}
		crd.Spec.Versions[i].Schema.OpenAPIV3Schema.Properties["spec"] = specProps

		statusP, statusRequired, err := getProps("status", vr.Schema)
		if err != nil {
			return nil, errors.Wrapf(err, errFmtGetProps, "status")
		}
		statusProps := crd.Spec.Versions[i].Schema.OpenAPIV3Schema.Properties["status"]
		statusProps.Required = statusRequired
		for k, v := range statusP {
			statusProps.Properties[k] = v
		}
		for k, v := range CompositeResourceStatusProps() {
			statusProps.Properties[k] = v
		}
		crd.Spec.Versions[i].Schema.OpenAPIV3Schema.Properties["status"] = statusProps
	}

	return crd, nil
}

// ForCompositeResourceClaim derives the CustomResourceDefinition for a
// composite resource claim from the supplied CompositeResourceDefinition.
func ForCompositeResourceClaim(xrd *v1.CompositeResourceDefinition) (*extv1.CustomResourceDefinition, error) {
	if err := validateClaimNames(xrd); err != nil {
		return nil, errors.Wrap(err, errInvalidClaimNames)
	}

	crd := &extv1.CustomResourceDefinition{
		Spec: extv1.CustomResourceDefinitionSpec{
			Scope:    extv1.NamespaceScoped,
			Group:    xrd.Spec.Group,
			Names:    *xrd.Spec.ClaimNames,
			Versions: make([]extv1.CustomResourceDefinitionVersion, len(xrd.Spec.Versions)),
		},
	}

	crd.SetName(xrd.Spec.ClaimNames.Plural + "." + xrd.Spec.Group)
	crd.SetLabels(xrd.GetLabels())

	crd.Spec.Names.Categories = append(crd.Spec.Names.Categories, CategoryClaim)

	for i, vr := range xrd.Spec.Versions {
		crd.Spec.Versions[i] = extv1.CustomResourceDefinitionVersion{
			Name:                     vr.Name,
			Served:                   vr.Served,
			Storage:                  vr.Referenceable,
			Deprecated:               pointer.BoolDeref(vr.Deprecated, false),
			DeprecationWarning:       vr.DeprecationWarning,
			AdditionalPrinterColumns: append(vr.AdditionalPrinterColumns, CompositeResourceClaimPrinterColumns()...),
			Schema: &extv1.CustomResourceValidation{
				OpenAPIV3Schema: BaseProps(),
			},
			Subresources: &extv1.CustomResourceSubresources{
				Status: &extv1.CustomResourceSubresourceStatus{},
			},
		}

		p, required, err := getProps("spec", vr.Schema)
		if err != nil {
			return nil, errors.Wrapf(err, errFmtGetProps, "spec")
		}
		specProps := crd.Spec.Versions[i].Schema.OpenAPIV3Schema.Properties["spec"]
		specProps.Required = append(specProps.Required, required...)
		for k, v := range p {
			specProps.Properties[k] = v
		}
		for k, v := range CompositeResourceClaimSpecProps() {
			specProps.Properties[k] = v
		}
		crd.Spec.Versions[i].Schema.OpenAPIV3Schema.Properties["spec"] = specProps

		statusP, statusRequired, err := getProps("status", vr.Schema)
		if err != nil {
			return nil, errors.Wrapf(err, errFmtGetProps, "status")
		}
		statusProps := crd.Spec.Versions[i].Schema.OpenAPIV3Schema.Properties["status"]
		statusProps.Required = statusRequired
		for k, v := range statusP {
			statusProps.Properties[k] = v
		}
		for k, v := range CompositeResourceStatusProps() {
			statusProps.Properties[k] = v
		}
		crd.Spec.Versions[i].Schema.OpenAPIV3Schema.Properties["status"] = statusProps
	}

	return crd, nil
}

func validateClaimNames(d *v1.CompositeResourceDefinition) error {
	if d.Spec.ClaimNames == nil {
		return errors.New(errMissingClaimNames)
	}

	if n := d.Spec.ClaimNames.Kind; n == d.Spec.Names.Kind {
		return errors.Errorf(errFmtConflictingClaimName, n)
	}

	if n := d.Spec.ClaimNames.Plural; n == d.Spec.Names.Plural {
		return errors.Errorf(errFmtConflictingClaimName, n)
	}

	if n := d.Spec.ClaimNames.Singular; n != "" && n == d.Spec.Names.Singular {
		return errors.Errorf(errFmtConflictingClaimName, n)
	}

	if n := d.Spec.ClaimNames.ListKind; n != "" && n == d.Spec.Names.ListKind {
		return errors.Errorf(errFmtConflictingClaimName, n)
	}

	return nil
}

func getProps(field string, v *v1.CompositeResourceValidation) (map[string]extv1.JSONSchemaProps, []string, error) {
	if v == nil {
		return nil, nil, nil
	}

	s := &extv1.JSONSchemaProps{}
	if err := json.Unmarshal(v.OpenAPIV3Schema.Raw, s); err != nil {
		return nil, nil, errors.Wrap(err, errParseValidation)
	}

	spec, ok := s.Properties[field]
	if !ok {
		return nil, nil, nil
	}

	return spec.Properties, spec.Required, nil
}

func loadXrd(path string) (*v1.CompositeResourceDefinition, error) {
	y, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var xrd v1.CompositeResourceDefinition
	err = yaml.Unmarshal(y, &xrd)
	if err != nil {
		return nil, err
	}
	return &xrd, nil
}

func generateCrdForPaths(paths []string, oututFolder string) error {
	err := generateCrdForPathsOfType(paths, oututFolder, ForCompositeResource)
	if err != nil {
		return err
	}
	err = generateCrdForPathsOfType(paths, oututFolder, ForCompositeResourceClaim)
	if err != nil {
		return err
	}
	return nil
}

func generateCrdForPathsOfType(paths []string, oututFolder string, generator func(xrd *v1.CompositeResourceDefinition) (*extv1.CustomResourceDefinition, error)) error {
	for _, m := range paths {
		fmt.Println(m)

		xrd, _ := loadXrd(m)

		crd, err := generator(xrd)
		crd.Kind = "CustomResourceDefinition"
		crd.APIVersion = "apiextensions.k8s.io/v1"
		if err != nil {
			return err
		}
		y, err := yaml.Marshal(crd)
		if err != nil {
			return err
		}

		output := filepath.Join(oututFolder, "/crds/", fmt.Sprintf("%s_%s.yaml", crd.Spec.Group, crd.Spec.Names.Plural))

		err = ioutil.WriteFile(output, y, 0644)
		if err != nil {
			return err
		}
	}
	return nil
}

func findPathsForPattern(pattern string, cwd string) ([]string, error) {
	iGlob := filepath.Join(cwd, "*/", pattern)
	ml, err := filepath.Glob(iGlob)
	if err != nil {
		return nil, err
	}

	return ml, nil
}

func generateCrdsForPattern(pattern string, cwd string) error {
	ml, err := findPathsForPattern(pattern, cwd)
	if err != nil {
		return err
	}

	err = generateCrdForPaths(ml, cwd)

	return err
}

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Println(err)
	}
	definitionFile := "xrd.yaml"
	err = generateCrdsForPattern(definitionFile, cwd)

	if err != nil {
		fmt.Printf("Error finding generator %s", err)
	}
	definitionFile = "test.yaml"
	err = generateCrdsForPattern(definitionFile, cwd)

	if err != nil {
		fmt.Printf("Error finding generator %s", err)
	}

}
