// +build !providerless

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

package osc

import (
	"encoding/json"
	"fmt"

	"github.com/outscale/osc-sdk-go/osc"
)

// SecurityGroupRuleSet maps IP strings of strings to OSC SecurityGroupRule
type SecurityGroupRuleSet map[string]osc.SecurityGroupRule

// SecurityGroupRulePredicate is an predicate to test whether SecurityGroupRule matches some condition.
type SecurityGroupRulePredicate interface {
	// Test checks whether specified SecurityGroupRule matches condition.
	Test(perm osc.SecurityGroupRule) bool
}

// NewSecurityGroupRuleSet creates a new SecurityGroupRuleSet
func NewSecurityGroupRuleSet(items ...osc.SecurityGroupRule) SecurityGroupRuleSet {
	s := make(SecurityGroupRuleSet)
	s.Insert(items...)
	return s
}

// Ungroup splits permissions out into individual permissions
// OSC will combine permissions with the same port but different SourceRanges together, for example
// We ungroup them so we can process them
func (s SecurityGroupRuleSet) Ungroup() SecurityGroupRuleSet {
	l := []osc.SecurityGroupRule{}
	for _, p := range s.List() {
		if len(p.IpRanges) <= 1 {
			l = append(l, p)
			continue
		}
		for _, ipRange := range p.IpRanges {
			c := osc.SecurityGroupRule{}
			c = p
			c.IpRanges = []string{ipRange}
			l = append(l, c)
		}
	}

	l2 := []osc.SecurityGroupRule{}
	for _, p := range l {
		if len(p.SecurityGroupsMembers) <= 1 {
			l2 = append(l2, p)
			continue
		}
		for _, u := range p.SecurityGroupsMembers {
			c := osc.SecurityGroupRule{}
			c = p
			c.SecurityGroupsMembers = []osc.SecurityGroupsMember{u}
			l2 = append(l, c)
		}
	}

	l3 := []osc.SecurityGroupRule{}
	for _, p := range l2 {
		if len(p.IpRanges) <= 1 {
			l3 = append(l3, p)
			continue
		}
		for _, v := range p.IpRanges {
			c := osc.SecurityGroupRule{}
			c = p
			c.IpRanges = []string{v}
			l3 = append(l3, c)
		}
	}

	return NewSecurityGroupRuleSet(l3...)
}

// Insert adds items to the set.
func (s SecurityGroupRuleSet) Insert(items ...osc.SecurityGroupRule) {
	for _, p := range items {
		k := keyForSecurityGroupRule(p)
		s[k] = p
	}
}

// Delete delete permission from the set.
func (s SecurityGroupRuleSet) Delete(items ...osc.SecurityGroupRule) {
	for _, p := range items {
		k := keyForSecurityGroupRule(p)
		delete(s, k)
	}
}

// DeleteIf delete permission from the set if permission matches predicate.
func (s SecurityGroupRuleSet) DeleteIf(predicate SecurityGroupRulePredicate) {
	for k, p := range s {
		if predicate.Test(p) {
			delete(s, k)
		}
	}
}

// List returns the contents as a slice.  Order is not defined.
func (s SecurityGroupRuleSet) List() []osc.SecurityGroupRule {
	res := make([]osc.SecurityGroupRule, 0, len(s))
	for _, v := range s {
		res = append(res, v)
	}
	return res
}

// IsSuperset returns true if and only if s is a superset of s2.
func (s SecurityGroupRuleSet) IsSuperset(s2 SecurityGroupRuleSet) bool {
	for k := range s2 {
		_, found := s[k]
		if !found {
			return false
		}
	}
	return true
}

// Equal returns true if and only if s is equal (as a set) to s2.
// Two sets are equal if their membership is identical.
// (In practice, this means same elements, order doesn't matter)
func (s SecurityGroupRuleSet) Equal(s2 SecurityGroupRuleSet) bool {
	return len(s) == len(s2) && s.IsSuperset(s2)
}

// Difference returns a set of objects that are not in s2
// For example:
// s1 = {a1, a2, a3}
// s2 = {a1, a2, a4, a5}
// s1.Difference(s2) = {a3}
// s2.Difference(s1) = {a4, a5}
func (s SecurityGroupRuleSet) Difference(s2 SecurityGroupRuleSet) SecurityGroupRuleSet {
	result := NewSecurityGroupRuleSet()
	for k, v := range s {
		_, found := s2[k]
		if !found {
			result[k] = v
		}
	}
	return result
}

// Len returns the size of the set.
func (s SecurityGroupRuleSet) Len() int {
	return len(s)
}

func keyForSecurityGroupRule(p osc.SecurityGroupRule) string {
	v, err := json.Marshal(p)
	if err != nil {
		panic(fmt.Sprintf("error building JSON representation of osc.SecurityGroupRule: %v", err))
	}
	return string(v)
}

var _ SecurityGroupRulePredicate = SecurityGroupRuleMatchDesc{}

// SecurityGroupRuleMatchDesc checks whether specific SecurityGroupRule contains description.
type SecurityGroupRuleMatchDesc struct {
	Description string
}

// Test whether specific SecurityGroupRule contains description.
func (p SecurityGroupRuleMatchDesc) Test(perm osc.SecurityGroupRule) bool {
	for _, v4Range := range perm.IpRanges {
		if v4Range == p.Description {
			return true
		}
	}
// NO IPV6
// 	for _, v6Range := range perm.Ipv6Ranges {
// 		if v6Range.Description == p.Description {
// 			return true
// 		}
// 	}

	for _, prefixListID := range perm.IpRanges {
		if prefixListID == p.Description {
			return true
		}
	}
	for _, group := range perm.SecurityGroupsMembers {
	    // A verififer si c'est bien SecurityGroupId
		if group.SecurityGroupId == p.Description {
			return true
		}
	}
	return false
}

var _ SecurityGroupRulePredicate = SecurityGroupRuleNotMatch{}

// SecurityGroupRuleNotMatch is the *not* operator for Predicate
type SecurityGroupRuleNotMatch struct {
	Predicate SecurityGroupRulePredicate
}

// Test whether specific SecurityGroupRule not match the embed predicate.
func (p SecurityGroupRuleNotMatch) Test(perm osc.SecurityGroupRule) bool {
	return !p.Predicate.Test(perm)
}
