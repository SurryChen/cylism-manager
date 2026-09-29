package cloud

import (
	"context"
	"time"
)

type PermissionIdentity struct {
	Type string `json:"type"`
	ARN  string `json:"arn"`
}

type AssignedPolicy struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Source    string `json:"source"`
	GroupName string `json:"group_name,omitempty"`
}

// PermissionInspection describes policy assignments, not effective authorization.
type PermissionInspection struct {
	Status      string              `json:"status"`
	InspectedAt time.Time           `json:"inspected_at"`
	Identity    *PermissionIdentity `json:"identity,omitempty"`
	Policies    []AssignedPolicy    `json:"policies"`
	Message     string              `json:"message,omitempty"`
}

type PermissionInspector interface {
	InspectPermissions(context.Context) PermissionInspection
}

func (s *Service) InspectPermissions(ctx context.Context, id uint) (PermissionInspection, error) {
	_, provider, err := s.provider(id)
	if err != nil {
		return PermissionInspection{}, err
	}
	inspector, ok := provider.(PermissionInspector)
	if !ok {
		return PermissionInspection{Status: "unsupported", InspectedAt: time.Now().UTC(), Policies: []AssignedPolicy{}, Message: "该云提供商暂不支持权限查看"}, nil
	}
	result := inspector.InspectPermissions(ctx)
	if result.InspectedAt.IsZero() {
		result.InspectedAt = time.Now().UTC()
	}
	if result.Policies == nil {
		result.Policies = []AssignedPolicy{}
	}
	return result, nil
}
