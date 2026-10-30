package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/rushikeshg25/runnelix/internal/identity"
)

func TestMembershipPermissions(t *testing.T) {
	for _, tc := range []struct {
		m       Membership
		allowed Permission
	}{
		{Membership{Role: "viewer", Publish: true, Execute: true}, Read},
		{Membership{Role: "viewer", DataRead: true}, Read | DataRead},
		{Membership{Role: "editor"}, Read | Edit},
		{Membership{Role: "editor", Publish: true, Execute: true}, Read | Edit | Publish | Execute},
		{Membership{Role: "operator"}, Read | Execute},
		{Membership{Role: "owner"}, Read | Edit | Publish | Execute | DataRead | Manage},
		{Membership{Role: "invalid", DataRead: true}, 0},
	} {
		for _, p := range []Permission{Read, Edit, Publish, Execute, DataRead, Manage, Execute | DataRead, 0, 128} {
			want := p != 0 && tc.allowed&p == p
			if tc.m.Allows(p) != want {
				t.Fatalf("%+v permission %d", tc.m, p)
			}
		}
	}
}
func TestMembershipOwnersAndIsolation(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	ws := workspace(t, s)
	owner := identity.Principal{Issuer: "https://issuer.example", Subject: "owner"}
	if err := s.ProvisionOwner(ctx, ws, owner); err != nil {
		t.Fatal(err)
	}
	if err := s.ProvisionOwner(ctx, ws, owner); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	member := Membership{Issuer: owner.Issuer, Subject: owner.Subject, Role: "viewer"}
	for _, remove := range []bool{false, true} {
		if err := s.ChangeMembership(ctx, ws, owner, member, remove); !errors.Is(err, ErrConflict) {
			t.Fatal("last owner changed", err)
		}
	}
	for _, p := range []identity.Principal{{Issuer: "https://other.example", Subject: owner.Subject}, {Issuer: owner.Issuer, Subject: "other"}} {
		if _, err := s.Membership(ctx, ws, p); !errors.Is(err, ErrForbidden) {
			t.Fatal("principal collision", err)
		}
		if err := s.ChangeMembership(ctx, ws, p, member, false); !errors.Is(err, ErrForbidden) {
			t.Fatal("non-owner mutation", err)
		}
	}
	if _, err := s.Membership(ctx, workspace(t, s), owner); !errors.Is(err, ErrForbidden) {
		t.Fatal("cross workspace", err)
	}
	second := Membership{Issuer: owner.Issuer, Subject: "second", Role: "owner"}
	if err := s.ChangeMembership(ctx, ws, owner, second, false); err != nil {
		t.Fatal(err)
	}
	// Concurrent self-demotions must leave one owner, irrespective of lock order.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, m := range []Membership{member, second} {
		wg.Add(1)
		go func(m Membership) {
			defer wg.Done()
			m.Role = "viewer"
			results <- s.ChangeMembership(ctx, ws, identity.Principal{Issuer: m.Issuer, Subject: m.Subject}, m, false)
		}(m)
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
}
