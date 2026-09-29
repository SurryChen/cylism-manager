package cloud

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/ram"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/sts"
	cloudservice "github.com/cylism/cylism-manager/internal/service/cloud"
)

func (p *aliyunProvider) InspectPermissions(ctx context.Context) cloudservice.PermissionInspection {
	result := cloudservice.PermissionInspection{
		Status: "unavailable", InspectedAt: time.Now().UTC(), Policies: []cloudservice.AssignedPolicy{},
	}
	if p.caller == nil || p.ram == nil {
		result.Message = "阿里云权限查询客户端不可用"
		return result
	}
	if ctx.Err() != nil {
		result.Message = "权限查询已取消"
		return result
	}
	identity, err := p.caller.GetCallerIdentity(sts.CreateGetCallerIdentityRequest())
	if err != nil || identity == nil {
		result.Message = "无法识别 AccessKey 对应身份，请检查密钥是否有效"
		return result
	}
	result.Identity = &cloudservice.PermissionIdentity{Type: identity.IdentityType, ARN: identity.Arn}
	userName, ok := ramUserName(identity)
	if !ok {
		result.Message = "仅支持查看 RAM 用户的授权策略"
		return result
	}

	successfulQueries := 0
	missing := []string{}
	directRequest := ram.CreateListPoliciesForUserRequest()
	directRequest.UserName = userName
	if policies, err := p.ram.ListPoliciesForUser(directRequest); err == nil && policies != nil {
		successfulQueries++
		for _, policy := range policies.Policies.Policy {
			result.Policies = append(result.Policies, assignedPolicy(policy, "direct", ""))
		}
	} else {
		missing = append(missing, "ram:ListPoliciesForUser")
	}

	if ctx.Err() != nil {
		if successfulQueries > 0 {
			result.Status = "partial"
		}
		result.Message = "权限查询已取消"
		return result
	}
	groupsRequest := ram.CreateListGroupsForUserRequest()
	groupsRequest.UserName = userName
	groups, err := p.ram.ListGroupsForUser(groupsRequest)
	if err != nil || groups == nil {
		missing = append(missing, "ram:ListGroupsForUser")
	} else {
		successfulQueries++
		for _, group := range groups.Groups.Group {
			if ctx.Err() != nil {
				if successfulQueries > 0 {
					result.Status = "partial"
				}
				result.Message = "权限查询已取消"
				return result
			}
			request := ram.CreateListPoliciesForGroupRequest()
			request.GroupName = group.GroupName
			policies, err := p.ram.ListPoliciesForGroup(request)
			if err != nil || policies == nil {
				missing = append(missing, "ram:ListPoliciesForGroup ("+group.GroupName+")")
				continue
			}
			successfulQueries++
			for _, policy := range policies.Policies.Policy {
				result.Policies = append(result.Policies, assignedPolicy(policy, "group", group.GroupName))
			}
		}
	}

	sort.Slice(result.Policies, func(i, j int) bool {
		a, b := result.Policies[i], result.Policies[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.GroupName != b.GroupName {
			return a.GroupName < b.GroupName
		}
		return a.Name < b.Name
	})
	if len(missing) == 0 {
		result.Status = "complete"
	} else {
		if successfulQueries > 0 {
			result.Status = "partial"
		}
		result.Message = "部分 RAM 授权信息无法读取，请检查 " + strings.Join(missing, "、") + " 权限"
	}
	return result
}

func ramUserName(identity *sts.GetCallerIdentityResponse) (string, bool) {
	if identity == nil || !strings.EqualFold(identity.IdentityType, "RAMUser") || identity.AccountId == "" {
		return "", false
	}
	name, ok := strings.CutPrefix(identity.Arn, "acs:ram::"+identity.AccountId+":user/")
	return name, ok && name != "" && !strings.ContainsAny(name, " \t\r\n:")
}

func assignedPolicy(policy ram.Policy, source, groupName string) cloudservice.AssignedPolicy {
	return cloudservice.AssignedPolicy{Name: policy.PolicyName, Type: policy.PolicyType, Source: source, GroupName: groupName}
}
