package core

import (
	"context"
	"errors"
	"testing"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestUserInvites(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	nathan, _ := signedUpAccount(t, db)

	invites, err := AccountInvites(ctx, db, nathan)
	if err != nil || invites.Left != 3 || len(invites.Made) != 0 || invites.Paused {
		t.Fatalf("new account's invites = %+v, %v", invites, err)
	}
	first, err := MakeInvite(ctx, db, nathan)
	if err != nil || len(first.Code) != 9 {
		t.Fatalf("make = %+v, %v", first, err)
	}
	check, err := CheckInvite(ctx, db, " "+first.Code+" ")
	if err != nil || !check.Valid || check.InvitedBy != "nathan" {
		t.Errorf("check = %+v, %v", check, err)
	}
	if check, _ := CheckInvite(ctx, db, "NOPE-NOPE"); check.Valid {
		t.Errorf("a made-up code = %+v", check)
	}
	operator, _ := CreateInvite(ctx, db, "for the Discord")
	if check, _ := CheckInvite(ctx, db, operator); !check.Valid || check.InvitedBy != "" {
		t.Errorf("an operator invite = %+v", check)
	}

	if _, err := StartSignUp(ctx, db, SignUp{Email: "sam@example.com", Name: "Sam", Username: "sam", Invite: first.Code}, true); err != nil {
		t.Fatal(err)
	}
	if check, _ := CheckInvite(ctx, db, first.Code); check.Valid {
		t.Errorf("a used invite = %+v", check)
	}
	invites, _ = AccountInvites(ctx, db, nathan)
	if invites.Left != 2 || len(invites.Made) != 1 || invites.Made[0].UsedBy != "sam" || invites.Made[0].UsedAt == nil {
		t.Errorf("after sam joined = %+v", invites)
	}

	for range 2 {
		if _, err := MakeInvite(ctx, db, nathan); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := MakeInvite(ctx, db, nathan); !errors.Is(err, ErrNoInvitesLeft) {
		t.Errorf("a fourth invite = %v", err)
	}
	left, err := GrantInvites(ctx, db, "NATHAN", 2)
	if err != nil || left != 2 {
		t.Errorf("grant = %d, %v", left, err)
	}
	if _, err := GrantInvites(ctx, db, "nobody", 2); !errors.Is(err, ErrNotFound) {
		t.Errorf("granting to nobody = %v", err)
	}

	if err := PauseInvites(ctx, db, true); err != nil {
		t.Fatal(err)
	}
	if _, err := MakeInvite(ctx, db, nathan); !errors.Is(err, ErrInvitesPaused) {
		t.Errorf("making an invite while paused = %v", err)
	}
	if check, _ := CheckInvite(ctx, db, invites.Made[0].Code); check.Valid {
		t.Errorf("used codes stay used while paused = %+v", check)
	}
	if err := PauseInvites(ctx, db, false); err != nil {
		t.Fatal(err)
	}
	if _, err := MakeInvite(ctx, db, nathan); err != nil {
		t.Errorf("after resuming = %v", err)
	}
}
