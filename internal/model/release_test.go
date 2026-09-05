package model

import "testing"

func TestReleaseStatusValidation(t *testing.T) {
	release := Release{ApplicationID: 1, Sequence: 2, Image: "registry.example.com/order-api@sha256:abc", Status: ReleaseStatusSucceeded, DesiredSpec: `{"service":{"port":80}}`}
	if release.Status != ReleaseStatusSucceeded || release.Sequence != 2 {
		t.Fatalf("unexpected release: %+v", release)
	}
	if !IsReleaseStatusValid(ReleaseStatusRollingBack) || IsReleaseStatusValid("unknown") {
		t.Fatal("release status validation is incorrect")
	}
}
