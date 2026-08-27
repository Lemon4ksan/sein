// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package masque_test

import (
	"net/netip"
	"testing"

	"github.com/lemon4ksan/foundation/silicon/offheap"
	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"
	"github.com/lemon4ksan/sein/tunnel/masque"
)

func TestCapsule_AddressAssign_Roundtrip(t *testing.T) {
	entries := []masque.AssignedAddress{
		{
			RequestID:    1,
			Addr:         netip.MustParseAddr("10.0.0.1"),
			PrefixLength: 32,
		},
		{
			RequestID:    2,
			Addr:         netip.MustParseAddr("fd00::1"),
			PrefixLength: 128,
		},
	}

	var buf [2048]byte
	n := masque.EncodeAddressAssignCapsule(entries, buf[:])
	require.Greater(t, n, 0)

	// Decode capsule header
	capsuleType, payloadLen, hdrLen, err := masque.DecodeCapsuleHeader(buf[:n])
	require.NoError(t, err)
	assert.Equal(t, masque.CapsuleAddressAssign, capsuleType)
	assert.Equal(t, uint64(n-hdrLen), payloadLen)

	// Decode payload
	payload := buf[hdrLen:n]
	decoded, err := masque.DecodeAddressAssignPayload(payload)
	require.NoError(t, err)
	require.Len(t, decoded, 2)

	assert.Equal(t, entries[0].Addr, decoded[0].Addr)
	assert.Equal(t, entries[0].RequestID, decoded[0].RequestID)
	assert.Equal(t, entries[1].Addr, decoded[1].Addr)
	assert.Equal(t, entries[1].RequestID, decoded[1].RequestID)
}

func TestCapsule_AddressAssign_POD_And_Slab(t *testing.T) {
	entries := []masque.AssignedAddress{
		{
			RequestID:    42,
			Addr:         netip.MustParseAddr("192.168.1.100"),
			PrefixLength: 24,
		},
	}

	var buf [2048]byte
	n := masque.EncodeAddressAssignCapsule(entries, buf[:])
	require.Greater(t, n, 0)

	_, _, hdrLen, err := masque.DecodeCapsuleHeader(buf[:n])
	require.NoError(t, err)
	payload := buf[hdrLen:n]

	// 1. Offheap Arena Decode
	_ = offheap.Scope(4096, func(arena *offheap.Arena) {
		pods, err := masque.DecodeAddressAssignPayloadPOD(arena, payload)
		require.NoError(t, err)
		require.Len(t, pods, 1)
		assert.Equal(t, uint64(42), pods[0].RequestID)
		assert.Equal(t, byte(4), pods[0].IPVersion)
	})

	// 2. Slab Allocator Decode
	slab, err := masque.NewAssignedAddressSlab(16)
	require.NoError(t, err)

	podsSlab, err := masque.DecodeAddressAssignPayloadPODSlab(slab, payload)
	require.NoError(t, err)
	require.Len(t, podsSlab, 1)
	assert.Equal(t, uint64(42), podsSlab[0].RequestID)
}

func TestCapsule_AddressRequest_And_RouteAdv(t *testing.T) {
	// 1. Address Request
	reqEntries := []masque.RequestedAddress{
		{
			RequestID:    10,
			Addr:         netip.MustParseAddr("10.0.0.5"),
			PrefixLength: 32,
		},
	}

	var reqBuf [2048]byte
	nReq, err := masque.EncodeAddressRequestCapsule(reqEntries, reqBuf[:])
	require.NoError(t, err)
	require.Greater(t, nReq, 0)

	_, _, hdrLenReq, err := masque.DecodeCapsuleHeader(reqBuf[:nReq])
	require.NoError(t, err)

	decodedReq, err := masque.DecodeAddressRequestPayload(reqBuf[hdrLenReq:nReq])
	require.NoError(t, err)
	require.Len(t, decodedReq, 1)
	assert.Equal(t, reqEntries[0].Addr, decodedReq[0].Addr)

	// 2. Route Advertisement
	routeEntries := []masque.IPAddressRange{
		{
			StartIP:    netip.MustParseAddr("172.16.0.0"),
			EndIP:      netip.MustParseAddr("172.16.255.255"),
			IPVersion:  4,
			IPProtocol: 0,
		},
	}

	var routeBuf [2048]byte
	nRoute, err := masque.EncodeRouteAdvertisementCapsule(routeEntries, routeBuf[:])
	require.NoError(t, err)
	require.Greater(t, nRoute, 0)

	_, _, hdrLenRoute, err := masque.DecodeCapsuleHeader(routeBuf[:nRoute])
	require.NoError(t, err)

	decodedRoute, err := masque.DecodeRouteAdvertisementPayload(routeBuf[hdrLenRoute:nRoute])
	require.NoError(t, err)
	require.Len(t, decodedRoute, 1)
	assert.Equal(t, routeEntries[0].StartIP, decodedRoute[0].StartIP)
	assert.Equal(t, routeEntries[0].EndIP, decodedRoute[0].EndIP)
}

func TestCapsule_ErrorsAndEdgeCases(t *testing.T) {
	// Truncated header
	_, _, _, err := masque.DecodeCapsuleHeader([]byte{})
	assert.Error(t, err)

	// Invalid IP version
	badPayload := []byte{
		0x01,       // RequestID=1
		0x99,       // Invalid IP version
		0x00, 0x00, // Short data
	}
	_, err = masque.DecodeAddressAssignPayload(badPayload)
	assert.Error(t, err)
}
