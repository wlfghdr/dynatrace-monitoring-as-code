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

package rest

import (
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/api"
	"gotest.tools/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

var mockApi = api.NewApi("mock-api", "/mock-api", "", true, true, "", false)
var mockApiNotSingle = api.NewApi("mock-api", "/mock-api", "", false, true, "", false)

func TestNewClientNoUrl(t *testing.T) {
	_, err := NewDynatraceClient("", "abc")
	assert.ErrorContains(t, err, "no environment url")
}

func TestUrlSuffixGetsTrimmed(t *testing.T) {
	client, err := newDynatraceClient("https://my-environment.live.dynatrace.com/", "abc", nil)
	assert.NilError(t, err)
	assert.Equal(t, client.environmentUrl, "https://my-environment.live.dynatrace.com")
}

func TestNewClientNoToken(t *testing.T) {
	_, err := NewDynatraceClient("http://my-environment.live.dynatrace.com/", "")
	assert.ErrorContains(t, err, "no token")
}

func TestNewClientNoValidUrlLocalPath(t *testing.T) {
	_, err := NewDynatraceClient("/my-environment/live/dynatrace.com/", "abc")
	assert.ErrorContains(t, err, "not valid")
}

func TestNewClientNoValidUrlTypo(t *testing.T) {
	_, err := NewDynatraceClient("https//my-environment.live.dynatrace.com/", "abc")
	assert.ErrorContains(t, err, "not valid")
}

func TestNewClientNoValidUrlNoHttps(t *testing.T) {
	_, err := NewDynatraceClient("http//my-environment.live.dynatrace.com/", "abc")
	assert.ErrorContains(t, err, "not valid")
}

func TestNewClient(t *testing.T) {
	_, err := NewDynatraceClient("https://my-environment.live.dynatrace.com/", "abc")
	assert.NilError(t, err, "not valid")
}

func TestReadByIdReturnsAnErrorUponEncounteringAnError(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		http.Error(res, "", http.StatusForbidden)
	}))
	defer func() { testServer.Close() }()
	client := newDynatraceClientForTesting(testServer)

	_, err := client.ReadById(mockApi, "test")
	assert.ErrorContains(t, err, "Response was")
}

func TestReadByIdEscapesTheId(t *testing.T) {
	unescapedId := "ruxit.perfmon.dotnetV4:%TimeInGC:time_in_gc_alert_high_generic"

	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {}))
	defer func() { testServer.Close() }()
	client := newDynatraceClientForTesting(testServer)

	_, err := client.ReadById(mockApiNotSingle, unescapedId)
	assert.NilError(t, err)
}

func TestReadByIdReturnsTheResponseGivenNoError(t *testing.T) {
	body := []byte{1, 3, 3, 7}

	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Write(body)
	}))
	defer func() { testServer.Close() }()

	client := newDynatraceClientForTesting(testServer)

	resp, err := client.ReadById(mockApi, "test")
	assert.NilError(t, err, "there should not be an error")
	assert.DeepEqual(t, body, resp)
}
