package xxhash

import (
	"testing"
)

func TestXXHash64Bytes(t *testing.T) {
	b := make([]byte, 64)
	for i := range b {
		b[i] = byte('A' + (i % 26))
	}

	fastSum := Sum64(b)

	d := New()
	d.Write(b)
	streamSum := d.Sum64()

	if fastSum != streamSum {
		t.Fatalf("mismatch: Sum64=%016x, Digest=%016x", fastSum, streamSum)
	}
}
