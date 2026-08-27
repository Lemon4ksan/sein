// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package masque_test

import (
	"testing"

	"github.com/lemon4ksan/sein/tunnel/masque"
)

func FuzzMASQUECapsule(f *testing.F) {
	f.Add([]byte{0x01, 0x07, 0x01, 0x04, 192, 0, 2, 1, 32})
	f.Add([]byte{0x02, 0x07, 0x01, 0x04, 192, 0, 2, 0, 24})
	f.Add([]byte{0x03, 0x06, 0x01, 0x04, 10, 0, 0, 1})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		capsuleType, payloadLen, hdrLen, err := masque.DecodeCapsuleHeader(data)
		if err != nil {
			return
		}
		if int(hdrLen+int(payloadLen)) <= len(data) {
			payload := data[hdrLen : hdrLen+int(payloadLen)]
			switch capsuleType {
			case masque.CapsuleAddressAssign:
				_, _ = masque.DecodeAddressAssignPayloadPOD(nil, payload)
			case masque.CapsuleAddressRequest:
				_, _ = masque.DecodeAddressRequestPayload(payload)
			case masque.CapsuleRouteAdvertisement:
				_, _ = masque.DecodeRouteAdvertisementPayload(payload)
			}
		}
	})
}
