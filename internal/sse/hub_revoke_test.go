package sse

import (
	"testing"

	"github.com/google/uuid"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/stretchr/testify/require"
)

// TestHubRevokeUserClosesOnlyThatUsersStreams
func TestHubRevokeUserClosesOnlyThatUsersStreams(t *testing.T) {
	hub := NewHub()
	alice, _ := uuid.Parse("11111111-1111-1111-1111-111111111111")
	bob, _ := uuid.Parse("22222222-2222-2222-2222-222222222222")

	aliceCh := hub.register(alice)
	bobCh := hub.register(bob)
	require.Equal(t, 2, hub.ClientCount())

	hub.RevokeUser(alice)

	_, open := <-aliceCh
	require.False(t, open, "alice's stream must be closed")
	_ = bobCh
	require.Equal(t, 1, hub.ClientCount(), "bob's stream must survive")
}

// TestHubSatisfiesTheAuthRevokerSeam pins the contract Task 4 depends on.
func TestHubSatisfiesTheAuthRevokerSeam(t *testing.T) {
	var _ auth.SessionRevoker = NewHub()
}
