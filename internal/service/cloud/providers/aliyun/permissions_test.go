package cloud

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/ram"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/sts"
)

type fakeCallerClient struct {
	identity *sts.GetCallerIdentityResponse
	err      error
}

func (f fakeCallerClient) GetCallerIdentity(*sts.GetCallerIdentityRequest) (*sts.GetCallerIdentityResponse, error) {
	return f.identity, f.err
}

type fakeRAMClient struct {
	direct      []ram.Policy
	groups      []ram.Group
	inherited   map[string][]ram.Policy
	directErr   error
	groupsErr   error
	groupErr    error
	afterDirect func()
}

func (f fakeRAMClient) ListPoliciesForUser(*ram.ListPoliciesForUserRequest) (*ram.ListPoliciesForUserResponse, error) {
	if f.afterDirect != nil {
		f.afterDirect()
	}
	if f.directErr != nil {
		return nil, f.directErr
	}
	return &ram.ListPoliciesForUserResponse{Policies: ram.PoliciesInListPoliciesForUser{Policy: f.direct}}, nil
}

func TestInspectPermissionsReportsCompleteForEmptyAssignments(t *testing.T) {
	provider := &aliyunProvider{caller: fakeCallerClient{identity: ramUser()}, ram: fakeRAMClient{}}
	result := provider.InspectPermissions(context.Background())
	if result.Status != "complete" || len(result.Policies) != 0 || result.Message != "" {
		t.Fatalf("inspection = %#v", result)
	}
}

func TestInspectPermissionsKeepsDirectPoliciesWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	provider := &aliyunProvider{
		caller: fakeCallerClient{identity: ramUser()},
		ram: fakeRAMClient{
			direct:      []ram.Policy{{PolicyName: "AliyunDNSFullAccess", PolicyType: "System"}},
			afterDirect: cancel,
		},
	}
	result := provider.InspectPermissions(ctx)
	if result.Status != "partial" || len(result.Policies) != 1 || !strings.Contains(result.Message, "取消") {
		t.Fatalf("inspection = %#v", result)
	}
}

func TestInspectPermissionsKeepsDirectPoliciesWhenGroupPolicyQueryFails(t *testing.T) {
	provider := &aliyunProvider{
		caller: fakeCallerClient{identity: ramUser()},
		ram: fakeRAMClient{
			direct:   []ram.Policy{{PolicyName: "AliyunDNSFullAccess", PolicyType: "System"}},
			groups:   []ram.Group{{GroupName: "operators"}},
			groupErr: errors.New("denied for secret-value"),
		},
	}
	result := provider.InspectPermissions(context.Background())
	if result.Status != "partial" || len(result.Policies) != 1 || !strings.Contains(result.Message, "ram:ListPoliciesForGroup") || strings.Contains(result.Message, "secret-value") {
		t.Fatalf("inspection = %#v", result)
	}
}

func TestInspectPermissionsRejectsMalformedRAMUserARN(t *testing.T) {
	identity := ramUser()
	identity.Arn = "acs:ram::999999:user/other"
	provider := &aliyunProvider{caller: fakeCallerClient{identity: identity}, ram: fakeRAMClient{}}
	result := provider.InspectPermissions(context.Background())
	if result.Status != "unavailable" || len(result.Policies) != 0 {
		t.Fatalf("inspection = %#v", result)
	}
}

func (f fakeRAMClient) ListGroupsForUser(*ram.ListGroupsForUserRequest) (*ram.ListGroupsForUserResponse, error) {
	if f.groupsErr != nil {
		return nil, f.groupsErr
	}
	return &ram.ListGroupsForUserResponse{Groups: ram.GroupsInListGroupsForUser{Group: f.groups}}, nil
}

func (f fakeRAMClient) ListPoliciesForGroup(request *ram.ListPoliciesForGroupRequest) (*ram.ListPoliciesForGroupResponse, error) {
	if f.groupErr != nil {
		return nil, f.groupErr
	}
	return &ram.ListPoliciesForGroupResponse{Policies: ram.PoliciesInListPoliciesForGroup{Policy: f.inherited[request.GroupName]}}, nil
}

func ramUser() *sts.GetCallerIdentityResponse {
	return &sts.GetCallerIdentityResponse{IdentityType: "RAMUser", AccountId: "123456", Arn: "acs:ram::123456:user/cylism-manager"}
}

func TestInspectPermissionsIncludesDirectAndGroupAssignments(t *testing.T) {
	provider := &aliyunProvider{
		caller: fakeCallerClient{identity: ramUser()},
		ram: fakeRAMClient{
			direct:    []ram.Policy{{PolicyName: "AliyunDNSFullAccess", PolicyType: "System"}},
			groups:    []ram.Group{{GroupName: "operators"}},
			inherited: map[string][]ram.Policy{"operators": {{PolicyName: "AliyunOSSFullAccess", PolicyType: "System"}}},
		},
	}
	result := provider.InspectPermissions(context.Background())
	if result.Status != "complete" || result.Identity == nil || result.Identity.ARN != ramUser().Arn || len(result.Policies) != 2 {
		t.Fatalf("inspection = %#v", result)
	}
	if result.Policies[0].Source != "direct" || result.Policies[1].Source != "group" || result.Policies[1].GroupName != "operators" {
		t.Fatalf("assignments = %#v", result.Policies)
	}
}

func TestInspectPermissionsKeepsPartialResultsWithoutLeakingProviderError(t *testing.T) {
	provider := &aliyunProvider{
		caller: fakeCallerClient{identity: ramUser()},
		ram: fakeRAMClient{
			direct:    []ram.Policy{{PolicyName: "AliyunDNSFullAccess", PolicyType: "System"}},
			groupsErr: errors.New("denied for secret-value"),
		},
	}
	result := provider.InspectPermissions(context.Background())
	if result.Status != "partial" || len(result.Policies) != 1 || !strings.Contains(result.Message, "ram:ListGroupsForUser") {
		t.Fatalf("inspection = %#v", result)
	}
	if strings.Contains(result.Message, "secret-value") {
		t.Fatalf("provider error leaked: %q", result.Message)
	}
}

func TestInspectPermissionsDoesNotTreatNonRAMIdentityAsEmptyPolicies(t *testing.T) {
	provider := &aliyunProvider{caller: fakeCallerClient{identity: &sts.GetCallerIdentityResponse{IdentityType: "Account", Arn: "acs:ram::123456:root"}}, ram: fakeRAMClient{}}
	result := provider.InspectPermissions(context.Background())
	if result.Status != "unavailable" || result.Identity == nil || len(result.Policies) != 0 {
		t.Fatalf("inspection = %#v", result)
	}
}

func TestInspectPermissionsHandlesIdentityFailure(t *testing.T) {
	provider := &aliyunProvider{caller: fakeCallerClient{err: errors.New("invalid secret-value")}}
	result := provider.InspectPermissions(context.Background())
	if result.Status != "unavailable" || result.Identity != nil || strings.Contains(result.Message, "secret-value") {
		t.Fatalf("inspection = %#v", result)
	}
}
