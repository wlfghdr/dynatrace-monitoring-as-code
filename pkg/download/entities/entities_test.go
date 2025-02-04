//go:build unit

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

package entities

import (
	"fmt"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/internal/idutils"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/client/dtclient"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/config"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/config/coordinate"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/config/parameter"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/config/parameter/value"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/config/template"
	v2 "github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/project/v2"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/rest"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
)

func TestDownloadAll(t *testing.T) {
	testType := "SOMETHING"
	testType2 := "SOMETHINGELSE"
	uuid := idutils.GenerateUUIDFromString(testType)

	type mockValues struct {
		EntitiesTypeList      func() ([]dtclient.EntitiesType, error)
		EntitiesTypeListCalls int
		EntitiesList          func() ([]string, error)
		EntitiesListCalls     int
	}
	tests := []struct {
		name       string
		mockValues mockValues
		want       v2.ConfigsPerType
	}{
		{
			name: "DownloadEntities - List Entity Types fails",
			mockValues: mockValues{
				EntitiesTypeList: func() ([]dtclient.EntitiesType, error) {
					return nil, rest.RespError{Err: fmt.Errorf("oh no"), StatusCode: 0}
				},
				EntitiesTypeListCalls: 1,
				EntitiesList: func() ([]string, error) {
					return nil, nil
				},
				EntitiesListCalls: 0,
			},
			want: nil,
		},
		{
			name: "DownloadEntities - List Entity fails",
			mockValues: mockValues{
				EntitiesTypeList: func() ([]dtclient.EntitiesType, error) {
					return []dtclient.EntitiesType{{EntitiesTypeId: testType}, {EntitiesTypeId: testType2}}, nil
				},
				EntitiesTypeListCalls: 1,
				EntitiesList: func() ([]string, error) {
					return nil, rest.RespError{Err: fmt.Errorf("oh no"), StatusCode: 0}
				},
				EntitiesListCalls: 2,
			},
			want: v2.ConfigsPerType{},
		},
		{
			name: "DownloadEntities",
			mockValues: mockValues{
				EntitiesTypeList: func() ([]dtclient.EntitiesType, error) {
					return []dtclient.EntitiesType{{EntitiesTypeId: testType}}, nil
				},
				EntitiesTypeListCalls: 1,
				EntitiesList: func() ([]string, error) {
					return []string{""}, nil
				},
				EntitiesListCalls: 1,
			},
			want: v2.ConfigsPerType{testType: {
				{
					Template: template.NewDownloadTemplate(testType, testType, "[]"),
					Coordinate: coordinate.Coordinate{
						Project:  "projectName",
						Type:     testType,
						ConfigId: uuid,
					},
					Type: config.EntityType{
						EntitiesType: testType,
					},
					Parameters: map[string]parameter.Parameter{
						config.NameParameter: &value.ValueParameter{Value: uuid},
					},
					Skip: false,
				},
			}},
		},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			c := dtclient.NewMockClient(gomock.NewController(t))
			entityTypeList, err := tt.mockValues.EntitiesTypeList()
			c.EXPECT().ListEntitiesTypes(gomock.Any()).Times(tt.mockValues.EntitiesTypeListCalls).Return(entityTypeList, err)
			entities, err := tt.mockValues.EntitiesList()
			c.EXPECT().ListEntities(gomock.Any(), gomock.Any()).Times(tt.mockValues.EntitiesListCalls).Return(entities, err)
			res := NewEntitiesDownloader(c).DownloadAll("projectName")
			assert.Equal(t, tt.want, res)
		})
	}
}

func TestDownload(t *testing.T) {
	testType := "SOMETHING"
	uuid := idutils.GenerateUUIDFromString(testType)

	type mockValues struct {
		EntitiesTypeList      func() ([]dtclient.EntitiesType, error)
		EntitiesTypeListCalls int
		EntitiesList          func() ([]string, error)
		EntitiesListCalls     int
	}
	tests := []struct {
		name          string
		EntitiesTypes []string
		mockValues    mockValues
		want          v2.ConfigsPerType
	}{
		{
			name: "DownloadEntities - empty list of entities types",
			mockValues: mockValues{
				EntitiesTypeList:      func() ([]dtclient.EntitiesType, error) { return []dtclient.EntitiesType{}, nil },
				EntitiesTypeListCalls: 0,
				EntitiesList:          func() ([]string, error) { return []string{}, nil },
				EntitiesListCalls:     0,
			},
			want: nil,
		},
		{
			name:          "DownloadEntities - entities list empty",
			EntitiesTypes: []string{testType},
			mockValues: mockValues{
				EntitiesTypeList: func() ([]dtclient.EntitiesType, error) {
					return []dtclient.EntitiesType{{EntitiesTypeId: testType}}, nil
				},
				EntitiesTypeListCalls: 1,
				EntitiesList: func() ([]string, error) {
					return make([]string, 0, 1), nil
				},
				EntitiesListCalls: 1,
			},
			want: v2.ConfigsPerType{},
		},
		{
			name:          "DownloadEntities - Not all entities found",
			EntitiesTypes: []string{testType, "SOMETHING_ELSE"},
			mockValues: mockValues{
				EntitiesTypeList: func() ([]dtclient.EntitiesType, error) {
					return []dtclient.EntitiesType{{EntitiesTypeId: testType}}, nil
				},
				EntitiesTypeListCalls: 1,
				EntitiesList: func() ([]string, error) {
					return make([]string, 0, 1), nil
				},
				EntitiesListCalls: 0,
			},
			want: nil,
		},
		{
			name:          "DownloadEntities - entities found",
			EntitiesTypes: []string{testType},
			mockValues: mockValues{
				EntitiesTypeList: func() ([]dtclient.EntitiesType, error) {
					return []dtclient.EntitiesType{{EntitiesTypeId: testType}}, nil
				},
				EntitiesTypeListCalls: 1,
				EntitiesList: func() ([]string, error) {
					return []string{""}, nil
				},
				EntitiesListCalls: 1,
			},
			want: v2.ConfigsPerType{testType: {
				{
					Template: template.NewDownloadTemplate(testType, testType, "[]"),
					Coordinate: coordinate.Coordinate{
						Project:  "projectName",
						Type:     testType,
						ConfigId: uuid,
					},
					Type: config.EntityType{
						EntitiesType: testType,
					},
					Parameters: map[string]parameter.Parameter{
						config.NameParameter: &value.ValueParameter{Value: uuid},
					},
					Skip: false,
				},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := dtclient.NewMockClient(gomock.NewController(t))
			entityTypeList, err := tt.mockValues.EntitiesTypeList()
			c.EXPECT().ListEntitiesTypes(gomock.Any()).Times(tt.mockValues.EntitiesTypeListCalls).Return(entityTypeList, err)
			entities, err := tt.mockValues.EntitiesList()
			c.EXPECT().ListEntities(gomock.Any(), gomock.Any()).Times(tt.mockValues.EntitiesListCalls).Return(entities, err)
			res := NewEntitiesDownloader(c).Download(tt.EntitiesTypes, "projectName")
			assert.Equal(t, tt.want, res)
		})
	}
}
