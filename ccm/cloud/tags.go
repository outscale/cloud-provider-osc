/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package cloud

const (
	// SGToDeleteTagKey is a tag key that is added to all SG requiring to be deleted
	SGToDeleteTagKey = "OscK8sToDelete"

	// MainSGTagKeyPrefix The main sg Tag
	// The tag key = OscK8sMainSG/clusterId
	MainSGTagKeyPrefix = "OscK8sMainSG/"
)

func mainSGTagKey(clusterID string) string {
	return MainSGTagKeyPrefix + clusterID
}
