/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package ccm

func Map[E, F any](in []E, fn func(item E) F) []F {
	out := make([]F, 0, len(in))
	for i := range in {
		out = append(out, fn(in[i]))
	}
	return out
}

func ToList[E comparable, F, G any](in map[E]F, fn func(key E, value F) G) []G {
	out := make([]G, 0, len(in))
	for k, v := range in {
		out = append(out, fn(k, v))
	}
	return out
}
