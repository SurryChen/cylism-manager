package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// RemovalNode freezes the Kubernetes and host identities used by a removal task.
type RemovalNode struct {
	Name         string   `json:"name"`
	UID          string   `json:"uid"`
	MachineID    string   `json:"-"`
	PodCIDRs     []string `json:"-"`
	ControlPlane bool     `json:"control_plane"`
	Ready        bool     `json:"ready"`
}

func (c *Client) GetRemovalNodeContext(ctx context.Context, name string) (*RemovalNode, error) {
	node, err := c.Clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	cidrs := append([]string(nil), node.Spec.PodCIDRs...)
	if len(cidrs) == 0 && node.Spec.PodCIDR != "" {
		cidrs = []string{node.Spec.PodCIDR}
	}
	return &RemovalNode{Name: node.Name, UID: string(node.UID), MachineID: node.Status.NodeInfo.MachineID, PodCIDRs: cidrs, ControlPlane: isControlPlaneNode(node), Ready: nodeReady(node)}, nil
}

func (c *Client) DrainRemovalNodeContext(ctx context.Context, name, uid string, options DrainOptions) (*DrainResult, error) {
	if uid == "" {
		return nil, fmt.Errorf("节点身份缺失")
	}
	return c.drainNodeContext(ctx, name, options, types.UID(uid))
}

// DeleteRemovalNodeContext cannot delete a replacement with the same name, or a
// node whose state changed after the final safety check.
func (c *Client) DeleteRemovalNodeContext(ctx context.Context, name, uid string) error {
	node, err := c.Clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if uid == "" || string(node.UID) != uid || isControlPlaneNode(node) {
		return fmt.Errorf("节点身份或角色已变化，请重新检查")
	}
	check, err := c.NodeRemovalCheckContext(ctx, name)
	if err != nil {
		return err
	}
	if !check.CanRemove {
		return fmt.Errorf("节点尚未满足移出条件：%s", check.Blockers[0].Reason)
	}
	return c.Clientset.CoreV1().Nodes().Delete(ctx, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &node.UID, ResourceVersion: &node.ResourceVersion}})
}
