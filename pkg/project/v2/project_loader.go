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

package v2

import (
	"fmt"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/internal/files"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/internal/log"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/internal/log/field"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/persistence/config/loader"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/config"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/config/coordinate"
	configErrors "github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/config/errors"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/config/parameter"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/manifest"
	"github.com/spf13/afero"
)

type ProjectLoaderContext struct {
	KnownApis       map[string]struct{}
	WorkingDir      string
	Manifest        manifest.Manifest
	ParametersSerde map[string]parameter.ParameterSerDe
}

type DuplicateConfigIdentifierError struct {
	Location           coordinate.Coordinate           `json:"location"`
	EnvironmentDetails configErrors.EnvironmentDetails `json:"environmentDetails"`
}

func (e DuplicateConfigIdentifierError) Coordinates() coordinate.Coordinate {
	return e.Location
}

func (e DuplicateConfigIdentifierError) LocationDetails() configErrors.EnvironmentDetails {
	return e.EnvironmentDetails
}

func (e DuplicateConfigIdentifierError) Error() string {
	return fmt.Sprintf("Config IDs need to be unique to project/type, found duplicate `%s`", e.Location)
}

func newDuplicateConfigIdentifierError(c config.Config) DuplicateConfigIdentifierError {
	return DuplicateConfigIdentifierError{
		Location: c.Coordinate,
		EnvironmentDetails: configErrors.EnvironmentDetails{
			Group:       c.Group,
			Environment: c.Environment,
		},
	}
}

func LoadProjects(fs afero.Fs, context ProjectLoaderContext) ([]Project, []error) {
	environments := toEnvironmentSlice(context.Manifest.Environments)
	projects := make([]Project, 0)

	var workingDirFs afero.Fs

	if context.WorkingDir == "." {
		workingDirFs = fs
	} else {
		workingDirFs = afero.NewBasePathFs(fs, context.WorkingDir)
	}

	log.Info("Loading %d projects...", len(context.Manifest.Projects))

	var errors []error

	for _, projectDefinition := range context.Manifest.Projects {
		project, projectErrors := loadProject(workingDirFs, context, projectDefinition, environments)

		if projectErrors != nil {
			errors = append(errors, projectErrors...)
			continue
		}

		projects = append(projects, project)
	}

	if errors != nil {
		return nil, errors
	}

	return projects, nil
}

func toEnvironmentSlice(environments map[string]manifest.EnvironmentDefinition) []manifest.EnvironmentDefinition {
	var result []manifest.EnvironmentDefinition

	for _, env := range environments {
		result = append(result, env)
	}

	return result
}

func loadProject(fs afero.Fs, context ProjectLoaderContext, projectDefinition manifest.ProjectDefinition,
	environments []manifest.EnvironmentDefinition) (Project, []error) {

	exists, err := afero.Exists(fs, projectDefinition.Path)
	if err != nil {
		return Project{}, []error{fmt.Errorf("failed to load project `%s` (%s): %w", projectDefinition.Name, projectDefinition.Path, err)}
	}
	if !exists {
		return Project{}, []error{fmt.Errorf("failed to load project `%s`: filepath `%s` does not exist", projectDefinition.Name, projectDefinition.Path)}
	}

	log.Debug("Loading project `%s` (%s)...", projectDefinition.Name, projectDefinition.Path)

	configs, errors := loadConfigsOfProject(fs, context, projectDefinition, environments)

	if d := findDuplicatedConfigIdentifiers(configs); d != nil {
		for _, c := range d {
			errors = append(errors, newDuplicateConfigIdentifierError(c))
		}
	}

	if errors != nil {
		return Project{}, errors
	}

	configMap := make(ConfigsPerTypePerEnvironments)

	for _, conf := range configs {
		if _, found := configMap[conf.Environment]; !found {
			configMap[conf.Environment] = make(map[string][]config.Config)
		}

		configMap[conf.Environment][conf.Coordinate.Type] = append(configMap[conf.Environment][conf.Coordinate.Type], conf)
	}

	return Project{
		Id:           projectDefinition.Name,
		GroupId:      projectDefinition.Group,
		Configs:      configMap,
		Dependencies: toDependenciesMap(projectDefinition.Name, configs),
	}, nil
}

func loadConfigsOfProject(fs afero.Fs, loadingContext ProjectLoaderContext, projectDefinition manifest.ProjectDefinition,
	environments []manifest.EnvironmentDefinition) ([]config.Config, []error) {

	configFiles, err := findConfigFiles(fs, projectDefinition.Path)
	if err != nil {
		return nil, []error{fmt.Errorf("failed to walk files: %w", err)}
	}

	var configs []config.Config
	var errs []error

	ctx := &loader.LoaderContext{
		ProjectId:       projectDefinition.Name,
		Environments:    environments,
		Path:            projectDefinition.Path,
		KnownApis:       loadingContext.KnownApis,
		ParametersSerDe: loadingContext.ParametersSerde,
	}

	for _, file := range configFiles {
		log.WithFields(field.F("file", file)).Debug("Loading configuration file %s", file)
		loadedConfigs, configErrs := loader.LoadConfig(fs, ctx, file)

		errs = append(errs, configErrs...)
		configs = append(configs, loadedConfigs...)
	}

	return configs, errs
}

// findConfigFiles finds all YAML files within the given root directory.
// Hidden directories (start with a dot (.)) are excluded.
// Directories marked as hidden on Windows are not excluded.
func findConfigFiles(fs afero.Fs, root string) ([]string, error) {
	var configFiles []string

	err := afero.Walk(fs, root, func(curPath string, info os.FileInfo, err error) error {
		name := info.Name()

		if info.IsDir() {
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
		}

		if files.IsYamlFileExtension(name) {
			configFiles = append(configFiles, path.Join(curPath))
		}
		return nil
	})

	return configFiles, err
}

func findDuplicatedConfigIdentifiers(configs []config.Config) []config.Config {

	coordinates := make(map[string]struct{})
	var duplicates []config.Config
	for _, c := range configs {
		id := toFullyQualifiedConfigIdentifier(c)
		if _, found := coordinates[id]; found {
			duplicates = append(duplicates, c)
		}
		coordinates[id] = struct{}{}
	}
	return duplicates
}

// toFullyUniqueConfigIdentifier returns a configs coordinate as well as environment,
// as in the scope of project loader we might have "overlapping" coordinates for any loaded
// environment or group override of the same configuration
func toFullyQualifiedConfigIdentifier(config config.Config) string {
	return fmt.Sprintf("%s:%s:%s", config.Group, config.Environment, config.Coordinate)
}

func toDependenciesMap(projectId string, configs []config.Config) DependenciesPerEnvironment {
	result := make(DependenciesPerEnvironment)

	for _, c := range configs {
		// ignore skipped configs
		if c.Skip {
			continue
		}

		for _, ref := range c.References() {
			// ignore project on same project
			if projectId == ref.Project {
				continue
			}

			if !containsProject(result[c.Environment], ref.Project) {
				result[c.Environment] = append(result[c.Environment], ref.Project)
			}
		}
	}

	return result
}

func containsProject(projects []string, project string) bool {
	for _, p := range projects {
		if p == project {
			return true
		}
	}

	return false
}
