// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"cmp"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// VersionMatrix manages multi-version routing trees (/v1, /v2, /v3) with declarative lifecycle filters.
type VersionMatrix struct {
	server       RouteBuilder
	allVersions  []string
	formatPrefix func(version string) string
	groups       map[string]*Group
}

// VersionGroup represents a scoped view over a VersionMatrix with specific active versions, path prefix, and middlewares.
type VersionGroup struct {
	matrix       *VersionMatrix
	activeVers   []string
	prefix       string
	middlewares  []Middleware
	errorMappers []ErrorMapperFunc
	mu           sync.RWMutex
}

// VersionGuardScope represents a protected multi-version scope configured with guards.
type VersionGuardScope struct {
	*VersionGroup
}

// Versioned initializes a declarative multi-version routing matrix for the specified API versions (e.g. "2", "3" or "v2", "v3").
// It maps each version under "/v{ver}" automatically.
func (s *Server) Versioned(versions ...string) *VersionGroup {
	return s.VersionMatrix(func(v string) string {
		v = strings.TrimPrefix(v, "/")
		if !strings.HasPrefix(strings.ToLower(v), "v") {
			return "/v" + v
		}
		return "/" + v
	}, versions...)
}

// VersionMatrix initializes a multi-version routing matrix with a custom version prefix formatter.
func (s *Server) VersionMatrix(prefixFormatter func(version string) string, versions ...string) *VersionGroup {
	if prefixFormatter == nil {
		prefixFormatter = func(v string) string {
			v = strings.TrimPrefix(v, "/")
			if !strings.HasPrefix(strings.ToLower(v), "v") {
				return "/v" + v
			}
			return "/" + v
		}
	}

	matrix := &VersionMatrix{
		server:       s,
		allVersions:  slices.Clone(versions),
		formatPrefix: prefixFormatter,
		groups:       make(map[string]*Group, len(versions)),
	}

	for _, v := range versions {
		prefix := prefixFormatter(v)
		matrix.groups[v] = s.Group(prefix)
	}

	return &VersionGroup{
		matrix:     matrix,
		activeVers: slices.Clone(versions),
	}
}

// Since filters active versions to those greater than or equal to minVersion (v >= minVersion).
func (vg *VersionGroup) Since(minVersion string) *VersionGroup {
	var filtered []string
	for _, v := range vg.activeVers {
		if compareVersions(v, minVersion) >= 0 {
			filtered = append(filtered, v)
		}
	}

	return &VersionGroup{
		matrix:       vg.matrix,
		activeVers:   filtered,
		prefix:       vg.prefix,
		middlewares:  slices.Clone(vg.middlewares),
		errorMappers: slices.Clone(vg.errorMappers),
	}
}

// Until filters active versions to those less than or equal to maxVersion (v <= maxVersion).
func (vg *VersionGroup) Until(maxVersion string) *VersionGroup {
	var filtered []string
	for _, v := range vg.activeVers {
		if compareVersions(v, maxVersion) <= 0 {
			filtered = append(filtered, v)
		}
	}

	return &VersionGroup{
		matrix:       vg.matrix,
		activeVers:   filtered,
		prefix:       vg.prefix,
		middlewares:  slices.Clone(vg.middlewares),
		errorMappers: slices.Clone(vg.errorMappers),
	}
}

// Between filters active versions to those within the range [minVersion, maxVersion].
func (vg *VersionGroup) Between(minVersion, maxVersion string) *VersionGroup {
	var filtered []string
	for _, v := range vg.activeVers {
		if compareVersions(v, minVersion) >= 0 && compareVersions(v, maxVersion) <= 0 {
			filtered = append(filtered, v)
		}
	}

	return &VersionGroup{
		matrix:       vg.matrix,
		activeVers:   filtered,
		prefix:       vg.prefix,
		middlewares:  slices.Clone(vg.middlewares),
		errorMappers: slices.Clone(vg.errorMappers),
	}
}

// Only restricts active versions strictly to the given versions list.
func (vg *VersionGroup) Only(versions ...string) *VersionGroup {
	var filtered []string
	for _, v := range vg.activeVers {
		for _, target := range versions {
			if normalizeVersion(v) == normalizeVersion(target) {
				filtered = append(filtered, v)
				break
			}
		}
	}

	return &VersionGroup{
		matrix:       vg.matrix,
		activeVers:   filtered,
		prefix:       vg.prefix,
		middlewares:  slices.Clone(vg.middlewares),
		errorMappers: slices.Clone(vg.errorMappers),
	}
}

// Except removes the given versions from the active versions list.
func (vg *VersionGroup) Except(versions ...string) *VersionGroup {
	var filtered []string
	for _, v := range vg.activeVers {
		excluded := false
		for _, target := range versions {
			if normalizeVersion(v) == normalizeVersion(target) {
				excluded = true
				break
			}
		}
		if !excluded {
			filtered = append(filtered, v)
		}
	}

	return &VersionGroup{
		matrix:       vg.matrix,
		activeVers:   filtered,
		prefix:       vg.prefix,
		middlewares:  slices.Clone(vg.middlewares),
		errorMappers: slices.Clone(vg.errorMappers),
	}
}

// Do executes a configuration callback on this VersionGroup.
func (vg *VersionGroup) Do(fn func(g *VersionGroup)) *VersionGroup {
	fn(vg)
	return vg
}

// Group creates a nested sub-group under this multi-version group's path prefix.
func (vg *VersionGroup) Group(prefix string, mw ...Middleware) *VersionGroup {
	fullPrefix := joinPaths(vg.prefix, prefix)
	return &VersionGroup{
		matrix:       vg.matrix,
		activeVers:   slices.Clone(vg.activeVers),
		prefix:       cleanPrefix(fullPrefix),
		middlewares:  append(slices.Clone(vg.middlewares), mw...),
		errorMappers: slices.Clone(vg.errorMappers),
	}
}

// Guard creates a protected VersionGuardScope within this multi-version group.
func (vg *VersionGroup) Guard(mw ...Middleware) *VersionGuardScope {
	return &VersionGuardScope{
		VersionGroup: vg.Group("", mw...),
	}
}

// Do executes the callback within the protected VersionGuardScope.
func (vgs *VersionGuardScope) Do(fn func(g *VersionGroup)) *VersionGuardScope {
	fn(vgs.VersionGroup)
	return vgs
}

// MapError registers a domain error mapping rule on the version guard scope.
func (vgs *VersionGuardScope) MapError(target error, domainErr DomainError) *VersionGuardScope {
	vgs.VersionGroup.MapError(target, domainErr)
	return vgs
}

// MapErrors registers multiple scoped error mappings on the version guard scope.
func (vgs *VersionGuardScope) MapErrors(errorsMap Errors) *VersionGuardScope {
	vgs.VersionGroup.MapErrors(errorsMap)
	return vgs
}

// Use appends middlewares to the multi-version group.
func (vg *VersionGroup) Use(mw ...Middleware) *VersionGroup {
	vg.middlewares = append(vg.middlewares, mw...)
	return vg
}

// MapError registers a mapping from an internal sentinel error to a Sein domain error.
func (vg *VersionGroup) MapError(target error, domainErr DomainError) *VersionGroup {
	vg.mu.Lock()
	defer vg.mu.Unlock()

	vg.errorMappers = append(vg.errorMappers, func(err error) (DomainError, bool) {
		if err == target {
			return domainErr, true
		}
		return nil, false
	})

	return vg
}

// MapErrors registers multiple error mappings on the multi-version group using an Errors table.
func (vg *VersionGroup) MapErrors(errorsMap Errors) *VersionGroup {
	vg.mu.Lock()
	defer vg.mu.Unlock()

	for target, domainErr := range errorsMap {
		t := target
		d := domainErr
		vg.errorMappers = append(vg.errorMappers, func(err error) (DomainError, bool) {
			if err == t {
				return d, true
			}
			return nil, false
		})
	}

	return vg
}

// Mount attaches a domain Module under this multi-version group.
func (vg *VersionGroup) Mount(prefix string, m Module, mw ...Middleware) *VersionGroup {
	sub := vg.Group(prefix, mw...)
	for _, ver := range vg.activeVers {
		targetGroup := vg.matrix.groups[ver]
		if targetGroup != nil {
			versionSub := targetGroup.Group(sub.prefix, sub.middlewares...)
			m.Mount(versionSub)
		}
	}
	return vg
}

func (vg *VersionGroup) registerRoute(method, path string, handler RawHandler, mw ...Middleware) {
	for _, ver := range vg.activeVers {
		targetGroup := vg.matrix.groups[ver]
		if targetGroup == nil {
			continue
		}

		fullPath := joinPaths(vg.prefix, path)
		combinedMW := append(slices.Clone(vg.middlewares), mw...)

		if len(vg.errorMappers) == 0 {
			targetGroup.registerRoute(method, fullPath, handler, combinedMW...)
			continue
		}

		mappers := slices.Clone(vg.errorMappers)
		wrappedHandler := func(req *Request) (any, error) {
			res, err := handler(req)
			if err != nil {
				for _, mapper := range mappers {
					if mapped, ok := mapper(err); ok {
						return nil, mapped
					}
				}
				return nil, err
			}
			return res, nil
		}
		targetGroup.registerRoute(method, fullPath, wrappedHandler, combinedMW...)
	}
}

// Get registers a GET route handler across all active versions in this group.
func (vg *VersionGroup) Get(path string, handler any, mw ...Middleware) {
	routeUniversal(vg, http.MethodGet, path, handler, mw...)
}

// Post registers a POST route handler across all active versions in this group.
func (vg *VersionGroup) Post(path string, handler any, mw ...Middleware) {
	routeUniversal(vg, http.MethodPost, path, handler, mw...)
}

// Patch registers a PATCH route handler across all active versions in this group.
func (vg *VersionGroup) Patch(path string, handler any, mw ...Middleware) {
	routeUniversal(vg, http.MethodPatch, path, handler, mw...)
}

// Put registers a PUT route handler across all active versions in this group.
func (vg *VersionGroup) Put(path string, handler any, mw ...Middleware) {
	routeUniversal(vg, http.MethodPut, path, handler, mw...)
}

// Delete registers a DELETE route handler across all active versions in this group.
func (vg *VersionGroup) Delete(path string, handler any, mw ...Middleware) {
	routeUniversal(vg, http.MethodDelete, path, handler, mw...)
}

// Options registers an OPTIONS route handler across all active versions in this group.
func (vg *VersionGroup) Options(path string, handler any, mw ...Middleware) {
	routeUniversal(vg, http.MethodOptions, path, handler, mw...)
}

// Head registers a HEAD route handler across all active versions in this group.
func (vg *VersionGroup) Head(path string, handler any, mw ...Middleware) {
	routeUniversal(vg, http.MethodHead, path, handler, mw...)
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "/")
	v = strings.TrimPrefix(strings.ToLower(v), "v")
	return v
}

func compareVersions(a, b string) int {
	na := normalizeVersion(a)
	nb := normalizeVersion(b)

	partsA := strings.Split(na, ".")
	partsB := strings.Split(nb, ".")

	maxLen := max(len(partsA), len(partsB))
	for i := 0; i < maxLen; i++ {
		var segA, segB string
		if i < len(partsA) {
			segA = partsA[i]
		}
		if i < len(partsB) {
			segB = partsB[i]
		}

		numA, errA := strconv.Atoi(segA)
		numB, errB := strconv.Atoi(segB)

		if errA == nil && errB == nil {
			if numA != numB {
				return cmp.Compare(numA, numB)
			}
		} else {
			if c := cmp.Compare(segA, segB); c != 0 {
				return c
			}
		}
	}

	return 0
}
