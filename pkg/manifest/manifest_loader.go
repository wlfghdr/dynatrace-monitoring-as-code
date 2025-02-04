// @license
// Copyright 2021 Dynatrace LLC
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package manifest

import (
	"errors"
	"fmt"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/internal/files"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/internal/log"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/internal/log/field"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/internal/slices"
	version2 "github.com/dynatrace/dynatrace-configuration-as-code/v2/internal/version"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/version"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v2"
	"os"
	"path/filepath"
	"strings"
)

// LoaderContext holds all information for [LoadManifest]
type LoaderContext struct {
	// Fs holds the abstraction of the file system.
	Fs afero.Fs

	// ManifestPath holds the path from where the manifest should be loaded.
	ManifestPath string

	// Environments is a filter to what environments should be loaded.
	// If it's empty, all environments are loaded.
	// If both Environments and Groups are specified, the union of both results is returned.
	//
	// If Environments contains items that do not match any environment in the specified manifest file, the loading errors.
	Environments []string

	// Groups is a filter to what environment-groups (and thus environments) should be loaded.
	// If it's empty, all environment-groups are loaded.
	// If both Environments and Groups are specified, the union of both results is returned.
	//
	// If Groups contains items that do not match any environment in the specified manifest file, the loading errors.
	Groups []string

	// Opts are LoaderOptions holding optional configuration for LoadManifest
	Opts LoaderOptions
}

type projectLoaderContext struct {
	fs           afero.Fs
	manifestPath string
}

// LoaderOptions are optional configuration for LoadManifest
type LoaderOptions struct {
	DontResolveEnvVars bool
}

type ManifestLoaderError struct {
	ManifestPath string `json:"manifestPath"`
	Reason       string `json:"reason"`
}

func (e ManifestLoaderError) Error() string {
	return fmt.Sprintf("%s: %s", e.ManifestPath, e.Reason)
}

func newManifestLoaderError(path string, reason string) ManifestLoaderError {
	return ManifestLoaderError{
		ManifestPath: path,
		Reason:       reason,
	}
}

type EnvironmentDetails struct {
	Group       string `json:"group"`
	Environment string `json:"environment"`
}

type EnvironmentLoaderError struct {
	ManifestLoaderError
	EnvironmentDetails EnvironmentDetails `json:"environmentDetails"`
}

func newManifestEnvironmentLoaderError(manifest string, group string, env string, reason string) EnvironmentLoaderError {
	return EnvironmentLoaderError{
		ManifestLoaderError: newManifestLoaderError(manifest, reason),
		EnvironmentDetails: EnvironmentDetails{
			Group:       group,
			Environment: env,
		},
	}
}

func (e EnvironmentLoaderError) Error() string {
	return fmt.Sprintf("%s:%s:%s: %s", e.ManifestPath, e.EnvironmentDetails.Group, e.EnvironmentDetails.Environment, e.Reason)
}

type ProjectLoaderError struct {
	ManifestLoaderError
	Project string `json:"project"`
}

func newManifestProjectLoaderError(manifest string, project string, reason string) ProjectLoaderError {
	return ProjectLoaderError{
		ManifestLoaderError: newManifestLoaderError(manifest, reason),
		Project:             project,
	}
}

func (e ProjectLoaderError) Error() string {
	return fmt.Sprintf("%s:%s: %s", e.ManifestPath, e.Project, e.Reason)
}

func LoadManifest(context *LoaderContext) (Manifest, []error) {
	log.WithFields(field.F("manifestPath", context.ManifestPath)).Info("Loading manifest %q. Restrictions: groups=%q, environments=%q", context.ManifestPath, context.Groups, context.Environments)

	manifestYAML, err := readManifestYAML(context)
	if err != nil {
		return Manifest{}, []error{err}
	}
	if errs := verifyManifestYAML(manifestYAML); errs != nil {
		var retErrs []error
		for _, e := range errs {
			retErrs = append(retErrs, newManifestLoaderError(context.ManifestPath, fmt.Sprintf("invalid manifest definition: %s", e)))
		}
		return Manifest{}, retErrs
	}

	manifestPath := filepath.Clean(context.ManifestPath)

	workingDir := filepath.Dir(manifestPath)
	var workingDirFs afero.Fs

	if workingDir == "." {
		workingDirFs = context.Fs
	} else {
		workingDirFs = afero.NewBasePathFs(context.Fs, workingDir)
	}

	relativeManifestPath := filepath.Base(manifestPath)

	projectDefinitions, projectErrors := toProjectDefinitions(&projectLoaderContext{
		fs:           workingDirFs,
		manifestPath: relativeManifestPath,
	}, manifestYAML.Projects)

	var errs []error
	if projectErrors != nil {
		errs = append(errs, projectErrors...)
	} else if len(projectDefinitions) == 0 {
		errs = append(errs, newManifestLoaderError(context.ManifestPath, "no projects defined in manifest"))
	}

	environmentDefinitions, manifestErrors := toEnvironments(context, manifestYAML.EnvironmentGroups)

	if manifestErrors != nil {
		errs = append(errs, manifestErrors...)
	} else if len(environmentDefinitions) == 0 {
		errs = append(errs, newManifestLoaderError(context.ManifestPath, "no environments defined in manifest"))
	}

	if errs != nil {
		return Manifest{}, errs
	}

	return Manifest{
		Projects:     projectDefinitions,
		Environments: environmentDefinitions,
	}, nil
}

func parseAuth(context *LoaderContext, a auth) (Auth, error) {
	token, err := parseAuthSecret(context, a.Token)
	if err != nil {
		return Auth{}, fmt.Errorf("error parsing token: %w", err)
	}

	if a.OAuth == nil {
		return Auth{
			Token: token,
		}, nil
	}

	o, err := parseOAuth(context, *a.OAuth)
	if err != nil {
		return Auth{}, fmt.Errorf("failed to parse OAuth credentials: %w", err)
	}

	return Auth{
		Token: token,
		OAuth: &o,
	}, nil

}

func parseAuthSecret(context *LoaderContext, s authSecret) (AuthSecret, error) {

	if !(s.Type == typeEnvironment || s.Type == "") {
		return AuthSecret{}, errors.New("type must be 'environment'")
	}

	if s.Name == "" {
		return AuthSecret{}, errors.New("no name given or empty")
	}

	if context.Opts.DontResolveEnvVars {
		log.Debug("Skipped resolving environment variable %s based on loader options", s.Name)
		return AuthSecret{
			Name:  s.Name,
			Value: fmt.Sprintf("SKIPPED RESOLUTION OF ENV_VAR: %s", s.Name),
		}, nil
	}

	v, f := os.LookupEnv(s.Name)
	if !f {
		return AuthSecret{}, fmt.Errorf("environment-variable %q was not found", s.Name)
	}

	if v == "" {
		return AuthSecret{}, fmt.Errorf("environment-variable %q found, but the value resolved is empty", s.Name)
	}

	return AuthSecret{Name: s.Name, Value: v}, nil
}

func parseOAuth(context *LoaderContext, a oAuth) (OAuth, error) {
	clientID, err := parseAuthSecret(context, a.ClientID)
	if err != nil {
		return OAuth{}, fmt.Errorf("failed to parse ClientID: %w", err)
	}

	clientSecret, err := parseAuthSecret(context, a.ClientSecret)
	if err != nil {
		return OAuth{}, fmt.Errorf("failed to parse ClientSecret: %w", err)
	}

	if a.TokenEndpoint != nil {
		urlDef, err := parseURLDefinition(context, *a.TokenEndpoint)
		if err != nil {
			return OAuth{}, fmt.Errorf(`failed to parse "tokenEndpoint": %w`, err)
		}

		return OAuth{
			ClientID:      clientID,
			ClientSecret:  clientSecret,
			TokenEndpoint: &urlDef,
		}, nil
	}

	return OAuth{
		ClientID:      clientID,
		ClientSecret:  clientSecret,
		TokenEndpoint: nil,
	}, nil
}

func readManifestYAML(context *LoaderContext) (manifest, error) {
	manifestPath := filepath.Clean(context.ManifestPath)

	if !files.IsYamlFileExtension(manifestPath) {
		return manifest{}, newManifestLoaderError(context.ManifestPath, "manifest file is not a yaml")
	}

	if exists, err := files.DoesFileExist(context.Fs, manifestPath); err != nil {
		return manifest{}, err
	} else if !exists {
		return manifest{}, newManifestLoaderError(context.ManifestPath, "manifest file does not exist")
	}

	rawData, err := afero.ReadFile(context.Fs, manifestPath)
	if err != nil {
		return manifest{}, newManifestLoaderError(context.ManifestPath, fmt.Sprintf("error while reading the manifest: %s", err))
	}

	var m manifest

	err = yaml.UnmarshalStrict(rawData, &m)
	if err != nil {
		return manifest{}, newManifestLoaderError(context.ManifestPath, fmt.Sprintf("error during parsing the manifest: %s", err))
	}
	return m, nil
}

func verifyManifestYAML(m manifest) []error {
	var errs []error

	if err := validateManifestVersion(m.ManifestVersion); err != nil {
		errs = append(errs, err)
	}

	if len(m.Projects) == 0 { //this should be checked over the Manifest
		errs = append(errs, fmt.Errorf("no `projects` defined"))
	}

	if len(m.EnvironmentGroups) == 0 { //this should be checked over the Manifest
		errs = append(errs, fmt.Errorf("no `environmentGroups` defined"))
	}

	return errs
}

var maxSupportedManifestVersion, _ = version2.ParseVersion(version.ManifestVersion)
var minSupportedManifestVersion, _ = version2.ParseVersion(version.MinManifestVersion)

func validateManifestVersion(manifestVersion string) error {
	if len(manifestVersion) == 0 {
		return fmt.Errorf("`manifestVersion` missing")
	}

	v, err := version2.ParseVersion(manifestVersion)
	if err != nil {
		return fmt.Errorf("invalid `manifestVersion`: %w", err)
	}

	if v.SmallerThan(minSupportedManifestVersion) {
		return fmt.Errorf("`manifestVersion` %s is no longer supported. Min required version is %s, please update manifest", manifestVersion, version.MinManifestVersion)
	}

	if v.GreaterThan(maxSupportedManifestVersion) {
		return fmt.Errorf("`manifestVersion` %s is not supported by monaco %s. Max supported version is %s, please check manifest or update monaco", manifestVersion, version.MonitoringAsCode, version.ManifestVersion)
	}

	return nil
}

func toEnvironments(context *LoaderContext, groups []group) (map[string]EnvironmentDefinition, []error) { // nolint:gocognit
	var errors []error
	environments := make(map[string]EnvironmentDefinition)

	groupNames := make(map[string]bool, len(groups))
	envNames := make(map[string]bool, len(groups))

	for i, group := range groups {
		if group.Name == "" {
			errors = append(errors, newManifestLoaderError(context.ManifestPath, fmt.Sprintf("missing group name on index `%d`", i)))
		}

		if groupNames[group.Name] {
			errors = append(errors, newManifestLoaderError(context.ManifestPath, fmt.Sprintf("duplicated group name %q", group.Name)))
		}

		groupNames[group.Name] = true

		for j, env := range group.Environments {

			if env.Name == "" {
				errors = append(errors, newManifestLoaderError(context.ManifestPath, fmt.Sprintf("missing environment name in group %q on index `%d`", group.Name, j)))
				continue
			}

			if envNames[env.Name] {
				errors = append(errors, newManifestLoaderError(context.ManifestPath, fmt.Sprintf("duplicated environment name %q", env.Name)))
				continue
			}
			envNames[env.Name] = true

			// skip loading if environments is not empty, the environments does not contain the env name, or the group should not be included
			if shouldSkipEnv(context, group, env) {
				log.WithFields(field.F("manifestPath", context.ManifestPath)).Debug("skipping loading of environment %q", env.Name)
				continue
			}

			parsedEnv, configErrors := parseEnvironment(context, env, group.Name)

			if configErrors != nil {
				errors = append(errors, configErrors...)
				continue
			}

			environments[parsedEnv.Name] = parsedEnv
		}
	}

	// validate that all required groups & environments are included
	for _, g := range context.Groups {
		if !groupNames[g] {
			errors = append(errors, newManifestLoaderError(context.ManifestPath, fmt.Sprintf("requested group %q not found", g)))
		}
	}

	for _, e := range context.Environments {
		if !envNames[e] {
			errors = append(errors, newManifestLoaderError(context.ManifestPath, fmt.Sprintf("requested environment %q not found", e)))
		}
	}

	if errors != nil {
		return nil, errors
	}

	return environments, nil
}

func shouldSkipEnv(context *LoaderContext, group group, env environment) bool {
	// if nothing is restricted, everything is allowed
	if len(context.Groups) == 0 && len(context.Environments) == 0 {
		return false
	}

	if slices.Contains(context.Groups, group.Name) {
		return false
	}

	if slices.Contains(context.Environments, env.Name) {
		return false
	}

	return true
}

func parseEnvironment(context *LoaderContext, config environment, group string) (EnvironmentDefinition, []error) {
	var errs []error

	a, err := parseAuth(context, config.Auth)
	if err != nil {
		errs = append(errs, newManifestEnvironmentLoaderError(context.ManifestPath, group, config.Name, fmt.Sprintf("failed to parse auth section: %s", err)))
	}

	urlDef, err := parseURLDefinition(context, config.URL)
	if err != nil {
		errs = append(errs, newManifestEnvironmentLoaderError(context.ManifestPath, group, config.Name, err.Error()))
	}

	if len(errs) > 0 {
		return EnvironmentDefinition{}, errs
	}

	return EnvironmentDefinition{
		Name:  config.Name,
		URL:   urlDef,
		Auth:  a,
		Group: group,
	}, nil
}

func parseURLDefinition(context *LoaderContext, u url) (URLDefinition, error) {

	// Depending on the type, the url.value either contains the env var name or the direct value of the url
	if u.Value == "" {
		return URLDefinition{}, errors.New("no `Url` configured or value is blank")
	}

	if u.Type == "" || u.Type == urlTypeValue {
		val := strings.TrimSuffix(u.Value, "/")

		return URLDefinition{
			Type:  ValueURLType,
			Value: val,
		}, nil
	}

	if u.Type == urlTypeEnvironment {

		if context.Opts.DontResolveEnvVars {
			log.Debug("Skipped resolving environment variable %s based on loader options", u.Value)
			return URLDefinition{
				Type:  EnvironmentURLType,
				Value: fmt.Sprintf("SKIPPED RESOLUTION OF ENV_VAR: %s", u.Value),
				Name:  u.Value,
			}, nil
		}

		val, found := os.LookupEnv(u.Value)
		if !found {
			return URLDefinition{}, fmt.Errorf("environment variable %q could not be found", u.Value)
		}

		if val == "" {
			return URLDefinition{}, fmt.Errorf("environment variable %q is defined but has no value", u.Value)
		}

		val = strings.TrimSuffix(val, "/")

		return URLDefinition{
			Type:  EnvironmentURLType,
			Value: val,
			Name:  u.Value,
		}, nil

	}

	return URLDefinition{}, fmt.Errorf("%q is not a valid URL type", u.Type)
}

func toProjectDefinitions(context *projectLoaderContext, definitions []project) (map[string]ProjectDefinition, []error) {
	var errors []error
	result := make(map[string]ProjectDefinition)

	definitionErrors := checkForDuplicateDefinitions(context, definitions)
	if len(definitionErrors) > 0 {
		return nil, definitionErrors
	}

	for _, project := range definitions {
		parsed, projectErrors := parseProjectDefinition(context, project)

		if projectErrors != nil {
			errors = append(errors, projectErrors...)
			continue
		}

		for _, project := range parsed {
			if p, found := result[project.Name]; found {
				errors = append(errors, newManifestLoaderError(context.manifestPath, fmt.Sprintf("duplicated project name `%s` used by %s and %s", project.Name, p, project)))
				continue
			}

			result[project.Name] = project
		}
	}

	if errors != nil {
		return nil, errors
	}

	return result, nil
}

func checkForDuplicateDefinitions(context *projectLoaderContext, definitions []project) (errors []error) {
	definedIds := map[string]struct{}{}
	for _, project := range definitions {
		if _, found := definedIds[project.Name]; found {
			errors = append(errors, newManifestLoaderError(context.manifestPath, fmt.Sprintf("duplicated project name `%s`", project.Name)))
		}
		definedIds[project.Name] = struct{}{}
	}
	return errors
}

func parseProjectDefinition(context *projectLoaderContext, project project) ([]ProjectDefinition, []error) {
	var projectType string

	if project.Type == "" {
		projectType = simpleProjectType
	} else {
		projectType = project.Type
	}

	if project.Name == "" {
		return nil, []error{newManifestProjectLoaderError(context.manifestPath, project.Name, "project name is required")}
	}

	switch projectType {
	case simpleProjectType:
		return parseSimpleProjectDefinition(context, project)
	case groupProjectType:
		return parseGroupingProjectDefinition(context, project)
	default:
		return nil, []error{newManifestProjectLoaderError(context.manifestPath, project.Name,
			fmt.Sprintf("invalid project type `%s`", projectType))}
	}
}

func parseSimpleProjectDefinition(context *projectLoaderContext, project project) ([]ProjectDefinition, []error) {
	if project.Path == "" && project.Name == "" {
		return nil, []error{newManifestProjectLoaderError(context.manifestPath, project.Name,
			"project is missing both name and path")}
	}

	if strings.ContainsAny(project.Name, `/\`) {
		return nil, []error{newManifestProjectLoaderError(context.manifestPath, project.Name,
			`project name is not allowed to contain '/' or '\'`)}
	}

	if project.Path == "" {
		return []ProjectDefinition{
			{
				Name: project.Name,
				Path: project.Name,
			},
		}, nil
	}

	return []ProjectDefinition{
		{
			Name: project.Name,
			Path: project.Path,
		},
	}, nil
}

func parseGroupingProjectDefinition(context *projectLoaderContext, project project) ([]ProjectDefinition, []error) {
	projectPath := filepath.FromSlash(project.Path)

	files, err := afero.ReadDir(context.fs, projectPath)

	if err != nil {
		return nil, []error{newManifestProjectLoaderError(context.manifestPath, project.Name, fmt.Sprintf("failed to read project dir: %v", err))}
	}

	var result []ProjectDefinition

	for _, file := range files {
		if !file.IsDir() {
			continue
		}

		result = append(result, ProjectDefinition{
			Name:  project.Name + "." + file.Name(),
			Group: project.Name,
			Path:  filepath.Join(projectPath, file.Name()),
		})
	}

	if result == nil {
		// TODO should we really fail here?
		return nil, []error{newManifestProjectLoaderError(context.manifestPath, project.Name,
			fmt.Sprintf("no projects found in `%s`", projectPath))}
	}

	return result, nil
}
