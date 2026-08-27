// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type fuzzTarget struct {
	pkg  string
	name string
}

var targets = []fuzzTarget{
	{"./internal/fast/h1engine", "FuzzH1Request"},
	{"./internal/fast/h1engine", "FuzzH1Chunked"},
	{"./internal/fast/h1engine", "FuzzH1Header"},
	{"./internal/fast/h2engine", "FuzzHPACKDecode"},
	{"./internal/fast/h2engine", "FuzzFrameRead"},
	{"./internal/fast/h2engine", "FuzzHuffmanDecode"},
	{"./internal/fast/h3engine", "FuzzQPACKDecode"},
	{"./internal/fast/h3engine", "FuzzH3FrameHeaderRead"},
	{"./internal/qpack", "FuzzQPACKDecoder"},
	{"./internal/qpack", "FuzzVarint"},
	{"./builtin/jwt", "FuzzJWTVerify"},
	{"./builtin/csrf", "FuzzCSRFTokenComparison"},
	{"./builtin/etag", "FuzzETagMatch"},
	{"./builtin/sse", "FuzzSSEFormatting"},
	{"./ws", "FuzzWSMask"},
	{"./ws", "FuzzWSAcceptKey"},
	{"./tunnel/masque", "FuzzMASQUECapsule"},
	{"./internal/binder", "FuzzBinderIngest"},
	{"./x/openapi", "FuzzOpenAPIPathConversion"},
	{"./x/openapi", "FuzzOpenAPITagParsing"},
	{"./x/otel", "FuzzParseTraceParent"},
	{"./x/sentry", "FuzzSentryDSN"},
	{"./internal/compress", "FuzzGzipRoundtrip"},
	{"./internal/compress", "FuzzDecompressLimit"},
	{"./rpc", "FuzzResolvePathParams"},
	{"./grpc/metadata", "FuzzMetadataCopyToHTTP"},
}

func main() {
	fuzzDuration := flag.String("fuzztime", "2s", "duration to fuzz each target")
	flag.Parse()

	fmt.Printf("=== Starting Heavy Fuzzing Suite (%d targets, %s each) ===\n\n", len(targets), *fuzzDuration)

	var failed []string
	startTotal := time.Now()

	for i, tgt := range targets {
		fmt.Printf("[%2d/%2d] Fuzzing %s :: %s (fuzztime=%s) ... ", i+1, len(targets), tgt.pkg, tgt.name, *fuzzDuration)

		start := time.Now()

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		cmd := exec.CommandContext(ctx, "go", "test", "-fuzz="+tgt.name, "-fuzztime="+*fuzzDuration, "-run=^$", tgt.pkg)

		var outBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = &outBuf

		err := cmd.Run()
		cancel()

		dur := time.Since(start).Round(time.Millisecond)

		if err != nil {
			fmt.Printf("FAILED (%s)\n", dur)
			fmt.Printf("--- Output for %s ---\n%s\n-----------------------\n", tgt.name, outBuf.String())
			failed = append(failed, fmt.Sprintf("%s (%s)", tgt.name, tgt.pkg))
		} else {
			fmt.Printf("PASSED (%s)\n", dur)
		}
	}

	totalDur := time.Since(startTotal).Round(time.Millisecond)
	fmt.Printf("\n=== Fuzzing Complete: %d passed, %d failed in %s ===\n", len(targets)-len(failed), len(failed), totalDur)

	if len(failed) > 0 {
		fmt.Printf("Failed targets:\n")
		for _, f := range failed {
			fmt.Printf("  - %s\n", f)
		}
		os.Exit(1)
	}
}
