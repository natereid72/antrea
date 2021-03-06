// Copyright 2020 Antrea Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package stats

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"

	oftest "github.com/vmware-tanzu/antrea/pkg/agent/openflow/testing"
	agenttypes "github.com/vmware-tanzu/antrea/pkg/agent/types"
	cpv1beta1 "github.com/vmware-tanzu/antrea/pkg/apis/controlplane/v1beta1"
	statsv1alpha1 "github.com/vmware-tanzu/antrea/pkg/apis/stats/v1alpha1"
)

var (
	np1 = cpv1beta1.NetworkPolicyReference{
		Type:      cpv1beta1.K8sNetworkPolicy,
		Namespace: "foo",
		Name:      "bar",
		UID:       "uid1",
	}
	np2 = cpv1beta1.NetworkPolicyReference{
		Type:      cpv1beta1.K8sNetworkPolicy,
		Namespace: "foo",
		Name:      "baz",
		UID:       "uid2",
	}
	acnp1 = cpv1beta1.NetworkPolicyReference{
		Type:      cpv1beta1.AntreaClusterNetworkPolicy,
		Namespace: "",
		Name:      "baz",
		UID:       "uid3",
	}
	anp1 = cpv1beta1.NetworkPolicyReference{
		Type:      cpv1beta1.AntreaNetworkPolicy,
		Namespace: "foo",
		Name:      "bar",
		UID:       "uid4",
	}
)

func TestCollect(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name                    string
		ruleStats               map[uint32]*agenttypes.RuleMetric
		ofIDToPolicyMap         map[uint32]*cpv1beta1.NetworkPolicyReference
		expectedStatsCollection *statsCollection
	}{
		{
			name: "one or multiple rules per policy",
			ruleStats: map[uint32]*agenttypes.RuleMetric{
				1: {
					Bytes:    10,
					Packets:  1,
					Sessions: 1,
				},
				2: {
					Bytes:    15,
					Packets:  2,
					Sessions: 1,
				},
				3: {
					Bytes:    30,
					Packets:  5,
					Sessions: 3,
				},
			},
			ofIDToPolicyMap: map[uint32]*cpv1beta1.NetworkPolicyReference{
				1: &np1,
				2: &np1,
				3: &np2,
			},
			expectedStatsCollection: &statsCollection{
				networkPolicyStats: map[types.UID]*statsv1alpha1.TrafficStats{
					np1.UID: {
						Bytes:    25,
						Packets:  3,
						Sessions: 2,
					},
					np2.UID: {
						Bytes:    30,
						Packets:  5,
						Sessions: 3,
					},
				},
				antreaClusterNetworkPolicyStats: map[types.UID]*statsv1alpha1.TrafficStats{},
				antreaNetworkPolicyStats:        map[types.UID]*statsv1alpha1.TrafficStats{},
			},
		},
		{
			name: "blended policies",
			ruleStats: map[uint32]*agenttypes.RuleMetric{
				1: {
					Bytes:    10,
					Packets:  1,
					Sessions: 1,
				},
				2: {
					Bytes:    15,
					Packets:  2,
					Sessions: 1,
				},
				3: {
					Bytes:    30,
					Packets:  5,
					Sessions: 3,
				},
			},
			ofIDToPolicyMap: map[uint32]*cpv1beta1.NetworkPolicyReference{
				1: &np1,
				2: &acnp1,
				3: &anp1,
			},
			expectedStatsCollection: &statsCollection{
				networkPolicyStats: map[types.UID]*statsv1alpha1.TrafficStats{
					np1.UID: {
						Bytes:    10,
						Packets:  1,
						Sessions: 1,
					},
				},
				antreaClusterNetworkPolicyStats: map[types.UID]*statsv1alpha1.TrafficStats{
					acnp1.UID: {
						Bytes:    15,
						Packets:  2,
						Sessions: 1,
					},
				},
				antreaNetworkPolicyStats: map[types.UID]*statsv1alpha1.TrafficStats{
					anp1.UID: {
						Bytes:    30,
						Packets:  5,
						Sessions: 3,
					},
				},
			},
		},
		{
			name: "unknown policy",
			ruleStats: map[uint32]*agenttypes.RuleMetric{
				1: {
					Bytes:    10,
					Packets:  1,
					Sessions: 1,
				},
				2: {
					Bytes:    15,
					Packets:  2,
					Sessions: 1,
				},
			},
			ofIDToPolicyMap: map[uint32]*cpv1beta1.NetworkPolicyReference{
				1: &np1,
				2: nil,
			},
			expectedStatsCollection: &statsCollection{
				networkPolicyStats: map[types.UID]*statsv1alpha1.TrafficStats{
					np1.UID: {
						Bytes:    10,
						Packets:  1,
						Sessions: 1,
					},
				},
				antreaClusterNetworkPolicyStats: map[types.UID]*statsv1alpha1.TrafficStats{},
				antreaNetworkPolicyStats:        map[types.UID]*statsv1alpha1.TrafficStats{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ofClient := oftest.NewMockClient(ctrl)
			ofClient.EXPECT().NetworkPolicyMetrics().Return(tt.ruleStats).Times(1)
			for ofID, policy := range tt.ofIDToPolicyMap {
				ofClient.EXPECT().GetPolicyFromConjunction(ofID).Return(policy)
			}

			m := &Collector{ofClient: ofClient}
			actualPolicyStats := m.collect()
			assert.Equal(t, tt.expectedStatsCollection, actualPolicyStats)
		})
	}
}

func TestCalculateDiff(t *testing.T) {
	tests := []struct {
		name              string
		lastStats         map[types.UID]*statsv1alpha1.TrafficStats
		curStats          map[types.UID]*statsv1alpha1.TrafficStats
		expectedstatsList []cpv1beta1.NetworkPolicyStats
	}{
		{
			name: "new networkpolicy and existing networkpolicy",
			lastStats: map[types.UID]*statsv1alpha1.TrafficStats{
				"uid1": {
					Bytes:    1,
					Packets:  1,
					Sessions: 1,
				},
			},
			curStats: map[types.UID]*statsv1alpha1.TrafficStats{
				"uid1": {
					Bytes:    25,
					Packets:  3,
					Sessions: 2,
				},
				"uid2": {
					Bytes:    30,
					Packets:  5,
					Sessions: 3,
				},
			},
			expectedstatsList: []cpv1beta1.NetworkPolicyStats{
				{
					NetworkPolicy: cpv1beta1.NetworkPolicyReference{UID: "uid1"},
					TrafficStats: statsv1alpha1.TrafficStats{
						Bytes:    24,
						Packets:  2,
						Sessions: 1,
					},
				},
				{
					NetworkPolicy: cpv1beta1.NetworkPolicyReference{UID: "uid2"},
					TrafficStats: statsv1alpha1.TrafficStats{
						Bytes:    30,
						Packets:  5,
						Sessions: 3,
					},
				},
			},
		},
		{
			name: "unchanged networkpolicy",
			lastStats: map[types.UID]*statsv1alpha1.TrafficStats{
				"uid1": {
					Bytes:    1,
					Packets:  1,
					Sessions: 1,
				},
				"uid2": {
					Bytes:    0,
					Packets:  0,
					Sessions: 0,
				},
			},
			curStats: map[types.UID]*statsv1alpha1.TrafficStats{
				"uid1": {
					Bytes:    1,
					Packets:  1,
					Sessions: 1,
				},
				"uid2": {
					Bytes:    0,
					Packets:  0,
					Sessions: 0,
				},
			},
			expectedstatsList: []cpv1beta1.NetworkPolicyStats{},
		},
		{
			name: "negative statistic",
			lastStats: map[types.UID]*statsv1alpha1.TrafficStats{
				"uid1": {
					Bytes:    10,
					Packets:  10,
					Sessions: 10,
				},
				"uid2": {
					Bytes:    5,
					Packets:  5,
					Sessions: 5,
				},
			},
			curStats: map[types.UID]*statsv1alpha1.TrafficStats{
				"uid1": {
					Bytes:    3,
					Packets:  3,
					Sessions: 3,
				},
				"uid2": {
					Bytes:    1,
					Packets:  1,
					Sessions: 1,
				},
			},
			expectedstatsList: []cpv1beta1.NetworkPolicyStats{
				{
					NetworkPolicy: cpv1beta1.NetworkPolicyReference{UID: "uid1"},
					TrafficStats: statsv1alpha1.TrafficStats{
						Bytes:    3,
						Packets:  3,
						Sessions: 3,
					},
				},
				{
					NetworkPolicy: cpv1beta1.NetworkPolicyReference{UID: "uid2"},
					TrafficStats: statsv1alpha1.TrafficStats{
						Bytes:    1,
						Packets:  1,
						Sessions: 1,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualMetrics := calculateDiff(tt.curStats, tt.lastStats)
			assert.ElementsMatch(t, tt.expectedstatsList, actualMetrics)
		})
	}
}
