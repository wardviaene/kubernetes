/*
Copyright 2016 The Kubernetes Authors.

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

package jws

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	content = "Hello from the other side. I must have called a thousand times."
	secret  = "my voice is my passcode. my voice is my passcode"
	id      = "joshua"
)

func TestComputeDetachedSignature(t *testing.T) {
	sig, err := ComputeDetachedSignature(content, id, secret)
	assert.NoError(t, err, "Error when computing signature: %v", err)
	assert.Equal(
		t,
		"eyJhbGciOiJIUzI1NiIsImtpZCI6Impvc2h1YSJ9..SFoKn0-YxqnvpSExt4LuCZ_7pvIBdX3SoUO_NsTpkdg",
		sig,
		"Wrong signature. Got: %v", sig)

	// Try with null content
	sig, err = ComputeDetachedSignature("", id, secret)
	assert.NoError(t, err, "Error when computing signature: %v", err)
	assert.Equal(
		t,
		"eyJhbGciOiJIUzI1NiIsImtpZCI6Impvc2h1YSJ9..kzZha466Sbz_vFhUBjjYp2rl3F7yHgdDdUIBGxQg4js",
		sig,
		"Wrong signature. Got: %v", sig)

}

func TestDetachedTokenIsValid(t *testing.T) {
	// Valid detached JWS token and valid inputs should succeed
	sig := "eyJhbGciOiJIUzI1NiIsImtpZCI6Impvc2h1YSJ9..SFoKn0-YxqnvpSExt4LuCZ_7pvIBdX3SoUO_NsTpkdg"
	assert.True(t, DetachedTokenIsValid(sig, content, id, secret),
		"Content %q and token \"%s:%s\" should equal signature: %q", content, id, secret, sig)

	// Invalid detached JWS token and valid inputs should fail
	sig2 := sig + "foo"
	assert.False(t, DetachedTokenIsValid(sig2, content, id, secret),
		"Content %q and token \"%s:%s\" should not equal signature: %q", content, id, secret, sig)
}
