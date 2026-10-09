package cloud

import (
	"context"
	"errors"
	"regexp"
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
	if err != nil {
		result.Message = callerIdentityErrorMessage(err)
		return result
	}
	if identity == nil {
		result.Message = "阿里云 STS 未返回调用身份，请稍后重试"
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

var aliyunDiagnosticID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

func callerIdentityErrorMessage(err error) string {
	message := "阿里云 STS 身份查询失败，请检查平台到 STS 的网络、DNS、代理及系统时间"
	var coded interface{ ErrorCode() string }
	if errors.As(err, &coded) {
		code := coded.ErrorCode()
		switch {
		case strings.EqualFold(code, "InvalidProtocol.NeedSsl"):
			message = "阿里云 STS 要求 HTTPS，请重启平台 API 以使用更新后的连接配置"
		case strings.Contains(strings.ToLower(code), "accesskey"), strings.Contains(strings.ToLower(code), "signature"):
			message = "阿里云拒绝 AccessKey 认证，请核对 AccessKey ID 和 Secret 是否匹配、有效"
		case strings.Contains(strings.ToLower(code), "denied"), strings.Contains(strings.ToLower(code), "forbidden"), strings.Contains(strings.ToLower(code), "permission"):
			message = "阿里云拒绝 STS 身份查询，请检查该身份的访问限制"
		default:
			message = "阿里云 STS 身份查询失败，请根据错误码检查服务状态、地域和请求时间"
		}
		if aliyunDiagnosticID.MatchString(code) {
			message += "；错误码：" + code
		}
	}
	var requested interface{ RequestId() string }
	if errors.As(err, &requested) && aliyunDiagnosticID.MatchString(requested.RequestId()) {
		message += "；RequestId：" + requested.RequestId()
	}
	return message
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
