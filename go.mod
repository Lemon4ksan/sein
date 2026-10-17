module github.com/lemon4ksan/sein

go 1.27.0

require (
	github.com/lemon4ksan/foundation v0.2.1
	github.com/lemon4ksan/mach v0.2.0
	golang.org/x/crypto v0.58.0
	golang.org/x/net v0.61.0
	golang.org/x/sys v0.49.0
)

require (
	github.com/lemon4ksan/seal v0.1.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace (
	github.com/lemon4ksan/foundation => ../foundation
	github.com/lemon4ksan/mach => ../mach
)
