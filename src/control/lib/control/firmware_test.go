//
// (C) Copyright 2020 Intel Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// GOVERNMENT LICENSE RIGHTS-OPEN SOURCE SOFTWARE
// The Government's rights to use, modify, reproduce, release, perform, display,
// or disclose this software are subject to the terms of the Apache License as
// provided in Contract No. 8F-30005.
// Any reproduction of computer software, computer software documentation, or
// portions thereof marked with this legend must also reproduce the markings.
//
// +build firmware

package control

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"

	"github.com/daos-stack/daos/src/control/common"
	"github.com/daos-stack/daos/src/control/lib/hostlist"
	"github.com/daos-stack/daos/src/control/logging"
)

func createTestHostSet(t *testing.T, hosts string) *hostlist.HostSet {
	set, err := hostlist.CreateSet(hosts)
	if err != nil {
		t.Fatalf("couldn't create host set: %s", err)
	}
	return set
}

func TestControl_FirmwareQuery(t *testing.T) {
	for name, tc := range map[string]struct {
		mic     *MockInvokerConfig
		req     *FirmwareQueryReq
		expResp *FirmwareQueryResp
		expErr  error
	}{
		"local failure": {
			req: &FirmwareQueryReq{SCM: true},
			mic: &MockInvokerConfig{
				UnaryError: errors.New("local failed"),
			},
			expErr: errors.New("local failed"),
		},
		"remote failure": {
			req: &FirmwareQueryReq{SCM: true},
			mic: &MockInvokerConfig{
				UnaryResponse: MockMSResponse("host1", errors.New("remote failed"), nil),
			},
			expResp: &FirmwareQueryResp{
				HostErrorsResp: HostErrorsResp{
					HostErrors: HostErrorsMap{
						"remote failed": &HostErrorSet{
							HostSet:   createTestHostSet(t, "host1"),
							HostError: errors.New("remote failed"),
						},
					},
				},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			log, buf := logging.NewTestLogger(t.Name())
			defer common.ShowBufferOnFailure(t, buf)

			mic := tc.mic
			if mic == nil {
				mic = DefaultMockInvokerConfig()
			}

			ctx := context.TODO()
			mi := NewMockInvoker(log, mic)

			gotResp, gotErr := FirmwareQuery(ctx, mi, tc.req)
			common.CmpErr(t, tc.expErr, gotErr)
			if tc.expErr != nil {
				return
			}

			cmpOpts := []cmp.Option{
				cmp.Comparer(func(r1, r2 *FirmwareQueryResp) bool {
					if len(r1.HostErrors) != len(r2.HostErrors) {
						return false
					}
					for errName, errorSet1 := range r1.HostErrors {
						errorSet2, ok := r2.HostErrors[errName]
						if !ok {
							return false
						}
						if errorSet2.HostSet.String() != errorSet1.HostSet.String() {
							return false
						}
					}
					// TODO KJ: Compare successful response
					return true
				}),
			}
			if diff := cmp.Diff(tc.expResp, gotResp, cmpOpts...); diff != "" {
				t.Fatalf("Unexpected response (-want, +got):\n%s\n", diff)
			}
		})
	}
}
