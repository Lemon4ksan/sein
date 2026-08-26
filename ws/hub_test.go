// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws_test

import (
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/sein/ws"
)

func TestHub_Subscriptions(t *testing.T) {
	hub := ws.NewHub()

	// Dummy pointers representing connections
	var c1 ws.Conn
	var c2 ws.Conn

	hub.Subscribe("general", &c1)
	hub.Subscribe("general", &c2)
	hub.Subscribe("random", &c1)

	assert.Equal(t, 2, hub.SubscribersCount("general"))
	assert.Equal(t, 1, hub.SubscribersCount("random"))
	assert.Equal(t, 2, hub.TopicsCount())
	assert.Equal(t, 2, hub.ConnectionsCount())

	hub.Unsubscribe("general", &c2)
	assert.Equal(t, 1, hub.SubscribersCount("general"))

	hub.UnsubscribeAll(&c1)
	assert.Equal(t, 0, hub.SubscribersCount("general"))
	assert.Equal(t, 0, hub.SubscribersCount("random"))
	assert.Equal(t, 0, hub.TopicsCount())
	assert.Equal(t, 0, hub.ConnectionsCount())
}
