package cloud

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"slices"

	"github.com/outscale/goutils/k8s/tags"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"github.com/samber/lo"
	"k8s.io/klog/v2"
)

type cloudVIP struct {
	*Cloud
}

func vipToStatus(n *osc.Nic) *lbStatus {
	st := &lbStatus{
		tags:           n.Tags,
		securityGroups: lo.Map(n.SecurityGroups, func(s osc.SecurityGroupLight, _ int) string { return s.SecurityGroupId }),
	}
	if n.LinkPublicIp != nil {
		st.host = n.LinkPublicIp.PublicDnsName
		st.ip = &n.LinkPublicIp.PublicIp
		return st
	}
	pip, found := lo.Find(n.PrivateIps, func(ip osc.PrivateIp) bool { return ip.IsPrimary })
	if found {
		st.host = pip.PrivateDnsName
		st.ip = &pip.PrivateIp
	} else {
		st.host = n.PrivateDnsName
	}
	return st
}

func (c cloudVIP) getNic(ctx context.Context, l *LoadBalancer) (*osc.Nic, error) {
	res, err := c.api.OAPI().ReadNics(ctx, osc.ReadNicsRequest{
		Filters: &osc.FiltersNic{
			TagKeys:   &[]string{tags.ServiceID},
			TagValues: &[]string{l.ServiceUID},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(*res.Nics) == 0 {
		return nil, ErrNotFound
	}
	return &(*res.Nics)[0], nil
}

func (c cloudVIP) getLoadBalancer(ctx context.Context, l *LoadBalancer) (*lbStatus, error) {
	lb, err := c.getNic(ctx, l)
	if err != nil {
		return nil, err
	}

	return vipToStatus(lb), nil
}

// CreateLoadBalancer creates a load-balancer.
func (c cloudVIP) createLoadBalancer(ctx context.Context, l *LoadBalancer, backend []VM) (s *lbStatus, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("VIP: %w", err)
		}
	}()
	createRequest := osc.CreateNicRequest{
		SubnetId: l.SubnetID,
	}
	sgs := append(l.SecurityGroups, l.AdditionalSecurityGroups...)
	createRequest.SecurityGroupIds = &sgs

	klog.FromContext(ctx).V(1).Info("Creating nic")
	res, err := c.api.OAPI().CreateNic(ctx, createRequest)
	if err != nil {
		return nil, fmt.Errorf("create nic: %w", err)
	}
	nic := res.Nic

	stags := l.Tags
	if stags == nil {
		stags = map[string]string{}
	}
	stags[tags.ServiceName] = l.ServiceName
	stags[tags.ServiceID] = l.ServiceUID
	stags[c.clusterIDTagKey()] = tags.ResourceLifecycleOwned

	tagsRequest := osc.CreateTagsRequest{
		ResourceIds: []string{nic.NicId},
		Tags: lo.MapToSlice(stags, func(k, v string) osc.ResourceTag {
			return osc.ResourceTag{Key: k, Value: v}
		}),
	}

	publicIP := l.publicIP
	publicIPID := l.PublicIPID
	if !l.Internal && publicIP == "" {
		klog.FromContext(ctx).V(1).Info("Creating public IP")
		res, err := c.api.OAPI().CreatePublicIp(ctx, osc.CreatePublicIpRequest{})
		if err != nil {
			return nil, fmt.Errorf("create public IP: %w", err)
		}
		publicIPID, publicIP = res.PublicIp.PublicIpId, res.PublicIp.PublicIp
		tagsRequest.ResourceIds = append(tagsRequest.ResourceIds, publicIPID)
	}
	slices.SortFunc(tagsRequest.Tags, func(a, b osc.ResourceTag) int {
		switch {
		case a.Key < b.Key:
			return -1
		case a.Key > b.Key:
			return 1
		default:
			return 0
		}
	})
	klog.FromContext(ctx).V(1).Info("Creating tags")
	_, err = c.api.OAPI().CreateTags(ctx, tagsRequest)
	if err != nil {
		return nil, fmt.Errorf("create tags: %w", err)
	}

	klog.FromContext(ctx).V(1).Info("Link public IP")
	_, err = c.api.OAPI().LinkPublicIp(ctx, osc.LinkPublicIpRequest{
		PublicIpId: &publicIPID,
		NicId:      &nic.NicId,
	})
	if err != nil {
		return nil, fmt.Errorf("link public IP: %w", err)
	}

	err = c.updateBackendVms(ctx, l, backend, nic)
	if err != nil {
		return nil, err
	}

	return vipToStatus(nic), nil
}

// UpdateLoadBalancer updates a load-balancer.
func (c cloudVIP) updateLoadBalancer(ctx context.Context, l *LoadBalancer, backend []VM) (st *lbStatus, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("VIP: %w", err)
		}
	}()
	existing, err := c.getNic(ctx, l)
	if err != nil {
		return nil, fmt.Errorf("check LB: %w", err)
	}
	if existing == nil {
		return nil, errors.New("existing LBU not found")
	}
	if err == nil {
		err = c.updateBackendVms(ctx, l, backend, existing)
	}

	return vipToStatus(existing), nil
}

func (c cloudVIP) updateBackendVms(ctx context.Context, l *LoadBalancer, vms []VM, existing *osc.Nic) error {
	// no change if nic linked to a running VM
	if existing.LinkNic != nil && slices.ContainsFunc(vms, func(v VM) bool {
		return v.ID == existing.LinkNic.VmId && v.State == osc.VmStateRunning
	}) {
		return nil
	}
	if existing.LinkNic != nil {
		klog.FromContext(ctx).V(2).Info("Unlinking nic from old Vm", "vmId", existing.LinkNic.VmId)
		_, err := c.api.OAPI().UnlinkNic(ctx, osc.UnlinkNicRequest{
			LinkNicId: existing.LinkNic.LinkNicId,
		})
		if err != nil {
			return fmt.Errorf("unlink nic: %w", err)
		}
	}
	vmId := vms[rand.Intn(len(vms))].ID
	klog.FromContext(ctx).V(2).Info("Linking nic to new Vm", "vmId", vmId)
	_, err := c.api.OAPI().LinkNic(ctx, osc.LinkNicRequest{
		DeviceNumber: l.DeviceNumber,
		NicId:        existing.NicId,
		VmId:         vmId,
	})
	if err != nil {
		return fmt.Errorf("unlink nic: %w", err)
	}

	return nil
}

// DeleteLoadBalancer deletes a load balancer.
func (c cloudVIP) deleteLoadBalancer(ctx context.Context, l *LoadBalancer) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("VIP: %w", err)
		}
	}()
	n, err := c.getNic(ctx, l)
	if err != nil {
		return fmt.Errorf("find nic: %w", err)
	}
	klog.FromContext(ctx).V(2).Info("Unlinking Nic")
	if n.LinkNic != nil {
		_, err = c.api.OAPI().UnlinkNic(ctx, osc.UnlinkNicRequest{LinkNicId: n.LinkNic.LinkNicId})
	}
	_, err = c.api.OAPI().DeleteNic(ctx, osc.DeleteNicRequest{
		NicId: n.NicId,
	})
	if err != nil {
		return fmt.Errorf("delete LBU: %w", err)
	}
	// TODO: mark public IP for deletion
	return nil
}
