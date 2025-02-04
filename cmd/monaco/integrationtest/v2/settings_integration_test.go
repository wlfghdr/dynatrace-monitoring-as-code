//go:build integration

/**
 * @license
 * Copyright 2020 Dynatrace LLC
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

package v2

import (
	"context"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/cmd/monaco/integrationtest"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/internal/idutils"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/internal/testutils"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/client/auth"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/client/dtclient"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/client/metadata"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/config"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/config/coordinate"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/manifest"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/project/v2/sort"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/rest"
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/dynatrace/dynatrace-configuration-as-code/v2/cmd/monaco/runner"
	"github.com/spf13/afero"
)

// tests all configs for a single environment
func TestIntegrationSettings(t *testing.T) {

	configFolder := "test-resources/integration-settings/"
	manifest := configFolder + "manifest.yaml"
	specificEnvironment := ""

	RunIntegrationWithCleanup(t, configFolder, manifest, specificEnvironment, "SettingsTwo", func(fs afero.Fs, _ TestContext) {

		// This causes Creation of all Settings
		cmd := runner.BuildCli(fs)
		cmd.SetArgs([]string{"deploy", "--verbose", manifest})
		err := cmd.Execute()

		assert.NoError(t, err)
		integrationtest.AssertAllConfigsAvailability(t, fs, manifest, []string{}, specificEnvironment, true)

		// This causes an Update of all Settings
		cmd = runner.BuildCli(fs)
		cmd.SetArgs([]string{"deploy", "--verbose", manifest})
		err = cmd.Execute()

		assert.NoError(t, err)
		integrationtest.AssertAllConfigsAvailability(t, fs, manifest, []string{}, specificEnvironment, true)
	})
}

// Tests a dry run (validation)
func TestIntegrationValidationSettings(t *testing.T) {

	t.Setenv("UNIQUE_TEST_SUFFIX", "can-be-nonunique-for-validation")

	configFolder := "test-resources/integration-settings/"
	manifest := configFolder + "manifest.yaml"

	cmd := runner.BuildCli(testutils.CreateTestFileSystem())
	cmd.SetArgs([]string{"deploy", "--verbose", "--dry-run", manifest})
	err := cmd.Execute()

	assert.NoError(t, err)
}

// TestOldExternalIDGetsUpdated tests whether a settings object with an "old" external ID that was
// generated using only "schemaID" and "configID" gets recognized and updated to have the "new" external ID
// that is composed of "projectName", "schemaID" and "configID"
func TestOldExternalIDGetsUpdated(t *testing.T) {

	fs := testutils.CreateTestFileSystem()
	var manifestPath = "test-resources/integration-settings-old-new-external-id/manifest.yaml"
	loadedManifest := integrationtest.LoadManifest(t, fs, manifestPath, "")
	projects := integrationtest.LoadProjects(t, fs, manifestPath, loadedManifest)
	sortedConfigs, _ := sort.ConfigsPerEnvironment(projects, []string{"platform_env"})
	environment := loadedManifest.Environments["platform_env"]
	configToDeploy := sortedConfigs["platform_env"][0]

	t.Cleanup(func() {
		integrationtest.CleanupIntegrationTest(t, fs, manifestPath, loadedManifest, "")
	})

	// first deploy with external id generate that does not consider the project name
	c := createSettingsClient(t, environment, dtclient.WithExternalIDGenerator(func(input coordinate.Coordinate) (string, error) {
		input.Project = ""
		id, _ := idutils.GenerateExternalID(input)
		return id, nil
	}))
	_, err := c.UpsertSettings(context.TODO(), dtclient.SettingsObject{
		Coordinate:     configToDeploy.Coordinate,
		SchemaId:       configToDeploy.Type.(config.SettingsType).SchemaId,
		SchemaVersion:  configToDeploy.Type.(config.SettingsType).SchemaVersion,
		Scope:          "environment",
		Content:        []byte(configToDeploy.Template.Content()),
		OriginObjectId: configToDeploy.OriginObjectId,
	})
	assert.NoError(t, err)

	cmd := runner.BuildCli(fs)
	cmd.SetArgs([]string{"deploy", "--verbose", manifestPath})
	err = cmd.Execute()

	assert.NoError(t, err)
	extID, _ := idutils.GenerateExternalID(sortedConfigs["platform_env"][0].Coordinate)

	// Check if settings 2.0 object with "new" external ID exists
	c = createSettingsClient(t, environment)
	settings, _ := c.ListSettings(context.TODO(), "builtin:anomaly-detection.metric-events", dtclient.ListSettingsOptions{DiscardValue: true, Filter: func(object dtclient.DownloadSettingsObject) bool {
		return object.ExternalId == extID
	}})
	assert.Len(t, settings, 1)

	// Check if no settings 2.0 object with "legacy" external ID exists
	coord := sortedConfigs["platform_env"][0].Coordinate
	coord.Project = ""
	legacyExtID, _ := idutils.GenerateExternalID(coord)
	settings, _ = c.ListSettings(context.TODO(), "builtin:anomaly-detection.metric-events", dtclient.ListSettingsOptions{DiscardValue: true, Filter: func(object dtclient.DownloadSettingsObject) bool {
		return object.ExternalId == legacyExtID
	}})
	assert.Len(t, settings, 0)

}

func TestDeploySettingsWithUniqueProperties(t *testing.T) {
	fs := testutils.CreateTestFileSystem()
	var manifestPath = "test-resources/settings-unique-properties/manifest.yaml"
	loadedManifest := integrationtest.LoadManifest(t, fs, manifestPath, "platform_env")

	t.Cleanup(func() {
		integrationtest.CleanupIntegrationTest(t, fs, manifestPath, loadedManifest, "")
	})

	cmd := runner.BuildCli(fs)
	cmd.SetArgs([]string{"deploy", "-e", "platform_env", "-p", "project1", manifestPath})
	err := cmd.Execute()
	assert.NoError(t, err)

	cmd.SetArgs([]string{"deploy", "-e", "platform_env", "-p", "project2", manifestPath})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func createSettingsClient(t *testing.T, env manifest.EnvironmentDefinition, opts ...func(dynatraceClient *dtclient.DynatraceClient)) dtclient.SettingsClient {
	oauthCredentials := auth.OauthCredentials{
		ClientID:     env.Auth.OAuth.ClientID.Value,
		ClientSecret: env.Auth.OAuth.ClientSecret.Value,
		TokenURL:     env.Auth.OAuth.GetTokenEndpointValue(),
	}

	tokenClient := auth.NewTokenAuthClient(env.Auth.Token.Value)
	oauthClient := auth.NewOAuthClient(context.TODO(), oauthCredentials)

	client := rest.NewRestClient(oauthClient, nil, rest.CreateRateLimitStrategy())
	clientClassic := rest.NewRestClient(tokenClient, nil, rest.CreateRateLimitStrategy())

	rest.NewRestClient(oauthClient, nil, rest.CreateRateLimitStrategy())
	classicURL, err := metadata.GetDynatraceClassicURL(context.TODO(), rest.NewRestClient(oauthClient, nil, rest.CreateRateLimitStrategy()), env.URL.Value)
	assert.NoError(t, err)

	c, err := dtclient.NewPlatformClient(
		env.URL.Value, classicURL, client, clientClassic,
	)
	assert.NoError(t, err)

	for _, o := range opts {
		o(c)
	}

	return c
}
