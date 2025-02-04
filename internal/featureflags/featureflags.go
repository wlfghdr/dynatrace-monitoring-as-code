/*
 * @license
 * Copyright 2023 Dynatrace LLC
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package featureflags

import (
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/internal/log"
	"os"
	"strconv"
	"strings"
)

// FeatureFlag represents a command line switch to turn certain features
// ON or OFF. Values are read from environment variables defined by
// the feature flag. The feature flag can have default values that are used
// when the resp. environment variable does not exist
type FeatureFlag struct {
	// envName is the environment variable name
	// that is used to read the value from
	envName string
	// defaultEnabled states whether this feature flag
	// is enabled or disabled by default
	defaultEnabled bool
}

// New creates a new FeatureFlag
// envName is the environment variable the feature flag is loading the values from when evaluated
// defaultEnabled defines whether the feature flag is enabled or not by default
func New(envName string, defaultEnabled bool) FeatureFlag {
	return FeatureFlag{
		envName:        envName,
		defaultEnabled: defaultEnabled,
	}
}

// Enabled evaluates the feature flag.
// Feature flags are considered to be "enabled" if their resp. environment variable
// is set to 1, t, T, TRUE, true or True.
// Feature flags are considered to be "disabled" if their resp. environment variable
// is set to 0, f, F, FALSE, false or False.
func (ff FeatureFlag) Enabled() bool {
	if val, ok := os.LookupEnv(ff.envName); ok {
		enabled, err := strconv.ParseBool(strings.ToLower(val))
		if err != nil {
			log.Warn("Unsupported value %q for feature flag %q. Using default value: %v", val, ff.envName, ff.defaultEnabled)
			return ff.defaultEnabled
		}
		return enabled
	}
	return ff.defaultEnabled
}

// EnvName gives back the environment variable name for
// the feature flag
func (ff FeatureFlag) EnvName() string {
	return ff.envName
}

// Entities returns the feature flag that tells whether Dynatrace Entities download/matching is enabled or not
func Entities() FeatureFlag {
	return FeatureFlag{
		envName:        "MONACO_FEAT_ENTITIES",
		defaultEnabled: false,
	}
}

// DangerousCommands returns the feature flag that tells whether dangerous commands for the CLI are enabled or not
func DangerousCommands() FeatureFlag {
	return FeatureFlag{
		envName:        "MONACO_ENABLE_DANGEROUS_COMMANDS",
		defaultEnabled: false,
	}
}

// VerifyEnvironmentType returns the feature flag that tells whether the environment check
// at the beginning of execution is enabled or not
func VerifyEnvironmentType() FeatureFlag {
	return FeatureFlag{
		envName:        "MONACO_FEAT_VERIFY_ENV_TYPE",
		defaultEnabled: true,
	}
}

// ManagementZoneSettingsNumericIDs returns the feature flag that tells whether configs of settings type builtin:management-zones
// are addressed directly via their object ID or their resolved numeric ID when they are referenced.
func ManagementZoneSettingsNumericIDs() FeatureFlag {
	return FeatureFlag{
		envName:        "MONACO_FEAT_USE_MZ_NUMERIC_ID",
		defaultEnabled: true,
	}
}

// FastDependencyResolver returns the feature flag controlling whether the fast (but memory intensive) Aho-Corasick
// algorithm based dependency resolver is used when downloading. If set to false, the old naive and CPU intensive resolver
// is used.
func FastDependencyResolver() FeatureFlag {
	return FeatureFlag{
		envName:        "MONACO_FEAT_FAST_DEPENDENCY_RESOLVER",
		defaultEnabled: false,
	}
}

// DownloadFilter returns the feature flag controlling whether download filters out configurations that we believe can't
// be managed by config-as-code. Some users may still want to download everything on an environment, and turning off the
// filters allows them to do so.
func DownloadFilter() FeatureFlag {
	return FeatureFlag{
		envName:        "MONACO_FEAT_DOWNLOAD_FILTER",
		defaultEnabled: true,
	}
}

// DownloadFilterSettings returns the feature flag controlling whether general filters are applied to Settings download.
func DownloadFilterSettings() FeatureFlag {
	return FeatureFlag{
		envName:        "MONACO_FEAT_DOWNLOAD_FILTER_SETTINGS",
		defaultEnabled: true,
	}
}

// DownloadFilterSettingsUnmodifiable returns the feature flag controlling whether Settings marked as unmodifiable by
// their dtclient.SettingsModificationInfo are filtered out on download.
func DownloadFilterSettingsUnmodifiable() FeatureFlag {
	return FeatureFlag{
		envName:        "MONACO_FEAT_DOWNLOAD_FILTER_SETTINGS_UNMODIFIABLE",
		defaultEnabled: true,
	}
}

// DownloadFilterClassicConfigs returns the feature flag controlling whether download filters are applied to Classic Config API download.
func DownloadFilterClassicConfigs() FeatureFlag {
	return FeatureFlag{
		envName:        "MONACO_FEAT_DOWNLOAD_FILTER_CLASSIC_CONFIGS",
		defaultEnabled: true,
	}
}

// ConsistentUUIDGeneration returns the feature flag controlling whether generated UUIDs use consistent separator characters regardless of OS
// This is default true and just exists to get old, technically buggy behavior on Windows again if needed.
func ConsistentUUIDGeneration() FeatureFlag {
	return FeatureFlag{
		envName:        "MONACO_FEAT_CONSISTENT_UUID_GENERATION",
		defaultEnabled: true,
	}
}

// DependencyGraphBasedSort toggles whether sort.GetSortedConfigsForEnvironments use sgraph datastructures and algorithms for sorting projects.
func DependencyGraphBasedSort() FeatureFlag {
	return FeatureFlag{
		envName:        "MONACO_FEAT_GRAPH_SORT",
		defaultEnabled: true,
	}
}

// DependencyGraphBasedDeploy toggles whether we use graphs for deployment.
func DependencyGraphBasedDeploy() FeatureFlag {
	return FeatureFlag{
		envName:        "MONACO_FEAT_GRAPH_DEPLOY",
		defaultEnabled: false,
	}
}

// DependencyGraphBasedDeployParallel toggles whether we use parallel graph based deployment
func DependencyGraphBasedDeployParallel() FeatureFlag {
	return FeatureFlag{
		envName:        "MONACO_FEAT_GRAPH_DEPLOY_PARALLEL",
		defaultEnabled: false,
	}
}

func Buckets() FeatureFlag {
	return FeatureFlag{
		envName:        "MONACO_FEAT_BUCKETS",
		defaultEnabled: false,
	}
}
