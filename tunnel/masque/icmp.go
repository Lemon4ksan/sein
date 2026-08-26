// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package masque

import (
	"encoding/binary"
	"net/netip"
)

// BuildICMPPacketTooBig4 constructs an ICMPv4 Fragmentation Needed packet (RFC 1191 / RFC 4884 / RFC 792 / RFC 9484 §7.2.1).
func BuildICMPPacketTooBig4(packet []byte, nextHopMTU uint16) ([]byte, error) {
	if len(packet) < 20 || (packet[0]>>4) != 4 {
		return nil, ErrInvalidIPHeader
	}

	if nextHopMTU < 68 {
		return nil, ErrMTUTooSmall
	}

	ipHdrLen := int(packet[0]&0x0f) * 4
	if len(packet) < ipHdrLen {
		return nil, ErrInvalidIPHeader
	}

	originalLen := min(max(len(packet), 128), 500)
	paddedLen := (originalLen + 3) &^ 3

	totalLen := 20 + 8 + paddedLen
	out := make([]byte, totalLen)

	out[0] = 0x45
	out[1] = 0x00
	binary.BigEndian.PutUint16(out[2:4], uint16(totalLen))
	out[8] = 64
	out[9] = 1

	copy(out[12:16], packet[16:20])
	copy(out[16:20], packet[12:16])

	out[20] = 3
	out[21] = 4
	out[22] = 0
	out[23] = 0
	out[24] = 0
	out[25] = byte(paddedLen / 4)
	binary.BigEndian.PutUint16(out[26:28], nextHopMTU)

	copy(out[28:], packet[:min(len(packet), originalLen)])

	binary.BigEndian.PutUint16(out[10:12], calculateInternetChecksum(out[:20]))
	binary.BigEndian.PutUint16(out[22:24], calculateInternetChecksum(out[20:]))

	return out, nil
}

// BuildICMPPacketTooBig6 constructs an ICMPv6 Packet Too Big packet (RFC 4443 §3.2 / RFC 8200 / RFC 9484 §7.2.1).
func BuildICMPPacketTooBig6(packet []byte, nextHopMTU uint32) ([]byte, error) {
	if len(packet) < 40 || (packet[0]>>4) != 6 {
		return nil, ErrInvalidIPHeader
	}

	if nextHopMTU < 1280 {
		return nil, ErrMTUTooSmall
	}

	originalLen := min(len(packet), 1200)
	totalLen := 40 + 8 + originalLen
	out := make([]byte, totalLen)

	out[0] = 0x60
	binary.BigEndian.PutUint16(out[4:6], uint16(8+originalLen))
	out[6] = 58
	out[7] = 64

	copy(out[8:24], packet[24:40])
	copy(out[24:40], packet[8:24])

	out[40] = 2
	out[41] = 0
	out[42] = 0
	out[43] = 0
	binary.BigEndian.PutUint32(out[44:48], nextHopMTU)

	copy(out[48:], packet[:originalLen])

	srcAddr, _ := netip.AddrFromSlice(out[8:24])
	dstAddr, _ := netip.AddrFromSlice(out[24:40])

	csum := calculateICMPv6Checksum(srcAddr, dstAddr, out[40:])
	binary.BigEndian.PutUint16(out[42:44], csum)

	return out, nil
}

func calculateInternetChecksum(b []byte) uint16 {
	var sum uint32

	for i := 0; i < len(b)-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(b[i : i+2]))
	}

	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}

	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}

	return ^uint16(sum)
}

func calculateICMPv6Checksum(srcIP, dstIP netip.Addr, icmpMessage []byte) uint16 {
	var ph [40]byte

	srcBytes := srcIP.As16()
	dstBytes := dstIP.As16()

	copy(ph[0:16], srcBytes[:])
	copy(ph[16:32], dstBytes[:])
	binary.BigEndian.PutUint32(ph[32:36], uint32(len(icmpMessage)))
	ph[39] = 58

	var sum uint32

	for i := 0; i < 40; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(ph[i : i+2]))
	}

	for i := 0; i < len(icmpMessage)-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(icmpMessage[i : i+2]))
	}

	if len(icmpMessage)%2 == 1 {
		sum += uint32(icmpMessage[len(icmpMessage)-1]) << 8
	}

	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}

	return ^uint16(sum)
}
