// +build !confonly

package router

import (
	"time"

	"v2ray.com/core/common/net"
	"v2ray.com/core/features/outbound"
)

// CIDRList is an alias of []*CIDR to provide sort.Interface.
type CIDRList []*CIDR

// Len implements sort.Interface.
func (l *CIDRList) Len() int {
	return len(*l)
}

// Less implements sort.Interface.
func (l *CIDRList) Less(i int, j int) bool {
	ci := (*l)[i]
	cj := (*l)[j]

	if len(ci.Ip) < len(cj.Ip) {
		return true
	}

	if len(ci.Ip) > len(cj.Ip) {
		return false
	}

	for k := 0; k < len(ci.Ip); k++ {
		if ci.Ip[k] < cj.Ip[k] {
			return true
		}

		if ci.Ip[k] > cj.Ip[k] {
			return false
		}
	}

	return ci.Prefix < cj.Prefix
}

// Swap implements sort.Interface.
func (l *CIDRList) Swap(i int, j int) {
	(*l)[i], (*l)[j] = (*l)[j], (*l)[i]
}

type Rule struct {
	Tag       string
	Balancer  *Balancer
	Condition Condition
}

func (r *Rule) GetTag() (string, error) {
	if r.Balancer != nil {
		return r.Balancer.PickOutbound()
	}
	return r.Tag, nil
}

func (r *Rule) Apply(ctx *Context) bool {
	return r.Condition.Apply(ctx)
}

func (rr *RoutingRule) BuildCondition() (Condition, error) {
	conds := NewConditionChan()

	if len(rr.Domain) > 0 {
		matcher, err := NewDomainMatcher(rr.Domain)
		if err != nil {
			return nil, newError("failed to build domain condition").Base(err)
		}
		conds.Add(matcher)
	}

	if len(rr.UserEmail) > 0 {
		conds.Add(NewUserMatcher(rr.UserEmail))
	}

	if len(rr.InboundTag) > 0 {
		conds.Add(NewInboundTagMatcher(rr.InboundTag))
	}

	if rr.PortList != nil {
		conds.Add(NewPortMatcher(rr.PortList))
	} else if rr.PortRange != nil {
		conds.Add(NewPortMatcher(&net.PortList{Range: []*net.PortRange{rr.PortRange}}))
	}

	if len(rr.Networks) > 0 {
		conds.Add(NewNetworkMatcher(rr.Networks))
	} else if rr.NetworkList != nil {
		conds.Add(NewNetworkMatcher(rr.NetworkList.Network))
	}

	if len(rr.Geoip) > 0 {
		cond, err := NewMultiGeoIPMatcher(rr.Geoip, false)
		if err != nil {
			return nil, err
		}
		conds.Add(cond)
	} else if len(rr.Cidr) > 0 {
		cond, err := NewMultiGeoIPMatcher([]*GeoIP{{Cidr: rr.Cidr}}, false)
		if err != nil {
			return nil, err
		}
		conds.Add(cond)
	}

	if len(rr.SourceGeoip) > 0 {
		cond, err := NewMultiGeoIPMatcher(rr.SourceGeoip, true)
		if err != nil {
			return nil, err
		}
		conds.Add(cond)
	} else if len(rr.SourceCidr) > 0 {
		cond, err := NewMultiGeoIPMatcher([]*GeoIP{{Cidr: rr.SourceCidr}}, true)
		if err != nil {
			return nil, err
		}
		conds.Add(cond)
	}

	if len(rr.CustomGeoip) > 0 {
		conds.Add(NewCustomGeoIPMatcher(rr.CustomGeoip, false))
	}

	if len(rr.Protocol) > 0 {
		conds.Add(NewProtocolMatcher(rr.Protocol))
	}

	if len(rr.Attributes) > 0 {
		cond, err := NewAttributeMatcher(rr.Attributes)
		if err != nil {
			return nil, err
		}
		conds.Add(cond)
	}
	if len(rr.Application) > 0 {
		conds.Add(NewApplicationMatcher(rr.Application))
	}

	if conds.Len() == 0 {
		return nil, newError("this rule has no effective fields").AtWarning()
	}

	return conds, nil
}

func (br *BalancingRule) Build(ohm outbound.Manager) (*Balancer, error) {
	var strategy BalancingStrategy
	if br.BalancingStrategy == BalancingRule_Random {
		strategy = &RandomStrategy{}
	}
	if br.BalancingStrategy == BalancingRule_Latency {
		if ls := NewLatencyStrategy(
			ohm,
			br.OutboundSelector,
			int(br.TotalMeasures),
			time.Duration(br.Interval)*time.Second,
			time.Duration(br.Delay)*time.Second,
			time.Duration(br.Timeout)*time.Second,
			time.Duration(br.Tolerance)*time.Millisecond,
			br.ProbeTarget,
			br.ProbeContent,
		); ls != nil {
			strategy = ls
		}
	}
	return &Balancer{
		selectors: br.OutboundSelector,
		strategy:  strategy,
		ohm:       ohm,
	}, nil
}
